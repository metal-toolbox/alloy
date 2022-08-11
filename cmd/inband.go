package cmd

import (
	"flag"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/collect"
	"github.com/metal-toolbox/alloy/internal/helpers"
	"github.com/metal-toolbox/alloy/internal/publish"
	"github.com/peterbourgon/ff/v3/ffcli"
	"golang.org/x/net/context"
)

type inbandCmd struct {
	rootCmd *rootCmd
}

func newInbandCmd(rootCmd *rootCmd) *ffcli.Command {
	c := inbandCmd{
		rootCmd: rootCmd,
	}

	fs := flag.NewFlagSet("alloy inband", flag.ExitOnError)
	rootCmd.RegisterFlags(fs)

	return &ffcli.Command{
		Name:       "inband",
		ShortUsage: "alloy inband -publish-target",
		ShortHelp:  "inband command runs on target hardware to collect inventory inband",
		FlagSet:    fs,
		Exec:       c.Exec,
	}
}

// Exec runs the inband collector command
//
// nolint:gocyclo // for now its ideal to have all the initialization in one method,
//                   this will be refactored if theres going to be further changes.
func (i *inbandCmd) Exec(ctx context.Context, _ []string) error {
	if i.rootCmd.pprof {
		helpers.EnablePProfile()
	}

	alloy, err := app.New(ctx, app.KindOutOfBand, c.rootCmd.cfgFile, c.rootCmd.LogLevel())
	if err != nil {
		return err
	}

	// setup cancel context with cancel func
	ctx, cancelFunc := context.WithCancel(ctx)

	publisher, err := publish.NewStdoutPublisher(ctx, alloy)
	if err != nil {
		return err
	}

	// spawn publisher as a routine
	alloy.SyncWg.Add(1)

	go func() {
		defer alloy.SyncWg.Done()

		if err := publisher.Run(ctx); err != nil {
			alloy.Logger.WithField("err", err).Error("error running inventory publisher routine")
			cancelFunc()
		}

		alloy.Logger.Trace("publisher routine returned")
	}()

	// routine listens for termination signal
	go func() {
		<-alloy.TermCh
		cancelFunc()
	}()

	// spawn out of band collector as a routine
	collector := collect.NewInbandCollector(alloy)
	if err := collector.Inventory(ctx); err != nil {
		alloy.Logger.WithField("err", err).Error("error running inband collector")
	}

	alloy.Logger.Trace("collector routine returned")

	// wait all routines are complete
	alloy.Logger.Trace("waiting for any other running routines..")
	alloy.SyncWg.Wait()
	alloy.Logger.Trace("done..")

	return nil
}
