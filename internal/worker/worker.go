package worker

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/collector"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/store"
	"github.com/metal-toolbox/alloy/internal/version"

	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"

	"github.com/sirupsen/logrus"
	"go.hollow.sh/toolbox/events"
	"go.hollow.sh/toolbox/events/registry"

	rctypes "github.com/metal-toolbox/rivets/condition"
	rkv "github.com/metal-toolbox/rivets/kv"
)

const (
	pkgName = "internal/worker"

	concurrency = 10

	fetchEventsInterval = 10 * time.Second

	// taskTimeout defines the time after which a task will be canceled.
	taskTimeout = 180 * time.Minute

	// taskInprogressTicker is the interval at which tasks in progress
	// will ack themselves as in progress on the event stream.
	//
	// This value should be set to less than the event stream Ack timeout value.
	taskInprogressTick = 3 * time.Minute
)

var (
	errConditionDeserialize = errors.New("unable to deserialize condition")
	errTaskFirmwareParam    = errors.New("error in task firmware parameters")
	errInitTask             = errors.New("error initializing new task from event")

	errAssetNotFound  = errors.New("asset not found in inventory")
	errInventoryQuery = errors.New("inventory query returned error")

	errCollector = errors.New("collector error")
)

type Worker struct {
	repository   store.Repository
	stream       events.Stream
	id           registry.ControllerID
	cfg          *app.Configuration
	syncWG       *sync.WaitGroup
	logger       *logrus.Logger
	name         string
	facilityCode string
	replicaCount int
	concurrency  int
	dispatched   int32
}

func New(
	ctx context.Context,
	facilityCode string,
	stream events.Stream,
	cfg *app.Configuration,
	syncWG *sync.WaitGroup,
	logger *logrus.Logger,
) (*Worker, error) {
	id, _ := os.Hostname()

	concurrency := concurrency
	if cfg.Concurrency != 0 {
		concurrency = cfg.Concurrency
	}

	repository, err := store.NewRepository(ctx, cfg.StoreKind, model.AppKindOutOfBand, cfg, logger)
	if err != nil {
		return nil, err
	}

	return &Worker{
		name:         id,
		facilityCode: facilityCode,
		cfg:          cfg,
		syncWG:       syncWG,
		logger:       logger,
		repository:   repository,
		stream:       stream,
		concurrency:  concurrency,
	}, nil
}

func (w *Worker) Run(ctx context.Context) {
	tickerFetchEvents := time.NewTicker(fetchEventsInterval).C

	if err := w.stream.Open(); err != nil {
		w.logger.WithError(err).Error("event stream connection error")
		return
	}

	// returned channel ignored, since this is a Pull based subscription.
	_, err := w.stream.Subscribe(ctx)
	if err != nil {
		w.logger.WithError(err).Error("event stream subscription error")
		return
	}

	w.logger.Info("connected to event stream.")

	// register worker in NATS active-controllers kv bucket
	w.startWorkerLivenessCheckin(ctx)

	if _, err := createOrBindKVBucketWithOpts(w.stream, w.cfg.NatsOptions.KV); err != nil {
		w.logger.WithError(err).Error("failed to create/bind to status kv" + inventoryStatusKVBucket)
	}

	v := version.Current()
	w.logger.WithFields(
		logrus.Fields{
			"version":     v.AppVersion,
			"commit":      v.GitCommit,
			"branch":      v.GitBranch,
			"concurrency": w.concurrency,
		},
	).Info("Alloy controller running")

Loop:
	for {
		select {
		case <-tickerFetchEvents:
			if w.concurrencyLimit() {
				continue
			}

			w.processEvents(ctx)

		case <-ctx.Done():
			if w.dispatched > 0 {
				continue
			}

			break Loop
		}
	}
}

