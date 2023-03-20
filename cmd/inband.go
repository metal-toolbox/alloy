package cmd

import (
	"log"
	"time"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/collector"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

var (
	// inbandTimeout time.Duration string value is used to timeout the inventory collection operation.
	inbandTimeout time.Duration
)

// inband inventory collection command
var cmdInband = &cobra.Command{
	Use:   "inband",
	Short: "Collect inventory data, bios configuration data on the host",
	Run: func(cmd *cobra.Command, args []string) {
		app, err := app.New(cmd.Context(), model.AppKindInband, cfgFile, logLevel)
		if err != nil {
			log.Fatal(err)
		}

		if outputStdout {
			storeKind = string(model.StoreKindMock)
		}

		c, err := collector.NewSingleDeviceCollector(
			cmd.Context(),
			model.StoreKind(storeKind),
			model.AppKindInband,
			app.Config,
			app.Logger,
		)

		if err != nil {
			log.Fatal(err)
		}

		// execution timeout
		timeoutC := time.NewTimer(inbandTimeout).C

		// setup cancel context with cancel func
		ctx, cancelFunc := context.WithCancel(cmd.Context())
		defer cancelFunc()

		asset := &model.Asset{}

		// doneCh is where the goroutine below notifies when its complete
		doneCh := make(chan struct{})

		// spawn collect routine in the background
		go func() {
			defer func() {
				if ctx.Err() == nil {
					doneCh <- struct{}{}
				}
			}()

			if err := c.Collect(ctx, asset); err != nil {
				app.Logger.Error(err)
			}
		}()

		// loop with a timeout to ensure collection does not exceed the configured timeout.
	Loop:
		for {
			select {
			case <-timeoutC:
				app.Logger.Error("aborted, timeout exceeded: " + inbandTimeout.String())
				break Loop
			case <-app.TermCh:
				app.Logger.Error("aborted on TERM signal.")
				break Loop
			case <-doneCh:
				app.Logger.Info("collection complete.")
				break Loop
			}
		}
	},
}

// install command flags
func init() {
	cmdInband.PersistentFlags().DurationVar(&inbandTimeout, "timeout", 10*time.Minute, "timeout inventory collection if the duration exceeds the given parameter, accepted values are int time.Duration string format - 12h, 5d...")

	rootCmd.AddCommand(cmdInband)
}
