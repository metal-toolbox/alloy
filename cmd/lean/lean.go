package lean

import (
	"context"
	"fmt"
	"time"

	"github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/ironlib"
	"github.com/metal-toolbox/ironlib/actions"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/metal-toolbox/alloy/cmd"
	"github.com/metal-toolbox/alloy/types"
)

type inventorier interface {
	GetInventory(context.Context) (*common.Device, error)
	GetBiosConfiguration(context.Context) (map[string]string, error)
	Close(context.Context) error
}

type ironlibAdapter struct {
	hdl actions.Getter
}

func (ila *ironlibAdapter) GetInventory(c context.Context) (*common.Device, error) {
	// add ironlib options here if desired
	opts := []actions.Option{}
	return ila.hdl.GetInventory(c, opts...)
}

func (ila *ironlibAdapter) GetBiosConfiguration(ctx context.Context) (map[string]string, error) {
	return ila.hdl.GetBIOSConfiguration(ctx)
}

func (*ironlibAdapter) Close(_ context.Context) error {
	return nil
}

type bmclibAdapter struct {
	client *bmclib.Client
}

func (bla *bmclibAdapter) GetInventory(c context.Context) (*common.Device, error) {
	return bla.client.Inventory(c)
}

func (bla *bmclibAdapter) GetBiosConfiguration(c context.Context) (map[string]string, error) {
	return bla.client.GetBiosConfiguration(c)
}

func (bla *bmclibAdapter) Close(c context.Context) error {
	return bla.client.Close(c)
}

var (
	cfgFile  string
	doInband bool
	getBIOS  bool

	bmcUser string
	bmcPwd  string
	bmcHost string

	assetID string

	timeout time.Duration
)

func getInventorier(ctx context.Context, ll *logrus.Logger) inventorier {
	if doInband {
		hdl, err := ironlib.New(ll)
		if err != nil {
			ll.WithError(err).Warn("creating ironlib handle")
			return nil
		}
		return &ironlibAdapter{
			hdl: hdl,
		}
	}

	bmcClient := bmclib.NewClient(bmcHost, bmcUser, bmcPwd)
	err := bmcClient.Open(ctx)
	if err != nil {
		ll.WithError(err).WithFields(logrus.Fields{
			"host": bmcHost,
			"user": bmcUser,
		}).Warn("creating bmc handle")
		return nil
	}
	return &bmclibAdapter{
		client: bmcClient,
	}
}

var lean = &cobra.Command{
	Use:   "lean",
	Short: "very bare-bones collection of inventory",
	Run: func(c *cobra.Command, _ []string) {
		ctx, cancel := context.WithTimeout(c.Context(), timeout)
		defer cancel()
		logger := logrus.New()
		logger.SetFormatter(&logrus.JSONFormatter{})
		logger.SetLevel(logrus.TraceLevel)

		i := getInventorier(ctx, logger)
		if i == nil {
			logger.Fatal("no inventorier available")
		}
		defer i.Close(context.Background())

		device, err := i.GetInventory(ctx)
		if err != nil {
			logger.WithError(err).Fatal("getting device inventory")
		}

		var biosCfg types.BiosConfig
		if getBIOS {
			biosCfg, err = i.GetBiosConfiguration(ctx)
			if err != nil {
				logger.WithError(err).Warn("collecting BIOS configuration")
				biosCfg = nil
			}
		}

		cfg, err := loadConfiguration(cfgFile)
		if err != nil {
			panic(err)
		}

		client, err := NewComponentInventoryClient(ctx, cfg)
		if err != nil {
			// TODO: find a way to handle errors gracefully.
			panic(err)
		}

		cisReq := types.InventoryDevice{
			Inv:     device,
			BiosCfg: biosCfg,
		}
		fmt.Printf("update inventory for server %v: %v\n", assetID, cisReq)

		var cisResp string
		if doInband {
			cisResp, err = client.UpdateInbandInventory(ctx, assetID, &cisReq)
		} else {
			cisResp, err = client.UpdateOutOfbandInventory(ctx, assetID, &cisReq)
		}

		if err != nil {
			panic(err)
		}

		fmt.Print(cisResp)
	},
}

func init() {
	cmd.RootCmd.AddCommand(lean)
	lean.Flags().StringVar(&cfgFile, "config", "", "configuration file")
	lean.Flags().StringVar(&bmcHost, "host", "bogusHost", "the BMC host")
	lean.Flags().StringVarP(&bmcUser, "user", "u", "bogusUser", "the BMC user")
	lean.Flags().StringVarP(&bmcPwd, "pwd", "p", "bogusPwd", "the BMC password")
	lean.Flags().BoolVarP(&doInband, "in-band", "i", true, "run in in-band mode")
	lean.Flags().BoolVarP(&getBIOS, "bios", "b", true, "collect bios configuration (in-band mode only)")
	lean.Flags().StringVarP(&assetID, "asset-id", "", "", "The asset identifier(aka server id) - required when store is set to serverservice")
	//nolint:gomnd // do shut up.
	lean.Flags().DurationVarP(&timeout, "timeout", "t", 20*time.Minute, "deadline for inventory to complete")
}
