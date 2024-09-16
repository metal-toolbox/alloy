package cmd

import (
	"log"
	"time"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/collector"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

var (
	// inbandTimeout time.Duration string value is used to timeout the inventory collection operation.
	inbandTimeout time.Duration

	assetID string
)

// inband inventory collection command
var cmdInband = &cobra.Command{
	Use:   "inband",
	Short: "Collect inventory data, bios configuration data on the host",
	Run: func(cmd *cobra.Command, _ []string) {
		alloy, err := app.New(model.AppKindInband, model.StoreKind(storeKind), cfgFile, model.LogLevel(logLevel))
		if err != nil {
			log.Fatal(err)
		}

		if outputStdout {
			storeKind = string(model.StoreKindMock)
		}

		if storeKind == string(model.StoreKindFleetDB) && assetID == "" {
			log.Fatal("--asset-id flag required for inband command with fleetdb store")
		}

		// execution timeout
		timeoutC := time.NewTimer(inbandTimeout).C

		// setup cancel context with cancel func
		ctx, cancelFunc := context.WithCancel(cmd.Context())
		defer cancelFunc()

		// doneCh is where the goroutine below notifies when its complete
		doneCh := make(chan struct{})

		// spawn collect routine in the background
		go func() {
			defer func() {
				if ctx.Err() == nil {
					doneCh <- struct{}{}
				}
			}()

			collectInband(ctx, alloy.Config, alloy.Logger)
		}()

		// loop with a timeout to ensure collection does not exceed the configured timeout.
	Loop:
		for {
			select {
			case <-timeoutC:
				alloy.Logger.Error("aborted, timeout exceeded: " + inbandTimeout.String())
				break Loop
			case <-alloy.TermCh:
				alloy.Logger.Error("aborted on TERM signal.")
				break Loop
			case <-doneCh:
				break Loop
			}
		}
	},
}

func collectInband(ctx context.Context, cfg *app.Configuration, logger *logrus.Logger) {
	v := version.Current()
	logger.WithFields(
		logrus.Fields{
			"version": v.AppVersion,
			"commit":  v.GitCommit,
			"branch":  v.GitBranch,
		},
	).Info("Alloy collector running")

	c, err := collector.NewDeviceCollector(
		ctx,
		model.StoreKind(storeKind),
		model.AppKindInband,
		cfg,
		logger,
	)

	if err != nil {
		logger.Error(err)
		return
	}

	if err := c.CollectInband(ctx, &model.Asset{ID: assetID}, outputStdout); err != nil {
		logger.Error(err)
		return
	}

	logger.Info("collection completed successfully.")
}

// install command flags
func init() {
	cmdInband.PersistentFlags().StringVarP(&assetID, "asset-id", "", "", "The asset identifier - required when store is set to fleetdb")
	cmdInband.PersistentFlags().DurationVar(&inbandTimeout, "timeout", 1*time.Minute, "timeout inventory collection if the duration exceeds the given parameter, accepted values are int time.Duration string format - 12h, 5d...")

	rootCmd.AddCommand(cmdInband)
}
