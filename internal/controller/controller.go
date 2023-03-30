package controller

import (
	"context"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/metal-toolbox/alloy/internal/app"
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
	repository        store.Repository
	streamBroker      events.StreamBroker
	checkpointHelper  TaskCheckpointer
	cfg               *app.Configuration
	tasksLocker       *TasksLocker
	syncWG            *sync.WaitGroup
	logger            *logrus.Logger
	iterCollectActive bool
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
	// TODO: implement stream reconnect loop
	eventCh, err := c.connectStream(ctx)
	if err != nil {
		c.logger.WithError(err).Error("event stream connection error")

		c.loopWithoutEventstream(ctx)

		return
	}

	c.logger.Info("connected to event stream.")

	c.loopWithEventstream(ctx, eventCh)
}

func (c *Controller) loopWithEventstream(ctx context.Context, eventCh events.MsgCh) {
	tickerFetchEvent := time.NewTicker(fetchEventsInterval).C
	tickerAckActive := time.NewTicker(ackActiveInterval).C
	tickerCollectAll := time.NewTicker(c.splayInterval()).C

	// to kick alloy to collect for all assets.
	sigHupCh := make(chan os.Signal, 1)
	signal.Notify(sigHupCh, syscall.SIGHUP)

	for {
		select {
		case <-ctx.Done():
			c.streamBroker.Close()

		case <-tickerFetchEvent:
			if c.maximumActive() {
				continue
			}

			c.syncWG.Add(1)

			go func() { defer c.syncWG.Done(); c.fetchEvent(ctx, eventCh) }()

		case <-tickerAckActive:
			c.syncWG.Add(1)

			go func() { defer c.syncWG.Done(); c.ackActive(ctx) }()

		case <-tickerCollectAll:
			c.syncWG.Add(1)

			go func() { defer c.syncWG.Done(); c.iterCollectOutofband(ctx) }()

		case <-sigHupCh:
			c.syncWG.Add(1)

			go func() { defer c.syncWG.Done(); c.iterCollectOutofband(ctx) }()

		case event := <-eventCh:
			c.syncWG.Add(1)

			go func() { defer c.syncWG.Done(); c.processEvent(ctx, event) }()
		}
	}
}

func (c *Controller) loopWithoutEventstream(ctx context.Context) {
	tickerCollectAll := time.NewTicker(c.splayInterval()).C

	// to kick alloy to collect for all assets.
	sigHupCh := make(chan os.Signal, 1)
	signal.Notify(sigHupCh, syscall.SIGHUP)

	for {
		select {
		case <-ctx.Done():
			return

		case <-sigHupCh:
			c.syncWG.Add(1)

			go func() { defer c.syncWG.Done(); c.iterCollectOutofband(ctx) }()

		case <-tickerCollectAll:
			c.syncWG.Add(1)

			go func() { defer c.syncWG.Done(); c.iterCollectOutofband(ctx) }()
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

	for idx := range active {
		if !cptypes.ConditionStateFinalized(active[idx].State) {
			found++
		}
	}

	return found >= concurrency
}
