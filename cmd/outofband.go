package cmd

import (
	"log"
	"time"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/collector"
	"github.com/metal-toolbox/alloy/internal/controller"
	"github.com/metal-toolbox/alloy/internal/helpers"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/spf13/cobra"
	"go.hollow.sh/toolbox/events"
	"golang.org/x/net/context"
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
		alloy, err := app.New(cmd.Context(), model.AppKindInband, model.StoreKind(storeKind), cfgFile, logLevel)
		if err != nil {
			log.Fatal(err)
		}

		// profiling endpoint
		if enableProfiling {
			helpers.EnablePProfile()
		}

		// serve metrics endpoint
		metrics.ListenAndServe()

		// setup cancel context with cancel func
		ctx, cancelFunc := context.WithCancel(cmd.Context())

		// routine listens for termination signal and cancels the context
		go func() {
			<-alloy.TermCh
			cancelFunc()
		}()

		if outputStdout {
			storeKind = string(model.StoreKindMock)
		}

		if len(assetIDs) > 0 {
			runOnAssets(ctx, alloy)

			return
		}

		if controllerMode {
			runController(ctx, alloy)

			return
		}

		log.Fatal("either --asset-ids OR --controller-mode was expected")
	},
}

func runController(ctx context.Context, alloy *app.App) {
	streamBroker, err := events.NewStreamBroker(*alloy.Config.NatsOptions)
	if err != nil {
		alloy.Logger.Fatal(err)
	}

	acontroller, err := controller.New(ctx, streamBroker, alloy.Config, alloy.SyncWg, alloy.Logger)
	if err != nil {
		alloy.Logger.Fatal(err)
	}

	acontroller.Run(ctx)
}

func runOnAssets(ctx context.Context, alloy *app.App) {
	c, err := collector.NewSingleDeviceCollector(
		ctx,
		model.StoreKind(storeKind),
		model.AppKindOutOfBand,
		alloy.Config,
		alloy.Logger,
	)

	if err != nil {
		log.Fatal(err)
	}

	for _, assetID := range assetIDs {
		asset := &model.Asset{ID: assetID}
		if err := c.CollectOutofband(ctx, asset); err != nil {
			alloy.Logger.Warn(err)
		}
	}
}

// install command flags
func init() {
	cmdOutofband.PersistentFlags().DurationVar(&interval, "collect-interval", 72*time.Hour, "interval sets the periodic data collection interval")
	cmdOutofband.PersistentFlags().DurationVar(&splay, "collect-splay", 3*time.Hour, "splay adds jitter to the collection interval")
	cmdOutofband.PersistentFlags().StringSliceVar(&assetIDs, "asset-ids", []string{}, "Collect inventory for the given comma separated list of asset IDs.")
	cmdOutofband.PersistentFlags().BoolVarP(&controllerMode, "controller-mode", "", false, "Run Alloy in a controller loop that periodically refreshes inventory, BIOS configuration data in the store and listens for events.")

	rootCmd.AddCommand(cmdOutofband)
}
