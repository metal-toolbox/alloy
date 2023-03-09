package controller

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/asset"
	"github.com/metal-toolbox/alloy/internal/collect"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/publish"
	"github.com/sirupsen/logrus"
	"go.hollow.sh/toolbox/events"
)

var (
	AckInprogressTimeout = 2 * time.Minute
	TaskTimeout          = 180 * time.Minute
)

type Controller struct {
	syncWG       *sync.WaitGroup
	assetGetter  asset.Getter
	collector    collect.Collector
	publisher    publish.Publisher
	streamBroker events.StreamBroker
	logger       *logrus.Logger
	running      *running
}

// running keeps track of tasks this controller is actively working on.
type running struct {
	tasks map[uuid.UUID]*Task
	mu    sync.RWMutex
}

// nolint:gocritic // task passed by value to be stored under lock.
func (t *running) add(task Task) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.tasks[task.ID] = &task
}

// nolint:gocritic // task passed by value to be stored under lock.
func (t *running) update(task Task) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.tasks[task.ID] = &task
}

func (t *running) purge(id uuid.UUID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.tasks, id)
}

func (t *running) list() []Task {
	t.mu.RLock()
	defer t.mu.RUnlock()

	tasks := make([]Task, 0, len(t.tasks))
	for _, task := range tasks {
		tasks = append(tasks, task)
	}

	return tasks
}

func New(
	logger *logrus.Logger,
	getter asset.Getter,
	collector collect.Collector,
	publisher publish.Publisher,
	streamBroker events.StreamBroker,
	syncWG *sync.WaitGroup,
) *Controller {
	return &Controller{
		logger:       logger,
		assetGetter:  getter,
		collector:    collector,
		publisher:    publisher,
		streamBroker: streamBroker,
		syncWG:       syncWG,
		running:      &running{tasks: make(map[uuid.UUID]*Task)},
	}
}

func (c *Controller) Run(ctx context.Context) error {
	tickerFetchWork := time.NewTicker(10 * time.Second).C
	tickerGarbageCollect := time.NewTicker(1 * time.Minute).C

	msgCh, err := c.streamBroker.Subscribe(ctx)
	if err != nil {
		c.logger.Fatal(err)
	}

	// init inventory collection channel
	assetCh := make(chan *model.Asset, 0)
	c.assetGetter.SetAssetChannel(assetCh)
	c.collector.SetAssetChannel(assetCh)
	c.publisher.SetAssetChannel(assetCh)

	c.logger.Info("listening for events ...")

	for {
		select {
		case <-tickerFetchWork:
			c.syncWG.Add(1)

			go c.fetchWork(ctx, msgCh)

		case <-tickerGarbageCollect:
			c.syncWG.Add(1)

			go c.garbageCollect(ctx)

		case <-ctx.Done():
			close(assetCh)
			c.streamBroker.Close()

			return nil
		case msg := <-msgCh:
			c.processMsg(ctx, msg)
		}
	}
}

func (c *Controller) garbageCollect(ctx context.Context) {
	// reset active counter on tasks in progress.
	// cancel tasks running for a long time.

	//active := c.running.list()
	//for _, task := range active {
	//	spew.Dump(task)
	//	if time.Now().Sub(task.UpdatedAt) < AckInprogressTimeout {
	//		continue
	//	}

	//}
}

func (c *Controller) fetchWork(ctx context.Context, msgCh events.MsgCh) {
	defer c.syncWG.Done()

	msgs, err := c.streamBroker.FetchMsg(ctx, 1)
	if err != nil {
		c.logger.WithFields(
			logrus.Fields{"err": err.Error()},
		).Error("error fetching work")
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

		return
	}

	if urn.ResourceType != "server" {
		c.logger.Warn("ignored msg with unknown resource type: " + urn.ResourceType)

		msg.Nak()

		return
	}

	task := NewTask(msg, data, urn)

	switch urn.Namespace {
	case "hollow-controllers":
		c.handleEvent(ctx, task)
	default:
		c.logger.Warn("ignored msg with unknown subject URN namespace: " + urn.Namespace)
	}
}

func (c *Controller) handleEvent(ctx context.Context, task *Task) {
	switch task.Data.EventType {
	case "inventoryOutofband":
		c.inventoryOutofband(ctx, task)
	default:
		c.logger.Warn("ignored msg with unknown eventType: " + task.Data.EventType)
		return
	}
}

// SetTaskProgress updates task progress in the events subsystem and the condition orchestrator.
func (c *Controller) SetTaskProgress(ctx context.Context, task *Task, state TaskState, status string) {
	task.State = state
	task.UpdatedAt = time.Now()

	if status != "" {
		task.Status = status
	}

	switch task.State {
	case Pending:
		// mark task as in progress in the events subsystem
		// resetting the event subsystem timer for this task.
		if err := task.Msg.InProgress(); err != nil {
			c.logger.WithFields(logrus.Fields{
				"err":      err.Error(),
				"serverID": task.Urn.ResourceID,
			}).Info("error setting task in progress state")
		}

		// add task to running
		c.running.add(*task)
		// condition orc api here

	case Active:
		// mark task as in progress in the events subsystem
		// resetting the event subsystem timer for this task.
		if err := task.Msg.InProgress(); err != nil {
			c.logger.WithFields(logrus.Fields{
				"err":      err.Error(),
				"serverID": task.Urn.ResourceID,
			}).Info("error setting task in progress state")
		}

		c.running.update(*task)

	case Succeeded, Failed:
		if err := task.Msg.Ack(); err != nil {
			c.logger.WithFields(logrus.Fields{
				"err":      err.Error(),
				"serverID": task.Urn.ResourceID,
			}).Info("error ack'ing task completion")
		}

		c.running.purge(task.ID)
	}
}
