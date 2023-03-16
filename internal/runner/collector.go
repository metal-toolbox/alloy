package runner

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
)

// InventoryRemote iterates over assets received on the asset channel
// and collects inventory out-of-band (remotely) for the assets received,
// the collected inventory is then sent over the collector channel to the publisher.
//
// This method returns after all the routines it dispatched (to the worker pool) have returned.
//
// RunInventoryCollect implements the Collector interface.
//
// nolint:gocyclo // this method is better not split up in its current form.
func (o *DeviceQueryor) IterOutofband(ctx context.Context, assetCh chan<- *model.Asset) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "IterOutofband()")
	defer span.End()

	// close collectorCh to notify consumers
	//defer close(o.collectorCh)

	// channel for routines spawned to indicate completion
	doneCh := make(chan struct{})

	// count of routines spawned to retrieve assets
	var dispatched int32

	var getterCompleted bool

	// tickerCh is the interval at which the loop below checks the collector task queue size
	// and if its reached completion.
	//
	// nolint:gomnd // ticker is internal to this method and is clear as is.
	tickerCh := time.NewTicker(1 * time.Second).C

Loop:
	for {
		select {
		case <-tickerCh:
			// pause/unpause asset getter based on the task queue size.
			o.taskQueueWait(span)

			// tasks dispatched were completed and the asset getter is completed.
			if dispatched == 0 && getterCompleted {
				break Loop
			}

		case <-doneCh:
			// count tasks completed
			metrics.TasksCompleted.With(metrics.StageLabelCollector).Add(1)

			atomic.AddInt32(&dispatched, ^int32(0))

		// spawn routines to collect inventory for assets
		case asset, ok := <-o.assetCh:
			// assetCh closed - getter completed
			if !ok {
				getterCompleted = true

				continue
			}

			if asset == nil {
				continue
			}

			// count assets received on the asset channel
			metrics.AssetsReceived.With(metrics.StageLabelCollector).Inc()

			// increment wait group
			o.syncWg.Add(1)

			// increment spawned count
			atomic.AddInt32(&dispatched, 1)

			func(ctx context.Context, target *model.Asset) {
				// submit inventory collection to worker pool
				o.workers.Submit(
					func() {
						defer o.syncWg.Done()
						defer func() {
							doneCh <- struct{}{}
						}()

						// count dispatched worker task
						metrics.TasksDispatched.With(metrics.StageLabelCollector).Add(1)

						o.CollectForAsset(ctx, target)
						o.collectorCh <- target
					},
				)
			}(ctx, asset)
		}
	}

	return nil
}
