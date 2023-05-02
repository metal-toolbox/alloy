package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/device"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/store"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var (
	ErrBiosCfgCollect   = errors.New("error collecting BIOS configuration")
	ErrInventoryCollect = errors.New("error collecting inventory data")
)

func init() {

}

// DeviceCollector holds attributes to collect inventory, bios configuration data from a single device.
type DeviceCollector struct {
	queryor    device.Queryor
	repository store.Repository
	kind       model.AppKind
}

// NewDeviceCollector is a constructor method to return a inventory, bios configuration data collector.
func NewDeviceCollector(ctx context.Context, storeKind model.StoreKind, appKind model.AppKind, cfg *app.Configuration, logger *logrus.Logger) (*DeviceCollector, error) {
	repository, err := store.NewRepository(ctx, storeKind, appKind, cfg, logger)
	if err != nil {
		return nil, err
	}

	queryor, err := device.NewQueryor(appKind, logger)
	if err != nil {
		return nil, err
	}

	return &DeviceCollector{
		kind:       appKind,
		queryor:    queryor,
		repository: repository,
	}, nil
}

// NewDeviceCollectorWithStore is a constructor method that accepts an initialized store repository - to return a inventory, bios configuration data collector.
func NewDeviceCollectorWithStore(repository store.Repository, appKind model.AppKind, logger *logrus.Logger) (*DeviceCollector, error) {
	queryor, err := device.NewQueryor(appKind, logger)
	if err != nil {
		return nil, err
	}

	return &DeviceCollector{
		kind:       appKind,
		queryor:    queryor,
		repository: repository,
	}, nil
}

// CollectOutofband querys inventory and bios configuration data for a device through its BMC.
func (c *DeviceCollector) CollectOutofband(ctx context.Context, asset *model.Asset, outputStdout bool) error {
	var errs error

	// fetch existing asset information from inventory
	existing, err := c.repository.AssetByID(ctx, asset.ID, true)
	if err != nil {
		errs = multierror.Append(errs, err)

		return errs
	}

	if existing == nil {
		return errors.Wrap(ErrInventoryCollect, "asset not found in store with required attributes")
	}

	// copy over attributes required for outofband collection
	asset.BMCAddress = existing.BMCAddress
	asset.BMCPassword = existing.BMCPassword
	asset.BMCUsername = existing.BMCUsername
	asset.Facility = existing.Facility
	asset.Errors = make(map[string]string)

	// collect inventory
	if errInventory := c.queryor.Inventory(ctx, asset); errInventory != nil {
		errs = multierror.Append(errs, errInventory)
	}

	// collect BIOS configurations
	if errBiosCfg := c.queryor.BiosConfiguration(ctx, asset); errBiosCfg != nil {
		errs = multierror.Append(errs, errBiosCfg)
	}

	// set collected inventory attributes based on inventory data
	// so as to not overwrite any of these existing values when published.
	if existing.Model != "" {
		asset.Model = existing.Model
	}

	if existing.Vendor != "" {
		asset.Vendor = existing.Vendor
	}

	if existing.Serial != "" {
		asset.Serial = existing.Serial
	}

	if outputStdout {
		if err != nil {
			return err
		}

		return c.prettyPrintJSON(asset)
	}

	if err := c.repository.AssetUpdate(ctx, asset); err != nil {
		errs = multierror.Append(errs, err)

		return errs
	}

	return nil
}

// CollectInband querys inventory and bios configuration data for a device through the host OS
// this expects Alloy is running within the alloy-inband docker image based on ironlib.
func (c *DeviceCollector) CollectInband(ctx context.Context, asset *model.Asset, outputStdout bool) error {
	var errs error

	// fetch existing asset information from inventory
	existing, err := c.repository.AssetByID(ctx, asset.ID, c.kind == model.AppKindOutOfBand)
	if err != nil {
		errs = multierror.Append(errs, err)
	}

	// collect inventory
	if errInventory := c.queryor.Inventory(ctx, asset); errInventory != nil {
		errs = multierror.Append(errs, errInventory)
	}

	// collect BIOS configurations
	if errBiosCfg := c.queryor.BiosConfiguration(ctx, asset); errBiosCfg != nil {
		errs = multierror.Append(errs, errBiosCfg)
	}

	if existing != nil {
		// set collected inventory attributes based on inventory data
		// so as to not overwrite any of these existing values when published.
		if existing.Model != "" {
			asset.Model = existing.Model
		}

		if existing.Vendor != "" {
			asset.Vendor = existing.Vendor
		}

		if existing.Serial != "" {
			asset.Serial = existing.Serial
		}
	}

	asset.Errors = make(map[string]string)

	if outputStdout {
		if err != nil {
			return err
		}

		return c.prettyPrintJSON(asset)
	}

	if err := c.repository.AssetUpdate(ctx, asset); err != nil {
		errs = multierror.Append(errs, err)

		return errs
	}

	return nil
}

func (c *DeviceCollector) prettyPrintJSON(asset *model.Asset) error {
	b, err := json.MarshalIndent(asset, "", " ")
	if err != nil {
		return err
	}

	fmt.Print(string(b))

	return nil
}

// AssetIterCollector holds attributes to iterate over the assets in the store repository and collect
// inventory, bios configuration for them remotely.
type AssetIterCollector struct {
	assetIterator AssetIterator
	queryor       device.Queryor
	repository    store.Repository
	syncWG        *sync.WaitGroup
	logger        *logrus.Logger
	concurrency   int32
}

