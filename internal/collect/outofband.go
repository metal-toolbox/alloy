package collect

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"time"

	bmclibv2 "github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/common"
	logrusrv2 "github.com/bombsimon/logrusr/v2"
	"github.com/gammazero/workerpool"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

const (
	// concurrency is the default number of workers to concurrently query BMCs
	concurrency = 10
)

// OutOfBand collector collects hardware, firmware inventory out of band
type OutOfBandCollector struct {
	mockClient  oobGetter
	logger      *logrus.Entry
	config      *model.Config
	syncWg      *sync.WaitGroup
	assetCh     <-chan *model.Asset
	termCh      <-chan os.Signal
	collectorCh chan<- *model.AssetDevice
	workers     workerpool.WorkerPool
}

// oobGetter interface defines methods that the bmclib client exposes
// this is mainly to swap the bmclib instance for tests
type oobGetter interface {
	Open(ctx context.Context) error
	Close(ctx context.Context) error
	Inventory(ctx context.Context) (*common.Device, error)
}

// NewOutOfBandCollector returns a instance of the OutOfBandCollector inventory collector
func NewOutOfBandCollector(alloy *app.App) Collector {
	logger := app.NewLogrusEntryFromLogger(logrus.Fields{"component": "collector.outofband"}, alloy.Logger)

	c := &OutOfBandCollector{
		logger:      logger,
		assetCh:     alloy.AssetCh,
		termCh:      alloy.TermCh,
		syncWg:      alloy.SyncWg,
		config:      alloy.Config,
		collectorCh: alloy.CollectorCh,
		workers:     *workerpool.New(concurrency),
	}

	if c.config.CollectorOutofband.Concurrency == 0 {
		c.config.CollectorOutofband.Concurrency = concurrency
	}

	return c
}

func (o *OutOfBandCollector) SetMockGetter(getter interface{}) {
	o.mockClient = getter.(oobGetter)
}

// Inventory implements the Collector interface to collect inventory inband
func (o *OutOfBandCollector) Inventory(ctx context.Context) error {
	// close collectorCh to notify consumers
	defer close(o.collectorCh)

	// channel for routines spawned to indicate completion
	doneCh := make(chan struct{})

	// count of routines spawned to retrieve assets
	var dispatched int32

	// spawn routines to collect inventory for assets
	for asset := range o.assetCh {
		if asset == nil {
			continue
		}

		// count assets received on the asset channel
		metrics.AssetsReceived.With(stageLabel).Inc()

		// increment wait group
		o.syncWg.Add(1)

		// increment spawned count
		atomic.AddInt32(&dispatched, 1)

		// measure tasks waiting queue size
		metrics.TaskQueueSize.With(stageLabel).Set(float64(o.workers.WaitingQueueSize()))

		for o.workers.WaitingQueueSize() > concurrency {
			if ctx.Err() != nil {
				break
			}

			o.logger.WithFields(logrus.Fields{
				"component":   "oob collector",
				"queue size":  o.workers.WaitingQueueSize(),
				"concurrency": concurrency,
			}).Debug("delay for queue size to drop..")

			// nolint:gomnd // delay is a magic number
			time.Sleep(5 * time.Second)

			// measure tasks waiting queue size, when this loop is active
			metrics.TaskQueueSize.With(stageLabel).Set(float64(o.workers.WaitingQueueSize()))
		}

		// context canceled
		if ctx.Err() != nil {
			break
		}

		// submit inventory collection to worker pool
		o.workers.Submit(
			func() {
				defer o.syncWg.Done()
				defer func() { doneCh <- struct{}{} }()

				// count dispatched worker task
				metrics.TasksDispatched.With(stageLabel).Add(1)

				o.spawn(ctx, asset)
			},
		)
	}

	// wait for dispatched routines to complete
	for dispatched > 0 {
		<-doneCh

		// count tasks completed
		metrics.TasksCompleted.With(stageLabel).Add(1)

		atomic.AddInt32(&dispatched, ^int32(0))
	}

	return nil
}

