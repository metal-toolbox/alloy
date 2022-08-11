package cmd

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/asset"
	"github.com/metal-toolbox/alloy/internal/collect"
	"github.com/metal-toolbox/alloy/internal/helpers"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/publish"
	"go.opentelemetry.io/otel"

	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type outOfBandCmd struct {
	rootCmd *rootCmd

	// assets source is the cli flag for where assets are to be retrieved from.
	// supported sources: csv OR serverService
	assetSourceKind string

	// assetSourceCSVFile is required when assetSource is set to csv.
	assetSourceCSVFile string

	// assetIDList is a comma separated list of asset IDs to lookup inventory.
	assetIDList string

	// collectInterval when defined runs alloy in a forever loop collecting inventory at the given interval value.
	collectInterval string

	// assetListAll sets up the asset getter to fetch all assets from the store.
	assetListAll bool

	// active is a bool flag indicating that the collector is currently running.
	active bool
}

var (
	errOutOfBandCollectInterval = errors.New("invalid collect interval")
	errCollectorActive          = errors.New("collector currently running")
)

func newOutOfBandCmd(rootCmd *rootCmd) *ffcli.Command {
	c := outOfBandCmd{
		rootCmd: rootCmd,
	}

	fs := flag.NewFlagSet("alloy outofband", flag.ExitOnError)
	fs.StringVar(&c.assetSourceKind, "asset-source", "", "Source from where asset information are to be retrieved (csv|emapi)")
	fs.StringVar(&c.assetIDList, "asset-ids", "", "Collect inventory for the given comma separated list of asset IDs.")
	fs.BoolVar(&c.assetListAll, "all", false, "Collect inventory for all assets.")
	fs.StringVar(&c.assetSourceCSVFile, "csv-file", "", "Source assets from csv file (required when -asset-source=csv)")
	fs.StringVar(&c.collectInterval, "collect-interval", "", "run as a process, collecting inventory at the given interval in the time.Duration string format - 12h, 5d...")

	rootCmd.RegisterFlags(fs)

	return &ffcli.Command{
		Name:       "outofband",
		ShortUsage: "alloy outofband [-asset-source -list, -all] -publish-target] [-csv-file]",
		ShortHelp:  "outofband command collects asset inventory out of band",
		FlagSet:    fs,
		Exec:       c.Exec,
	}
}

func (c *outOfBandCmd) Exec(ctx context.Context, _ []string) error {
	if err := c.validateFlags(); err != nil {
		return err
	}

	// default collect inventory for all assets
	if c.assetIDList == "" {
		c.assetListAll = true
	}

	// init alloy app
	alloy, err := app.New(ctx, app.KindOutOfBand, c.rootCmd.cfgFile, c.rootCmd.LogLevel())
	if err != nil {
		return err
	}

	// profiling endpoint
	if c.rootCmd.pprof {
		helpers.EnablePProfile()
	}

	// serve metrics endpoint
	metrics.ListenAndServe()

	if c.collectInterval != "" {
		return c.collectAtIntervals(ctx, alloy, c.collectInterval)
	}

	return c.collect(ctx, alloy)
}

// initAssetGetter initializes the Asset Getter which retrieves asset information to collect inventory data.
func (c *outOfBandCmd) initAssetGetter(ctx context.Context, alloy *app.App) (asset.Getter, error) {
	switch c.assetSourceKind {
	case asset.SourceKindCSV:
		if c.assetSourceCSVFile != "" {
			alloy.Config.AssetGetter.Csv.File = c.assetSourceCSVFile
		}

		if alloy.Config.AssetGetter.Csv.File == "" {
			return nil, errors.Wrap(model.ErrConfig, "csv asset source requires a csv file parameter")
		}

		fh, err := os.Open(c.assetSourceCSVFile)
		if err != nil {
			return nil, err
		}

		// init csv asset source
		return asset.NewCSVGetter(ctx, alloy, fh)

	case asset.SourceKindServerService:
		return asset.NewServerServiceGetter(ctx, alloy)
	default:
		return nil, errors.Wrap(model.ErrConfig, "unknown asset getter: "+c.assetSourceKind)
	}
}

// initAssetPublisher initializes the inventory publisher.
func (c *outOfBandCmd) initAssetPublisher(ctx context.Context, alloy *app.App) (publish.Publisher, error) {
	switch c.rootCmd.publisherKind {
	case publish.KindStdout:
		return publish.NewStdoutPublisher(ctx, alloy)
	case publish.KindServerService:
		return publish.NewServerServicePublisher(ctx, alloy)
	default:
		return nil, errors.Wrap(model.ErrConfig, "unknown inventory publisher: "+c.rootCmd.publisherKind)
	}
}

