package controller

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/collector"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/store"

	// TODO: move these two into a shared package

	cptypes "github.com/metal-toolbox/conditionorc/pkg/types"
	"github.com/sirupsen/logrus"
	"go.hollow.sh/toolbox/events"
)

var (
	concurrency          = 10
	collectInterval      = 1 * time.Hour
	collectIntervalSplay = 10 * time.Minute
	fetchEventsInterval  = 10 * time.Second
	AckActiveTimeout     = 3 * time.Minute
	ackActiveInterval    = 1 * time.Minute
	TaskTimeout          = 180 * time.Minute
)

type Controller struct {
	collectAllRunning bool
	cfg               *app.Configuration
	tasksLocker       *TasksLocker
	syncWG            *sync.WaitGroup
	logger            *logrus.Logger
	repository        store.Repository
	streamBroker      events.StreamBroker
	checkpointHelper  TaskCheckpointer
}

func New(
	ctx context.Context,
	streamBroker events.StreamBroker,
	cfg *app.Configuration,
	syncWG *sync.WaitGroup,
	logger *logrus.Logger,
) (*Controller, error) {
	tasksLocker := NewTasksLocker()

	checkpointHelper, err := NewTaskCheckpointer("http://conditionorc-api:9001", tasksLocker)
	if err != nil {
		return nil, err
	}

	if cfg.Concurrency == 0 {
		cfg.Concurrency = concurrency
	}

	if cfg.CollectInterval == 0 {
		cfg.CollectInterval = collectInterval
	}

	if cfg.CollectIntervalSplay == 0 {
		cfg.CollectIntervalSplay = collectIntervalSplay
	}

	repository, err := store.NewRepository(ctx, cfg.StoreKind, model.AppKindOutOfBand, cfg, logger)
	if err != nil {
		return nil, err
	}

	return &Controller{
		cfg:              cfg,
		tasksLocker:      tasksLocker,
		syncWG:           syncWG,
		logger:           logger,
		repository:       repository,
		streamBroker:     streamBroker,
		checkpointHelper: checkpointHelper,
	}, nil
}

func (c *Controller) connectStream(ctx context.Context) (events.MsgCh, error) {
	if err := c.streamBroker.Open(); err != nil {
		return nil, err
	}

	return c.streamBroker.Subscribe(ctx)
}

func (c *Controller) Run(ctx context.Context) {
	c.logger.Info("listening for events ...")

	// TODO: implement stream reconnect loop
	eventCh, err := c.connectStream(ctx)
	if err != nil {
		c.logger.WithError(err).Error("event stream connection error")

		c.loopWithoutEventstream(ctx)

		return
	}

	c.logger.Error("connected to event stream.")
	c.loopWithEventstream(ctx, eventCh)
}

func (c *Controller) loopWithEventstream(ctx context.Context, eventCh events.MsgCh) {
	tickerFetchEvents := time.NewTicker(fetchEventsInterval).C
	tickerAckActive := time.NewTicker(ackActiveInterval).C
	tickerCollectAll := time.NewTicker(c.splayInterval()).C

	for {
		select {
		case <-ctx.Done():
			c.streamBroker.Close()

		case <-tickerFetchEvents:
			if c.maximumActive() {
				continue
			}

			c.syncWG.Add(1)

			go func() { defer c.syncWG.Done(); c.fetchEvents(ctx, eventCh) }()

		case <-tickerAckActive:
			c.syncWG.Add(1)

			go func() { defer c.syncWG.Done(); c.ackActive(ctx) }()

		case <-tickerCollectAll:
			c.syncWG.Add(1)

			go func() { defer c.syncWG.Done(); c.collectAll(ctx) }()

		case msg := <-eventCh:
			c.syncWG.Add(1)

			go func() { defer c.syncWG.Done(); c.processMsg(ctx, msg) }()
		}
	}
}

func (c *Controller) loopWithoutEventstream(ctx context.Context) {
	tickerCollectAll := time.NewTicker(c.splayInterval()).C

	for {
		select {
		case <-ctx.Done():
			return

		case <-tickerCollectAll:
			c.syncWG.Add(1)

			go func() { defer c.syncWG.Done(); c.collectAll(ctx) }()
		}
	}
}

