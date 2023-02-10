package cmd

import (
	"flag"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/asset"
	"github.com/metal-toolbox/alloy/internal/collect"
	"github.com/metal-toolbox/alloy/internal/helpers"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/publish"
	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type inbandCmd struct {
	rootCmd *rootCmd

	// assetID when specified is assigned to the device
	// it is required when publishing to server service.
	assetID string

	// assets source is the cli flag for where assets are to be retrieved from.
	// supported sources: csv OR serverService
	assetSourceKind string

	// assetSourceCSVFile is required when assetSource is set to csv.
	assetSourceCSVFile string

	// timeout time.Duration string value is used to timeout the inventory collection operation.
	timeout string
}

var (
	errAssetID = errors.New("asset ID invalid or not specified")
)

func newInbandCmd(rootCmd *rootCmd) *ffcli.Command {
	c := inbandCmd{
		rootCmd: rootCmd,
	}

	fs := flag.NewFlagSet("alloy inband", flag.ExitOnError)
	fs.StringVar(&c.assetID, "asset-id", "", "The inventory asset identifier - required when publishing to server service.")
	fs.StringVar(&c.timeout, "timeout", "10m", "timeout inventory collection if the duration exceeds the given parameter, accepted values are int time.Duration string format - 12h, 5d...")
	fs.StringVar(&c.assetSourceKind, "asset-source", "", "Source from where asset information are to be retrieved (csv|serverService)")

	rootCmd.RegisterFlags(fs)

	return &ffcli.Command{
		Name:       "inband",
		ShortUsage: "alloy inband -asset-id <> -publish-target [stdout|serverService]",
		ShortHelp:  "inband command runs on target hardware to collect inventory inband",
		FlagSet:    fs,
		Exec:       c.Exec,
	}
}

// Exec runs the inband collector command
//
// nolint:gocyclo // for now its ideal to have all the initialization in one method.
//
//	TODO: refactor into separate methods for further changes.
func (i *inbandCmd) Exec(ctx context.Context, _ []string) error {
	if i.rootCmd.pprof {
		helpers.EnablePProfile()
	}

	// server service publisher target requires a valid asset ID
	if i.rootCmd.publisherKind == publish.KindServerService {
		if i.assetID == "" {
			return errors.Wrap(errAssetID, "-asset-id <id> is required for the server service publisher target")
		}

		_, err := uuid.Parse(i.assetID)
		if err != nil {
			return errors.Wrap(errAssetID, err.Error())
		}
	}

	alloy, err := app.New(ctx, model.AppKindInband, i.rootCmd.cfgFile, i.rootCmd.LogLevel())
	if err != nil {
		return err
	}

	var inventoryAsset *model.Asset

	// init asset getter when --asset-source is specified
	if i.assetSourceKind != "" {
		var getter asset.Getter

		getter, err = i.initAssetGetter(ctx, alloy)
		if err != nil {
			return err
		}

		inventoryAsset, err = getter.AssetByID(ctx, i.assetID, false)
		if err != nil {
			return errors.Wrap(err, "inventory lookup for asset failed, asset ID: "+i.assetID)
		}
	}

	// init publisher
	publisher, err := i.initAssetPublisher(ctx, alloy)
	if err != nil {
		return err
	}

	// init collector
	collector := collect.NewInbandCollector(alloy)

	timeoutDuration, err := time.ParseDuration(i.timeout)
	if err != nil {
		return err
	}

	// execution timeout
	timeoutC := time.NewTimer(timeoutDuration).C

	// setup cancel context with cancel func
	ctx, cancelFunc := context.WithCancel(ctx)

	// doneCh is where the collect publish routines notify when complete.
	doneCh := make(chan struct{})

	// collect and publish routine
	go func() {
		defer func() {
			if ctx.Err() == nil {
				doneCh <- struct{}{}
			}
		}()

		collected, err := collector.InventoryLocal(ctx)
		if err != nil {
			alloy.Logger.Error(err)
			return
		}

		collected.ID = i.assetID

		// set collected inventory attributes based on inventory data
		// so as to not overwrite any of these existing values when published.
		if inventoryAsset != nil {
			if inventoryAsset.Model != "" {
				collected.Model = inventoryAsset.Model
			}

			if inventoryAsset.Vendor != "" {
				collected.Vendor = inventoryAsset.Vendor
			}

			if inventoryAsset.Serial != "" {
				collected.Serial = inventoryAsset.Serial
			}
		}

		err = publisher.PublishOne(ctx, collected)
		if err != nil {
			alloy.Logger.Error(err)
		}
	}()

	// loop with a timeout to ensure collection does not exceed the configured timeout.
Loop:
	for {
		select {
		case <-timeoutC:
			alloy.Logger.Error("aborted, timeout exceeded: " + i.timeout)
			break Loop
		case <-alloy.TermCh:
			alloy.Logger.Error("aborted on TERM signal.")
			cancelFunc()
			break Loop
		case <-doneCh:
			alloy.Logger.Info("collect and publish routines done.")
			break Loop
		}
	}

	return nil
}

// initAssetPublisher initializes the inventory publisher.
func (i *inbandCmd) initAssetPublisher(ctx context.Context, alloy *app.App) (publish.Publisher, error) {
	switch i.rootCmd.publisherKind {
	case publish.KindStdout:
		return publish.NewStdoutPublisher(ctx, alloy)
	case publish.KindServerService:
		if i.assetSourceKind != asset.SourceKindServerService {
			// To ensure the asset data in serverService is looked up before publishing data to serverService.
			return nil, errors.Wrap(model.ErrConfig, "serverService publisher requires --asset-source serverService")
		}

		return publish.NewServerServicePublisher(ctx, alloy)

	default:
		return nil, errors.Wrap(model.ErrConfig, "unknown inventory publisher: "+i.rootCmd.publisherKind)
	}
}

// initAssetGetter initializes the Asset Getter which retrieves asset information to collect inventory data.
func (i *inbandCmd) initAssetGetter(ctx context.Context, alloy *app.App) (asset.Getter, error) {
	switch i.assetSourceKind {
	case asset.SourceKindCSV:
		if i.assetSourceCSVFile != "" {
			alloy.Config.AssetGetter.Csv.File = i.assetSourceCSVFile
		}

		if alloy.Config.AssetGetter.Csv.File == "" {
			return nil, errors.Wrap(model.ErrConfig, "csv asset source requires a csv file parameter")
		}

		fh, err := os.Open(i.assetSourceCSVFile)
		if err != nil {
			return nil, err
		}

		// init csv asset source
		return asset.NewCSVGetter(ctx, alloy, fh)

	case asset.SourceKindServerService:
		return asset.NewServerServiceGetter(ctx, alloy)
	default:
		return nil, errors.Wrap(model.ErrConfig, "unknown asset getter: "+i.assetSourceKind)
	}
}