// validateFlags checks expected outofband flag parameters are valid
func (c *outOfBandCmd) validateFlags() error {
	if err := c.validateFlagSource(); err != nil {
		return err
	}

	if err := c.validateFlagPublish(); err != nil {
		return err
	}

	return nil
}

// validateFlagSource checks the -asset-source flag parameter values are as expected.
func (c *outOfBandCmd) validateFlagSource() error {
	switch c.assetSourceKind {
	case asset.SourceKindServerService:

	case asset.SourceKindCSV:
		if c.assetSourceCSVFile == "" {
			return errors.Wrap(
				errParseCLIParam,
				"-asset-source=csv requires parameter -csv-file",
			)
		}

	default:
		return errors.Wrap(
			errParseCLIParam,
			"invalid -asset-source parameter, accepted values are csv OR serverService",
		)
	}

	return nil
}

// validateFlagPublish checks the -publish flag parameter values are as expected.
func (c *outOfBandCmd) validateFlagPublish() error {
	switch c.rootCmd.publisherKind {
	case publish.KindServerService, publish.KindStdout:
		return nil
	default:
		return errors.Wrap(
			errParseCLIParam,
			"-publish parameter required, accepted values are stdout OR serverService",
		)
	}
}

func (c *outOfBandCmd) collectAtIntervals(ctx context.Context, alloy *app.App, interval string) error {
	tInterval, err := time.ParseDuration(interval)
	if err != nil {
		return errors.Wrap(errOutOfBandCollectInterval, err.Error())
	}

	if tInterval < time.Duration(1*time.Minute) {
		return errors.Wrap(errOutOfBandCollectInterval, "minimum collect interval is 1m")
	}

	alloy.Logger.Info("inventory collection scheduled at interval: " + interval)

Loop:
	for {
		select {
		case <-time.NewTicker(tInterval).C:
			if c.active {
				return errors.Wrap(errCollectorActive, "skipped invocation")
			}

			go func() {
				// set active flag to indicate the collector is currently active
				c.active = true
				defer func() { c.active = false }()

				err := c.collect(ctx, alloy)
				if err != nil {
					alloy.Logger.Warn(err)
				}
			}()
		case <-alloy.TermCh:
			break Loop
		}
	}

	// wait until all routines are complete
	alloy.SyncWg.Wait()

	return nil
}

// collect runs the asset getter, publisher and collects inventory out of band
func (c *outOfBandCmd) collect(ctx context.Context, alloy *app.App) error {
	alloy.Logger.Trace("collector spawned.")

	// trace
	tracer := otel.Tracer("alloy")
	ctx, span := tracer.Start(ctx, "collect")

	defer span.End()

	// init collector channels
	alloy.InitAssetCollectorChannels()

	// init asset getter
	getter, err := c.initAssetGetter(ctx, alloy)
	if err != nil {
		return err
	}

	// init asset publisher
	publisher, err := c.initAssetPublisher(ctx, alloy)
	if err != nil {
		return err
	}

	// setup cancel context with cancel func
	ctx, cancelFunc := context.WithCancel(ctx)

	// spawn asset getter as a routine
	alloy.SyncWg.Add(1)

	go func() {
		defer alloy.SyncWg.Done()

		if c.assetIDList != "" {
			if err := getter.ListByIDs(ctx, strings.Split(c.assetIDList, ",")); err != nil {
				alloy.Logger.WithField("err", err).Error("error running asset getter routine")
				cancelFunc()
			}
		} else {
			if err := getter.ListAll(ctx); err != nil {
				alloy.Logger.WithField("err", err).Error("error running asset getter routine")
				cancelFunc()
			}
		}

		alloy.Logger.Trace("getter done")
	}()

	// spawn publisher as a routine
	alloy.SyncWg.Add(1)

	go func() {
		defer alloy.SyncWg.Done()

		if err := publisher.Run(ctx); err != nil {
			alloy.Logger.WithField("err", err).Error("error running inventory publisher routine")
			cancelFunc()
		}

		alloy.Logger.Trace("publisher done")
	}()

	// routine listens for termination signal
	go func() {
		<-alloy.TermCh
		cancelFunc()
	}()

	// spawn out of band collector as a routine
	collector := collect.NewOutOfBandCollector(alloy)
	if err := collector.InventoryRemote(ctx); err != nil {
		alloy.Logger.WithField("err", err).Error("error running outofband collector")
	}

	alloy.Logger.Trace("collector done")

	// wait all routines are complete
	alloy.Logger.Trace("waiting for routines..")
	alloy.SyncWg.Wait()
	alloy.Logger.Trace("done..")

	return nil
}