func (c *Controller) splayInterval() time.Duration {
	// randomize to given splay value and add to interval
	rand.Seed(time.Now().UnixNano())

	// nolint:gosec // Ideally this should be using crypto/rand,
	//                 although the generated random value here is just used to add jitter/splay to
	//                 the interval value and is not used outside of this context.
	return c.cfg.CollectInterval + time.Duration(rand.Int63n(int64(c.cfg.CollectIntervalSplay)))
}

func (c *Controller) maximumActive() bool {
	active := c.tasksLocker.List()

	var found int

	for _, task := range active {
		if !cptypes.ConditionStateFinalized(task.State) {
			found++
		}
	}

	return found >= concurrency
}

func (c *Controller) ackActive(ctx context.Context) {
	// reset active counter on tasks in progress.
	// cancel tasks running for a long time.

	active := c.tasksLocker.List()

	for _, task := range active {
		if time.Since(task.UpdatedAt) > AckActiveTimeout {
			if err := task.Msg.InProgress(); err != nil {
				c.logger.WithError(err).Error("error ack msg as in-progress")
			} else {
				fmt.Println(task.Urn.ResourceID)
				fmt.Println("acked msg as in progress")
			}
		}
	}
}

func (c *Controller) fetchEvents(ctx context.Context, msgCh events.MsgCh) {
	msgs, err := c.streamBroker.PullMsg(ctx, 1)
	if err != nil {
		c.logger.WithFields(
			logrus.Fields{"err": err.Error()},
		).Debug("error fetching work")
	}

	for _, msg := range msgs {
		msgCh <- msg
	}
}

func (c *Controller) processMsg(ctx context.Context, msg events.Message) {
	data, err := msg.Data()
	if err != nil {
		c.logger.WithFields(
			logrus.Fields{"err": err.Error(), "subject": msg.Subject()},
		).Error("data unpack error")

		return
	}

	urn, err := msg.SubjectURN(data)
	if err != nil {
		c.logger.WithFields(
			logrus.Fields{"err": err.Error(), "subject": msg.Subject()},
		).Error("error parsing subject URN in msg")

		if err := msg.Ack(); err != nil {
			c.logger.WithError(err).Warn("failed Nak msg")
		}

		return
	}

	if urn.ResourceType != cptypes.ServerResourceType {
		c.logger.Warn("ignored msg with unknown resource type: " + urn.ResourceType)

		if err := msg.Ack(); err != nil {
			c.logger.WithError(err).Warn("failed Nak msg")
		}

		return
	}

	task, err := NewTaskFromMsg(msg, data, urn)
	if err != nil {
		c.logger.WithError(err).Warn("error creating task from msg")

		if err := msg.Ack(); err != nil {
			c.logger.WithError(err).Warn("failed Nak msg")
		}

		return
	}

	switch urn.Namespace {
	case "hollow-controllers":
		c.handleEvent(ctx, task)
	default:
		if err := msg.Ack(); err != nil {
			c.logger.WithError(err).Warn("failed Nak msg")
		}

		c.logger.Warn("ignored msg with unknown subject URN namespace: " + urn.Namespace)
	}
}

func (c *Controller) handleEvent(ctx context.Context, task *Task) {
	switch task.Data.EventType {
	case string(cptypes.InventoryOutofband):
		c.inventoryOutofband(ctx, task)
	default:
		c.logger.Warn("ignored msg with unknown eventType: " + task.Data.EventType)
		return
	}
}

func (c *Controller) collectAll(ctx context.Context) {
	if c.collectAllRunning {
		c.logger.Warn("collectAll currently running, skipped re-run")
		return
	}

	c.collectAllRunning = true
	defer func() { c.collectAllRunning = false }()

	iterCollector, err := collector.NewAssetIterCollectorWithStore(
		ctx,
		model.AppKindOutOfBand,
		c.repository,
		int32(c.cfg.Concurrency),
		c.syncWG,
		c.logger,
	)
	if err != nil {
		c.logger.WithError(err).Error("collectAll asset iterator error")
		return
	}

	c.logger.Info("collecting inventory for all assets..")
	iterCollector.Collect(ctx)
}