func (w *Worker) processEvents(ctx context.Context) {
	// XXX: consider having a separate context for message retrieval
	msgs, err := w.stream.PullMsg(ctx, 1)

	switch {
	case err == nil:
	case errors.Is(err, nats.ErrTimeout):
		w.logger.WithFields(
			logrus.Fields{"err": err.Error()},
		).Trace("no new events")
	default:
		w.logger.WithFields(
			logrus.Fields{"err": err.Error()},
		).Warn("retrieving new messages")

		metrics.NATSError("pull-msg")
	}

	for _, msg := range msgs {
		if ctx.Err() != nil || w.concurrencyLimit() {
			w.eventNak(msg)

			return
		}

		// spawn msg process handler
		w.syncWG.Add(1)

		go func(msg events.Message) {
			defer w.syncWG.Done()

			atomic.AddInt32(&w.dispatched, 1)
			defer atomic.AddInt32(&w.dispatched, -1)

			w.processSingleEvent(ctx, msg)
		}(msg)
	}
}

func (w *Worker) processSingleEvent(ctx context.Context, e events.Message) {
	// extract parent trace context from the event if any.
	ctx = e.ExtractOtelTraceContext(ctx)

	ctx, span := otel.Tracer(pkgName).Start(
		ctx,
		"worker.processSingleEvent",
	)
	defer span.End()

	condition, err := conditionFromEvent(e)
	if err != nil {
		w.logger.WithError(err).WithField("subject", e.Subject()).Warn("unable to retrieve condition from message")

		metrics.RegisterEventCounter(false, "ack")
		w.eventAckComplete(e)

		return
	}

	// check and see if the task is or has-been handled by another worker
	currentState, err := rkv.ConditionStatus(
		condition.ID.String(),
		w.facilityCode,
		inventoryStatusKVBucket,
		events.AsNatsJetStreamContext(w.stream.(*events.NatsJetstream)),
	)

	// errors from ConditionStatus are for logging purposes only
	if err != nil {
		w.logger.WithField("conditionID", condition.ID.String()).Warn(err)
	}

	switch currentState {
	case rkv.InProgress:
		w.logger.WithField("conditionID", condition.ID.String()).Info("condition is already in progress")
		w.eventAckInProgress(e)
		metrics.RegisterSpanEvent(span, condition, w.id.String(), "", "ackInProgress")

		return

	case rkv.Complete:
		w.logger.WithField("conditionID", condition.ID.String()).Info("condition is complete")
		w.eventAckComplete(e)
		metrics.RegisterSpanEvent(span, condition, w.id.String(), "", "ackComplete")

		return

	case rkv.Orphaned:
		w.logger.WithField("conditionID", condition.ID.String()).Warn("restarting this condition")
		metrics.RegisterSpanEvent(span, condition, w.id.String(), "", "restarting condition")

	// we need to restart this event
	case rkv.NotStarted:
		w.logger.WithField("conditionID", condition.ID.String()).Info("starting new condition")
		metrics.RegisterSpanEvent(span, condition, w.id.String(), "", "start new condition")

	// break out here, this is a new event
	case rkv.Indeterminate:
		w.logger.WithField("conditionID", condition.ID.String()).Warn("unable to determine state of this condition")
		// send it back to NATS to try again
		w.eventNak(e)
		metrics.RegisterSpanEvent(span, condition, w.id.String(), "", "sent nack, indeterminate state")

		return
	}

	w.doWork(ctx, condition, e)
}

