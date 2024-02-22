package lean

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/ironlib"
	"github.com/metal-toolbox/ironlib/actions"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/metal-toolbox/alloy/cmd"
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
	doInband bool
	getBIOS  bool

	bmcUser string
	bmcPwd  string
	bmcHost string

	timeout time.Duration
)

type inventory struct {
	Device  *common.Device    `json:"device"`
	BiosCfg map[string]string `json:"bios_cfg,omitempty"`
}

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
		logger := &logrus.Logger{
			Formatter: &logrus.JSONFormatter{},
			Level:     logrus.TraceLevel,
		}

		i := getInventorier(ctx, logger)
		if i == nil {
			logger.Fatal("no inventorier available")
		}

		device, err := i.GetInventory(ctx)
		if err != nil {
			logger.WithError(err).Fatal("getting device inventory")
		}

		var biosCfg map[string]string
		if getBIOS {
			biosCfg, err = i.GetBiosConfiguration(ctx)
			if err != nil {
				logger.WithError(err).Warn("collecting BIOS configuration")
				biosCfg = nil
			}
		}

		byt, err := json.MarshalIndent(&inventory{
			Device:  device,
			BiosCfg: biosCfg,
		}, "", " ")

		if err != nil {
			panic(err.Error())
		}

		fmt.Print(string(byt))
	},
}

func init() {
	cmd.RootCmd.AddCommand(lean)
	lean.Flags().StringVarP(&bmcHost, "host", "h", "", "the BMC host")
	lean.Flags().StringVarP(&bmcUser, "user", "u", "", "the BMC user")
	lean.Flags().StringVarP(&bmcPwd, "pwd", "p", "", "the BMC password")
	lean.Flags().BoolVarP(&doInband, "in-band", "i", true, "run in in-band mode")
	lean.Flags().BoolVarP(&getBIOS, "bios", "b", true, "collect bios configuration (in-band mode only)")
	//nolint:gomnd // do shut up.
	lean.Flags().DurationVarP(&timeout, "timeout", "t", 20*time.Minute, "deadline for inventory to complete")
}
