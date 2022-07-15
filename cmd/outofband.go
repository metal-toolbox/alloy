package cmd

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/asset"
	"github.com/metal-toolbox/alloy/internal/collect"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/publish"

	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type outofbandCmd struct {
	rootCmd *rootCmd

	// assets source is the cli flag for where assets are to be retrieved from.
	// supported sources: csv OR emapi
	assetSourceKind string

	// assetSourceCSVFile is required when assetSource is set to csv.
	assetSourceCSVFile string

	// assetIDList is a comma separated list of asset IDs to lookup inventory.
	assetIDList string

	// assetListAll sets up the asset getter to fetch all assets from the store.
	assetListAll bool
}

func newOutofbandCmd(rootCmd *rootCmd) *ffcli.Command {
	c := outofbandCmd{
		rootCmd: rootCmd,
	}

	fs := flag.NewFlagSet("alloy outofband", flag.ExitOnError)
	fs.StringVar(&c.assetSourceKind, "asset-source", "", "Source from where asset information are to be retrieved (csv|emapi)")
	fs.StringVar(&c.assetIDList, "list", "", "Collect inventory for the given comma separated list of asset IDs.")
	fs.BoolVar(&c.assetListAll, "all", false, "Collect inventory for all assets.")
	fs.StringVar(&c.assetSourceCSVFile, "csv-file", "", "Source assets from csv file (required when -asset-source=csv)")

	rootCmd.RegisterFlags(fs)

	return &ffcli.Command{
		Name:       "outofband",
		ShortUsage: "alloy outofband [-asset-source -list, -all] -publish-target] [-csv-file]",
		ShortHelp:  "outofband command collects asset inventory out of band",
		FlagSet:    fs,
		Exec:       c.Exec,
	}
}

func (c *outofbandCmd) Exec(ctx context.Context, _ []string) error {
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

	return c.collect(ctx, alloy, getter, publisher)
}

// initAssetGetter initializes the Asset Getter which retrieves asset information to collect inventory data.
func (c *outofbandCmd) initAssetGetter(ctx context.Context, alloy *app.App) (asset.Getter, error) {
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
		return asset.NewCSVSource(ctx, alloy, fh)

	case asset.SourceKindEMAPI:
		return asset.NewEMAPISource(ctx, alloy)
	default:
		return nil, errors.Wrap(model.ErrConfig, "unknown asset getter: "+c.assetSourceKind)
	}
}

// initAssetPublisher initializes the inventory publisher.
func (c *outofbandCmd) initAssetPublisher(ctx context.Context, alloy *app.App) (publish.Publisher, error) {
	switch c.rootCmd.publisherKind {
	case publish.KindStdout:
		return publish.NewStdoutPublisher(ctx, alloy)
	case publish.KindHollow:
		return publish.NewHollowPublisher(ctx, alloy)
	default:
		return nil, errors.Wrap(model.ErrConfig, "unknown inventory publisher: "+c.rootCmd.publisherKind)
	}
}

// validateFlags checks expected outofband flag parameters are valid
func (c *outofbandCmd) validateFlags() error {
	if err := c.validateFlagSource(); err != nil {
		return err
	}

	if err := c.validateFlagPublish(); err != nil {
		return err
	}

	return nil
}

// validateFlagSource checks the -asset-source flag parameter values are as expected.
func (c *outofbandCmd) validateFlagSource() error {
	switch c.assetSourceKind {
	case asset.SourceKindEMAPI:
		if c.rootCmd.cfgFile == "" {
			return errors.Wrap(
				model.ErrConfig,
				"-asset-source=emapi requires a valid config file parameter -config-file",
			)
		}

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
			"invalid -asset-source parameter, accepted values are csv OR emapi",
		)
	}

	return nil
}

// validateFlagPublish checks the -publish flag parameter values are as expected.
func (c *outofbandCmd) validateFlagPublish() error {
	switch c.rootCmd.publisherKind {
	case publish.KindHollow, publish.KindStdout:
		return nil
	default:
		return errors.Wrap(
			errParseCLIParam,
			"-publish parameter required, accepted values are stdout OR hollow",
		)
	}
}

// collect runs the asset getter, publisher and collects inventory out of band
func (c *outofbandCmd) collect(ctx context.Context, alloy *app.App, getter asset.Getter, publisher publish.Publisher) error {
	go func() {
		log.Println(http.ListenAndServe("localhost:9091", nil))
	}()

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
	if err := collector.Inventory(ctx); err != nil {
		alloy.Logger.WithField("err", err).Error("error running outofband collector")
	}

	alloy.Logger.Trace("collector done")

	// wait all routines are complete
	alloy.Logger.Trace("waiting for routines..")
	alloy.SyncWg.Wait()
	alloy.Logger.Trace("done..")

	return nil
}