// doWork executes the task and updates the nats JS with the event status along with publishing the task status.
func (w *Worker) doWork(ctx context.Context, condition *rctypes.Condition, e events.Message) {
	ctx, span := otel.Tracer(pkgName).Start(
		ctx,
		"worker.do",
	)
	defer span.End()

	task, err := newTaskFromCondition(condition)
	if err != nil {
		w.logger.WithError(err).Warn("error initializing task from condition")

		w.eventAckComplete(e)

		metrics.RegisterEventCounter(false, "ack")
		metrics.RegisterSpanEvent(span, condition, w.id.String(), "", "sent ack, error task init")

		return
	}

	startTS := time.Now()

	publisher, err := newStatusKVPublisher(w.stream, w.logger, w.id.String(), w.cfg.NatsOptions.KV)
	if err != nil {
		w.logger.WithError(err).Warn("status KV init - internal error")

		w.eventNak(e)

		metrics.RegisterEventCounter(false, "nack")
		metrics.RegisterSpanEvent(span, condition, w.id.String(), "", "sent nack, error task init")
	}

	// update task state, status
	task.Status = "Collecting inventory outofband for device"
	task.SetState(rctypes.Active)

	w.logger.WithFields(logrus.Fields{
		"deviceID":    task.Parameters.AssetID.String(),
		"conditionID": task.ID,
	}).Info(task.Status)

	// publish update
	publisher.Publish(ctx, task)

	// check no error
	err = w.runTaskWithMonitor(ctx, task, e)
	switch err {
	case errInventoryQuery:
		// inventory lookup failure - non 404 errors
		task.SetState(rctypes.Failed)
		task.Status = err.Error()

		metrics.RegisterEventCounter(true, "nack")
		w.eventNak(e) // have the message bus re-deliver the message

		metrics.RegisterSpanEvent(
			span,
			condition,
			w.id.String(),
			task.Parameters.AssetID.String(),
			"sent nack: store query error",
		)

	case errAssetNotFound, errCollector:
		// asset was not found
		task.SetState(rctypes.Failed)
		task.Status = err.Error()

		w.eventAckComplete(e)

		metrics.RegisterSpanEvent(
			span,
			condition,
			w.id.String(),
			task.Parameters.AssetID.String(),
			"sent ack: error"+err.Error(),
		)

	case nil:
		// work completed successfully
		task.SetState(rctypes.Succeeded)
		publisher.Publish(ctx, task)

		w.eventAckComplete(e)

		metrics.RegisterConditionMetrics(startTS, string(rctypes.Succeeded))
		metrics.RegisterEventCounter(true, "ack")
		metrics.RegisterSpanEvent(
			span,
			condition,
			w.id.String(),
			task.Parameters.AssetID.String(),
			"sent ack: condition finalized",
		)

		publisher.Publish(ctx, task)
		w.logger.WithFields(logrus.Fields{
			"deviceID":    task.Parameters.AssetID.String(),
			"conditionID": task.ID,
			"elapsed":     time.Since(startTS).String(),
			"state":       task.state,
			"status":      task.Status,
		}).Info("task for device completed")
	}
}

// runTaskWithMonitor runs the task method based on the parameters, while ack'ing its progress to the NATS JS.
func (w *Worker) runTaskWithMonitor(ctx context.Context, task *Task, e events.Message) error {
	ctx, span := otel.Tracer(pkgName).Start(
		ctx,
		"worker.runTaskWithMonitor",
	)
	defer span.End()

	// the child function is expected to close this channel to indicate its done
	doneCh := make(chan bool)

	// monitor sends in progress ack's until the task completes.
	monitor := func() {
		defer w.syncWG.Done()

		ticker := time.NewTicker(taskInprogressTick)
		defer ticker.Stop()

	Loop:
		for {
			select {
			case <-ticker.C:
				w.eventAckInProgress(e)
			case <-doneCh:
				break Loop
			}
		}
	}

	w.syncWG.Add(1)

	go monitor()

	taskCtx, cancel := context.WithTimeout(ctx, taskTimeout)
	defer cancel()

	switch task.Parameters.Method {
	case rctypes.InbandInventory:
		return errors.Wrap(errTaskFirmwareParam, "inband inventory collector not implemented")
	case rctypes.OutofbandInventory:
		return w.inventoryOutofband(taskCtx, task, doneCh)
	default:
		return errors.Wrap(errTaskFirmwareParam, "invalid method: "+string(task.Parameters.Method))
	}
}

func (w *Worker) inventoryOutofband(ctx context.Context, task *Task, doneCh chan bool) error {
	ctx, span := otel.Tracer(pkgName).Start(
		ctx,
		"worker.inventoryOutofband",
	)
	defer span.End()

	defer close(doneCh)

	// fetch asset inventory from inventory store
	asset, err := w.repository.AssetByID(ctx, task.Parameters.AssetID.String(), true)
	if err != nil {
		if strings.Contains(err.Error(), "no rows in result set") {
			return errors.Wrap(errAssetNotFound, err.Error())
		}

		return errors.Wrap(errInventoryQuery, err.Error())
	}

	c, err := collector.NewDeviceCollectorWithStore(w.repository, w.cfg.AppKind, w.logger)
	if err != nil {
		return errors.Wrap(errCollector, err.Error())
	}

	return c.CollectOutofband(ctx, asset, false)
}