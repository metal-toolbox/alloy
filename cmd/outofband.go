package cmd

import (
	"flag"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/asset"
	"github.com/metal-toolbox/alloy/internal/collect"
	"github.com/metal-toolbox/alloy/internal/helpers"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/publish"
	"github.com/prometheus/client_golang/prometheus"
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

	// collectSplay when set adds additional time to the collectInterval
	// the additional time added is between zero and the collectSplay time.Duration value
	collectSplay string

	// assetListAll sets up the asset getter to fetch all assets from the store.
	assetListAll bool

	// active is a bool flag indicating that the collector is currently running.
	active bool
}

var (
	errOutOfBandCollectInterval = errors.New("invalid collect interval")
)

func newOutOfBandCmd(rootCmd *rootCmd) *ffcli.Command {
	c := outOfBandCmd{
		rootCmd: rootCmd,
	}

	fs := flag.NewFlagSet("alloy outofband", flag.ExitOnError)
	fs.StringVar(&c.assetSourceKind, "asset-source", "", "Source from where asset information are to be retrieved (csv|serverService)")
	fs.StringVar(&c.assetIDList, "asset-ids", "", "Collect inventory for the given comma separated list of asset IDs.")
	fs.BoolVar(&c.assetListAll, "all", false, "Collect inventory for all assets.")
	fs.StringVar(&c.assetSourceCSVFile, "csv-file", "", "Source assets from csv file (required when -asset-source=csv)")
	fs.StringVar(&c.collectInterval, "collect-interval", "", "run as a process, collecting inventory at the given interval in the time.Duration string format - 12h, 5d...")
	fs.StringVar(&c.collectSplay, "collect-splay", "0s", "")

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
	alloy, err := app.New(ctx, model.AppKindOutOfBand, c.rootCmd.cfgFile, c.rootCmd.LogLevel())
	if err != nil {
		return err
	}

	// profiling endpoint
	if c.rootCmd.pprof {
		helpers.EnablePProfile()
	}

	// serve metrics endpoint
	metrics.ListenAndServe()

	// setup cancel context with cancel func
	ctx, cancelFunc := context.WithCancel(ctx)

	// routine listens for termination signal and cancels the context
	go func() {
		<-alloy.TermCh
		cancelFunc()
	}()

	// collect interval parameter was specified
	if c.collectInterval != "" {
		// parse collect interval
		interval, err := time.ParseDuration(c.collectInterval)
		if err != nil {
			return errors.Wrap(errOutOfBandCollectInterval, err.Error())
		}

		if interval < time.Duration(1*time.Minute) {
			return errors.Wrap(errOutOfBandCollectInterval, "minimum collect interval is 1m")
		}

		// collection interval to be randomized based on splay value
		if c.collectSplay != "0s" {
			// parse splay interval
			splay, err := time.ParseDuration(c.collectSplay)
			if err != nil {
				return errors.Wrap(errOutOfBandCollectInterval, err.Error())
			}

			// randomize to given splay value and add to interval
			rand.Seed(time.Now().UnixNano())

			// nolint:gosec // Ideally this should be using crypto/rand,
			//                 although the generated random value here is just used to add jitter/splay to
			//                 the interval value and is not used outside of this context.
			interval += time.Duration(rand.Int63n(int64(splay)))
		}

		return c.collectAtIntervals(ctx, alloy, interval)
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

func (c *outOfBandCmd) collectAtIntervals(ctx context.Context, alloy *app.App, interval time.Duration) error {
	// register for SIGHUP
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGHUP)

	collectFunc := func() {
		// set active flag to indicate the collector is currently active
		c.active = true

		// set 1 to indicate activity
		metrics.OOBCollectionActive.Set(1)

		defer func() {
			c.active = false

			metrics.OOBCollectionActive.Set(0)
		}()

		// start measure total collection time
		startTS := time.Now()

		err := c.collect(ctx, alloy)
		if err != nil {
			alloy.Logger.Warn(err)
		}

		// measure total collection tim
		metrics.CollectTotalTimeSummary.With(
			prometheus.Labels{"collect_kind": model.AppKindOutOfBand},
		).Observe(time.Since(startTS).Seconds())

		// update next scheduled alloy run
		metrics.OOBCollectScheduleTimestamp.With(
			prometheus.Labels{"timestamp": "true"},
		).Set(float64(time.Now().Add(interval).Unix()))
	}

	alloy.Logger.Infof(
		"inventory collection scheduled at interval: %s, next collection: %s",
		interval.String(),
		time.Now().Add(interval),
	)

	// set next scheduled alloy run
	metrics.OOBCollectScheduleTimestamp.With(
		prometheus.Labels{"timestamp": "true"},
	).Set(float64(time.Now().Add(interval).Unix()))

Loop:
	for {
		select {
		case <-time.NewTicker(interval).C:
			if c.active {
				continue
			}

			collectFunc()
		case <-signalCh:
			if c.active {
				continue
			}

			alloy.Logger.Info("SIGHUP received, running oob inventory collection..")
			collectFunc()

		case <-ctx.Done():
			alloy.Logger.Info("got cancel signal, wait for spawned routines to complete...")
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

	// spawn asset getter as a routine
	alloy.SyncWg.Add(1)

	go func() {
		defer alloy.SyncWg.Done()

		if c.assetIDList != "" {
			if err := getter.ListByIDs(ctx, strings.Split(c.assetIDList, ",")); err != nil {
				alloy.Logger.WithField("err", err).Error("error running asset getter routine")
				return
			}
		} else {
			if err := getter.ListAll(ctx); err != nil {
				alloy.Logger.WithField("err", err).Error("error running asset getter routine")
				return
			}
		}

		alloy.Logger.Trace("getter done")
	}()

	// spawn publisher as a routine
	alloy.SyncWg.Add(1)

	go func() {
		defer alloy.SyncWg.Done()

		if err := publisher.RunInventoryPublisher(ctx); err != nil {
			alloy.Logger.WithField("err", err).Error("error running inventory publisher routine")
			return
		}

		alloy.Logger.Trace("publisher done")
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
