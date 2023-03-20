package collector

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/device"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/store"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
)

var (
	ErrBiosCfgCollect   = errors.New("error collecting BIOS configuration")
	ErrInventoryCollect = errors.New("error collecting inventory data")
)

type SingleDeviceCollector struct {
	outputStdout bool
	kind         model.AppKind
	queryor      device.Queryor
	repository   store.Repository
}

func NewSingleDeviceCollector(ctx context.Context, storeKind model.StoreKind, appKind model.AppKind, cfg *app.Configuration, logger *logrus.Logger) (*SingleDeviceCollector, error) {
	repository, err := store.NewRepository(ctx, storeKind, appKind, cfg, logger)
	if err != nil {
		return nil, err
	}

	queryor, err := device.NewQueryor(appKind, logger)
	if err != nil {
		return nil, err
	}

	return &SingleDeviceCollector{
		kind:       appKind,
		queryor:    queryor,
		repository: repository,
	}, nil
}

func NewSingleDeviceCollectorWithRepository(ctx context.Context, repository store.Repository, appKind model.AppKind, logger *logrus.Logger) (*SingleDeviceCollector, error) {
	queryor, err := device.NewQueryor(appKind, logger)
	if err != nil {
		return nil, err
	}

	return &SingleDeviceCollector{
		kind:       appKind,
		queryor:    queryor,
		repository: repository,
	}, nil
}

func (c *SingleDeviceCollector) Collect(ctx context.Context, asset *model.Asset) error {
	var errs error

	// fetch existing asset information from inventory
	existing, err := c.repository.AssetByID(ctx, asset.ID, c.kind == model.AppKindOutOfBand)
	if err != nil {
		errs = multierror.Append(errs, err)
	}

	// collect inventory
	if err := c.queryor.Inventory(ctx, asset); err != nil {
		errs = multierror.Append(errs, err)
	}

	// collect BIOS configurations
	if err := c.queryor.BiosConfiguration(ctx, asset); err != nil {
		errs = multierror.Append(errs, err)

		return errs
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

	if err := c.repository.AssetUpdate(ctx, asset); err != nil {
		errs = multierror.Append(errs, err)

		return errs
	}

	return nil
}

type AssetIterCollector struct {
	concurrency   int32
	queryor       device.Queryor
	repository    store.Repository
	assetIterator AssetIterator
	syncWG        *sync.WaitGroup
	logger        *logrus.Logger
}

func NewAssetIterCollector(ctx context.Context, storeKind model.StoreKind, appKind model.AppKind, cfg *app.Configuration, syncWG *sync.WaitGroup, logger *logrus.Logger) (*AssetIterCollector, error) {
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
		concurrency:   int32(cfg.CollectorOutofband.Concurrency),
		queryor:       queryor,
		assetIterator: *assetIterator,
		repository:    repository,
		syncWG:        syncWG,
		logger:        logger,
	}, nil
}

func (d *AssetIterCollector) Collect(ctx context.Context) {
	ctx, span := tracer.Start(ctx, "Collect()")
	defer span.End()

	assetCh := d.assetIterator.AssetChannel()

	// pauser is a flag, when set will cause the asset fetcher to pause sending assets
	// on the asset channel until the flag has been cleared.
	var pauser *Pauser

	// count of routines spawned to retrieve assets
	var dispatched int32

	d.syncWG.Add(1)

	// asset fetcher routine
	go func() {
		defer d.syncWG.Done()
		d.assetIterator.IterInBatches(ctx, pauser)
	}()

	// bool set when asset fetcher routine completes
	var fetcherDone bool

	// init OOB collector
	// outofbandCollector := outofband.NewCollector(d.logger)

	// tickerCh is the interval at which the loop below checks the collector task queue size
	// and if its reached completion.
	//
	// nolint:gomnd // ticker is internal to this method and is clear as is.
	tickerCh := time.NewTicker(1 * time.Second).C

	// routines spawned by the loop below indicate on doneCh when complete.
	doneCh := make(chan struct{})

Loop:
	for {
		select {
		case <-tickerCh:
			// pause/unpause asset fetcher based on the task queue size.
			d.taskQueueWait(span, pauser, dispatched)

			// tasks dispatched were completed and the asset getter is completed.
			if dispatched == 0 && fetcherDone {
				break Loop
			}

		case <-doneCh:
			// count tasks completed
			metrics.TasksCompleted.With(metrics.StageLabelCollector).Add(1)

			atomic.AddInt32(&dispatched, ^int32(0))

		// spawn routines to collect inventory for assets
		case asset, ok := <-assetCh:
			// assetCh closed - getter completed
			if !ok {
				fetcherDone = true

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

			go func(ctx context.Context, asset *model.Asset) {
				// submit inventory collection to worker pool
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
	collector := &SingleDeviceCollector{
		queryor:    d.queryor,
		repository: d.repository,
	}

	if err := collector.Collect(ctx, asset); err != nil {
		d.logger.WithFields(logrus.Fields{
			"assetID": asset.ID,
			"err":     err.Error(),
		}).Warn("data collector error")
	}
}

// taskQueueWait sets, unsets the asset getter pause flag.
//
// This enables the DeviceQueryor to 'push back' on the asset fetcher to
// pause assets being sent on the asset channel based on the the number of queryor routines active.
//
// The asset getter pause flag is unset once the count of tasks waiting in the worker queue is below threshold levels.
func (d *AssetIterCollector) taskQueueWait(span trace.Span, pauser *Pauser, dispatched int32) {
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
		}).Trace("paused asset getter.")

		return
	}

	if pauser.Value() {
		pauser.UnPause()

		d.logger.WithFields(logrus.Fields{
			"component":   "oob collector",
			"active":      dispatched,
			"concurrency": d.concurrency,
		}).Trace("un-paused asset getter.")
	}
}