// NewAssetIterCollector is a constructor method that returns an AssetIterCollector.
func NewAssetIterCollector(
	ctx context.Context,
	storeKind model.StoreKind,
	appKind model.AppKind,
	cfg *app.Configuration,
	syncWG *sync.WaitGroup,
	logger *logrus.Logger,
) (*AssetIterCollector, error) {
	repository, err := store.NewRepository(ctx, storeKind, appKind, cfg, logger)
	if err != nil {
		return nil, err
	}

	queryor, err := device.NewQueryor(appKind, logger)
	if err != nil {
		return nil, err
	}

	assetIterator := NewAssetIterator(repository, logger)

	return &AssetIterCollector{
		concurrency:   int32(cfg.Concurrency),
		queryor:       queryor,
		assetIterator: *assetIterator,
		repository:    repository,
		syncWG:        syncWG,
		logger:        logger,
	}, nil
}

// NewAssetIterCollectorWithStore is a constructor method that accepts an initialized store to return an AssetIterCollector.
func NewAssetIterCollectorWithStore(
	appKind model.AppKind,
	repository store.Repository,
	concurrency int32,
	syncWG *sync.WaitGroup,
	logger *logrus.Logger,
) (*AssetIterCollector, error) {
	queryor, err := device.NewQueryor(appKind, logger)
	if err != nil {
		return nil, err
	}

	assetIterator := NewAssetIterator(repository, logger)

	return &AssetIterCollector{
		concurrency:   concurrency,
		queryor:       queryor,
		assetIterator: *assetIterator,
		repository:    repository,
		syncWG:        syncWG,
		logger:        logger,
	}, nil
}

// Collect iterates over assets returned by the AssetIterator and collects their inventory, bios configuration data.
func (d *AssetIterCollector) Collect(ctx context.Context) {
	tracer := otel.Tracer("collector.AssetIteratorCollector")
	ctx, span := tracer.Start(context.TODO(), "Collect()")

	defer span.End()

	// pauser helps throttle asset retrieval to match the data collection rate.
	pauser := NewPauser()

	// count of routines spawned to retrieve assets
	var dispatched int32

	d.syncWG.Add(1)

	// asset fetcher routine
	go func() {
		defer d.syncWG.Done()
		d.assetIterator.IterInBatches(ctx, int(d.concurrency), pauser)
	}()

	// bool set when asset iterator closes its channel.
	var done bool

	// interval to check collection completion
	var checkCompletionInterval = 1 * time.Second

	tickerCheckComplete := time.NewTicker(checkCompletionInterval)
	defer tickerCheckComplete.Stop()

	// routines spawned by the loop below indicate on doneCh when complete.
	doneCh := make(chan struct{})

Loop:
	for {
		select {
		case <-tickerCheckComplete.C:

			// tasks dispatched were completed and the asset getter is completed.
			if dispatched == 0 && done {
				break Loop
			}

		case <-doneCh:
			// count tasks completed
			metrics.TasksCompleted.With(metrics.StageLabelCollector).Add(1)

			atomic.AddInt32(&dispatched, ^int32(0))

		// spawn routines to collect inventory for assets
		case asset, ok := <-d.assetIterator.Channel():
			// assetCh closed - iterator returned.
			if !ok {
				done = true

				continue
			}

			if asset == nil {
				continue
			}

			// count assets received on the asset channel
			metrics.AssetsReceived.With(metrics.StageLabelCollector).Inc()

			// increment wait group
			d.syncWG.Add(1)

			// increment spawned count
			atomic.AddInt32(&dispatched, 1)

			// throttle asset iterator based on dispatched vs concurrency limit
			d.throttle(span, pauser, dispatched)

			// run collection in routine
			go func(ctx context.Context, asset *model.Asset) {
				defer d.syncWG.Done()
				defer func() {
					doneCh <- struct{}{}
				}()

				// count dispatched worker task
				metrics.TasksDispatched.With(metrics.StageLabelCollector).Add(1)

				d.collect(ctx, asset)
			}(ctx, asset)
		}
	}
}

func (d *AssetIterCollector) collect(ctx context.Context, asset *model.Asset) {
	collector := &DeviceCollector{
		queryor:    d.queryor,
		repository: d.repository,
	}

	d.logger.WithFields(
		logrus.Fields{
			"assetID": asset.ID,
			"BMC":     asset.BMCAddress,
		},
	).Debug("collecting data for asset")

	if err := collector.CollectOutofband(ctx, asset, false); err != nil {
		d.logger.WithFields(logrus.Fields{
			"assetID": asset.ID,
			"err":     err.Error(),
		}).Warn("data collector error")
	}

	d.logger.WithFields(
		logrus.Fields{
			"assetID": asset.ID,
			"BMC":     asset.BMCAddress,
		},
	).Debug("collection complete.")
}

// throttle allows this collector to to 'push back' on the asset iterator
// to throttle assets being sent based on the routines dispatched and the configured concurrency value.
func (d *AssetIterCollector) throttle(_ trace.Span, pauser *Pauser, dispatched int32) {
	// measure tasks waiting queue size
	metrics.TaskQueueSize.With(metrics.StageLabelCollector).Set(float64(dispatched))

	if dispatched > d.concurrency {
		if pauser.Value() {
			// fetcher was previously paused
			return
		}

		pauser.Pause()

		d.logger.WithFields(logrus.Fields{
			"component":   "oob collector",
			"active":      dispatched,
			"concurrency": d.concurrency,
		}).Trace("paused asset iterator.")

		return
	}

	if pauser.Value() {
		pauser.UnPause()

		d.logger.WithFields(logrus.Fields{
			"component":   "oob collector",
			"active":      dispatched,
			"concurrency": d.concurrency,
		}).Trace("resumed asset iterator.")
	}
}
