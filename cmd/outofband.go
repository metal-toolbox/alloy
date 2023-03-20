package cmd

import (
	"log"
	"time"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/collector"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/spf13/cobra"
)

var (
	// interval is passed to the alloy controller along with collectSplay to set the interval
	// at which the inventory data in the store is refreshed.
	interval time.Duration

	// splay when set adds additional time to the collectInterval
	// the additional time added is between zero and the collectSplay time.Duration value
	splay time.Duration

	// assetIDs is the list of asset IDs to lookup out of band inventory for.
	assetIDs []string

	// controllerMode when enabled runs alloy in controller mode to periodically fetch data for all assets in the
	// configured store.
	controllerMode bool
)

// outofband inventory, bios configuration collection command
var cmdOutofband = &cobra.Command{
	Use:   "outofband",
	Short: "Collect inventory data, bios configuration data through the BMC",
	Run: func(cmd *cobra.Command, args []string) {
		app, err := app.New(cmd.Context(), model.AppKindInband, cfgFile, logLevel)
		if err != nil {
			log.Fatal(err)
		}

		if outputStdout {
			storeKind = string(model.StoreKindMock)
		}

		if len(assetIDs) > 0 {
			c, err := collector.NewSingleDeviceCollector(
				cmd.Context(),
				model.StoreKind(storeKind),
				model.AppKindOutOfBand,
				app.Config,
				app.Logger,
			)

			if err != nil {
				log.Fatal(err)
			}

			for _, assetID := range assetIDs {
				asset := &model.Asset{ID: assetID}
				if err := c.Collect(cmd.Context(), asset); err != nil {
					app.Logger.Warn(err)
				}
			}

			return
		}

		if controllerMode {

			return
		}

		log.Fatal("either --asset-ids OR --controller was expected")
	},
}

// install command flags
func init() {
	cmdOutofband.PersistentFlags().DurationVar(&interval, "interval", 72*time.Hour, "interval sets the periodic data collection interval")
	cmdOutofband.PersistentFlags().DurationVar(&splay, "splay", 3*time.Hour, "splay adds jitter to the collection interval")
	cmdOutofband.PersistentFlags().StringSliceVar(&assetIDs, "asset-ids", []string{}, "Collect inventory for the given comma separated list of asset IDs.")
	cmdOutofband.PersistentFlags().BoolVarP(&controllerMode, "controller-mode", "", false, "Run Alloy in a controller loop that periodically refreshes inventory, BIOS configuration data in the store and listens for events.")

	rootCmd.AddCommand(cmdOutofband)
}
