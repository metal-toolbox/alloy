package runner

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metal-toolbox/alloy/internal/collect/outofband"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/store"
	"github.com/sirupsen/logrus"

	"go.opentelemetry.io/otel/trace"
)

type DeviceQueryor struct {
	concurrency int32
	store       store.Repository
	syncWG      *sync.WaitGroup
	logger      *logrus.Logger
}

func (d *DeviceQueryor) OutofbandCollectAll(ctx context.Context, fetcher *AssetFetcher, publisher *publisher.S) {
	ctx, span := tracer.Start(ctx, "OutofbandCollectAll()")
	defer span.End()

	// fetcher returns assets from the inventory on a channel
	fetcher := NewAssetFetcher(d.store, d.logger)

	assetCh := fetcher.AssetChannel()

	// pauser is a flag, when set will cause the asset fetcher to pause sending assets
	// on the asset channel until the flag has been cleared.
	var pauser *Pauser

	// count of routines spawned to retrieve assets
	var dispatched int32

	d.syncWG.Add(1)

	// asset fetcher routine
	go func() {
		defer d.syncWG.Done()
		fetcher.IterInBatches(ctx, pauser)
	}()

	// bool set when asset fetcher routine completes
	var fetcherDone bool

	// init OOB collector
	outofbandCollector := outofband.NewCollector(d.logger)

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

				// init collector

				outofbandCollector.CollectForAsset(ctx, asset)
				// publish
			}(ctx, asset)
		}
	}
}

// taskQueueWait sets, unsets the asset getter pause flag.
//
// This enables the DeviceQueryor to 'push back' on the asset fetcher to
// pause assets being sent on the asset channel based on the the number of queryor routines active.
//
// The asset getter pause flag is unset once the count of tasks waiting in the worker queue is below threshold levels.
func (d *DeviceQueryor) taskQueueWait(span trace.Span, pauser *Pauser, dispatched int32) {
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
