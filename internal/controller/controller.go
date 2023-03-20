package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/metal-toolbox/alloy/internal/collector"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/store"

	// TODO: move these two into a shared package

	cptypes "github.com/metal-toolbox/conditionorc/pkg/types"
	"github.com/sirupsen/logrus"
	"go.hollow.sh/toolbox/events"
)

var (
	concurrency      = 10
	AckActiveTimeout = 3 * time.Minute
	TaskTimeout      = 180 * time.Minute
)

type Controller struct {
	multiCollector   *collector.AssetIterCollector
	singleCollector  *collector.SingleDeviceCollector
	repository       store.Repository
	streamBroker     events.StreamBroker
	checkpointHelper TaskCheckpointer
	tasksLocker      *TasksLocker
	syncWG           *sync.WaitGroup
	logger           *logrus.Logger
}

func New(
	ctx context.Context,
	multiCollector *collector.AssetIterCollector,
	singleCollector *collector.SingleDeviceCollector,
	repository store.Repository,
	streamBroker events.StreamBroker,
	syncWG *sync.WaitGroup,
	logger *logrus.Logger,
) (*Controller, error) {
	tasksLocker := NewTasksLocker()

	checkpointHelper, err := NewTaskCheckpointer("http://conditionorc-api:9001", tasksLocker)
	if err != nil {
		return nil, err
	}

	singleCollector, err = collector.NewSingleDeviceCollectorWithRepository(
		ctx,
		repository,
		model.AppKindOutOfBand,
		logger,
	)
	if err != nil {
		return nil, err
	}

	return &Controller{
		multiCollector:   multiCollector,
		singleCollector:  singleCollector,
		repository:       repository,
		streamBroker:     streamBroker,
		checkpointHelper: checkpointHelper,
		tasksLocker:      tasksLocker,
		syncWG:           syncWG,
		logger:           logger,
	}, nil
}

func (c *Controller) Run(ctx context.Context) error {
	tickerFetchWork := time.NewTicker(10 * time.Second).C
	tickerAckActive := time.NewTicker(1 * time.Minute).C

	msgCh, err := c.streamBroker.Subscribe(ctx)
	if err != nil {
		c.logger.Fatal(err)
	}

	c.logger.Info("listening for events ...")

	for {
		select {
		case <-tickerFetchWork:
			if c.maximumActive() {
				continue
			}

			c.syncWG.Add(1)

			go c.fetchWork(ctx, msgCh)

		case <-tickerAckActive:
			c.syncWG.Add(1)

			go c.ackActive(ctx)

		case <-ctx.Done():
			c.streamBroker.Close()

			return nil
		case msg := <-msgCh:
			c.syncWG.Add(1)

			go c.processMsg(ctx, msg)
		}
	}
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
	defer c.syncWG.Done()
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

func (c *Controller) fetchWork(ctx context.Context, msgCh events.MsgCh) {
	defer c.syncWG.Done()

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
	defer c.syncWG.Done()

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