// spawn runs the asset inventory collection and writes the collected inventory to the collectorCh
func (o *OutOfBandCollector) spawn(ctx context.Context, asset *model.Asset) {
	// bmc is the bmc client instance
	var bmc oobGetter

	o.logger.WithFields(
		logrus.Fields{
			"IP": asset.BMCAddress.String(),
		}).Trace("collecting inventory for asset..")

	if o.mockClient == nil {
		bmc = newBMCClient(
			ctx,
			asset,
			o.logger.Logger,
		)
	} else {
		// mock client for tests
		bmc = o.mockClient
	}

	// measure BMC connection open
	startTS := time.Now()

	if err := bmc.Open(ctx); err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"IP":  asset.BMCAddress.String(),
				"err": err,
			}).Warn("error in bmc connection open")

		span.SetStatus(codes.Error, " BMC connection: "+err.Error())

		// count connection open error metric
		metricBMCQueryErrorCount.With(
			metrics.AddLabels(
				stageLabel,
				prometheus.Labels{
					"query_kind": "conn_open",
					"vendor":     asset.Vendor,
					"model":      asset.Model,
				}),
		).Inc()

		return
	}

	// measure BMC connection open
	metricBMCQueryTimeSummary.With(
		metrics.AddLabels(
			stageLabel,
			prometheus.Labels{
				"query_kind": "conn_open",
				"vendor":     asset.Vendor,
				"model":      asset.Model,
			}),
	).Observe(time.Since(startTS).Seconds())

	defer func() {
		// measure BMC connection close
		startTS = time.Now()

		if err := bmc.Close(ctx); err != nil {
			o.logger.WithFields(
				logrus.Fields{
					"IP":  asset.BMCAddress.String(),
					"err": err,
				}).Warn("error in bmc connection close")

			// count connection close error metric
			metricBMCQueryErrorCount.With(
				metrics.AddLabels(
					stageLabel,
					prometheus.Labels{
						"query_kind": "conn_close",
						"vendor":     asset.Vendor,
						"model":      asset.Model,
					}),
			).Inc()
		}

		// measure BMC connection close
		metricBMCQueryTimeSummary.With(
			metrics.AddLabels(
				stageLabel,
				prometheus.Labels{
					"query_kind": "conn_close",
					"vendor":     asset.Vendor,
					"model":      asset.Model,
				}),
		).Observe(time.Since(startTS).Seconds())
	}()

	// measure BMC inventory query
	startTS = time.Now()

	device, err := bmc.Inventory(ctx)
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"IP":  asset.BMCAddress.String(),
				"err": err,
			}).Warn("error in bmc inventory collection")

		// count inventory query error metric
		metricBMCQueryErrorCount.With(
			metrics.AddLabels(
				stageLabel,
				prometheus.Labels{
					"query_kind": "inventory",
					"vendor":     asset.Vendor,
					"model":      asset.Model,
				}),
		).Inc()

		return
	}

	// measure BMC inventory query
	metricBMCQueryTimeSummary.With(
		metrics.AddLabels(
			stageLabel,
			prometheus.Labels{
				"query_kind": "inventory",
				"vendor":     asset.Vendor,
				"model":      asset.Model,
			}),
	).Observe(time.Since(startTS).Seconds())

	o.collectorCh <- &model.AssetDevice{ID: asset.ID, Device: device}
}

//  newBMCClient initializes a bmclib client with the given credentials
func newBMCClient(ctx context.Context, asset *model.Asset, l *logrus.Logger) *bmclibv2.Client {
	logger := logrus.New()
	logger.Formatter = l.Formatter

	// setup a logr logger for bmclib
	// bmclib uses logr, for which the trace logs are logged with log.V(3),
	// this is a hax so the logrusr lib will enable trace logging
	// since any value that is less than (logrus.LogLevel - 4) >= log.V(3) is ignored
	// https://github.com/bombsimon/logrusr/blob/master/logrusr.go#L64
	switch l.GetLevel() {
	case logrus.TraceLevel:
		logger.Level = 7
	case logrus.DebugLevel:
		logger.Level = 5
	}

	logruslogr := logrusrv2.New(logger)

	bmcClient := bmclibv2.NewClient(
		asset.BMCAddress.String(),
		"", // port unset
		asset.BMCUsername,
		asset.BMCPassword,
		bmclibv2.WithLogger(logruslogr),
	)

	// measure BMC compatibility query
	startTS := time.Now()

	// filter BMC providers based on compatibility
	//
	// TODO(joel) : when the vendor is known, bmclib could be given hints so as to skip the compatibility check.
	bmcClient.Registry.Drivers = bmcClient.Registry.FilterForCompatible(ctx)

	// measure BMC compatibility check query
	metricBMCQueryTimeSummary.With(
		metrics.AddLabels(
			stageLabel,
			prometheus.Labels{
				"query_kind": "compatibility_check",
				"vendor":     asset.Vendor,
				"model":      asset.Model,
			}),
	).Observe(time.Since(startTS).Seconds())

	return bmcClient
}
