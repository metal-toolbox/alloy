package controller

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.hollow.sh/toolbox/events"
	"go.infratographer.com/x/pubsubx"
	"go.infratographer.com/x/urnx"
	"golang.org/x/exp/slices"

	cptypes "github.com/metal-toolbox/conditionorc/pkg/types"
)

const (
	controllerURNNamespace = "hollow-controllers"
)

var (
	EventAckError = "error Acking event"
	ErrNewTask    = errors.New("error retrieving task data from event")
)

func actionableResourceTypes() []string {
	return []string{
		cptypes.ServerResourceType,
		"facility",
	}
}

// NewTaskFromEvent returns a new task object with the given parameters
func NewTaskFromEvent(msg events.Message, msgData *pubsubx.Message, urn *urnx.URN) (*Task, error) {
	value, exists := msgData.AdditionalData["data"]
	if !exists {
		return nil, errors.Wrap(ErrNewTask, "data in event is empty")
	}

	// we do this marshal, unmarshal dance here
	// since value is of type map[string]interface{} and unpacking this
	// into a known type isn't easily feasible (or atleast I'd be happy to find out otherwise).
	cbytes, err := json.Marshal(value)
	if err != nil {
		return nil, errors.Wrap(ErrNewTask, err.Error())
	}

	condition := &cptypes.Condition{}
	if err := json.Unmarshal(cbytes, condition); err != nil {
		return nil, errors.Wrap(ErrNewTask, err.Error())
	}

	return &Task{
		ID:        uuid.New(),
		State:     cptypes.Pending,
		Request:   condition,
		Msg:       msg,
		Data:      *msgData,
		Urn:       *urn,
		CreatedAt: time.Now(),
	}, nil
}

func (c *Controller) fetchEvent(ctx context.Context, msgCh events.MsgCh) {
	msgs, err := c.streamBroker.PullMsg(ctx, 1)
	if err != nil {
		c.logger.WithFields(
			logrus.Fields{"err": err.Error()},
		).Debug("no new events")
	}

	for _, msg := range msgs {
		msgCh <- msg
	}
}

func (c *Controller) ackComplete(event events.Message) {
	if err := event.Ack(); err != nil {
		c.logger.WithError(err).Warn(EventAckError)
	}
}

// reset active counter on tasks in progress.
//
// TODO: cancel long running tasks - pass cancel func into task struct?
func (c *Controller) ackActive(ctx context.Context) {
	active := c.tasksLocker.List()

	for idx := range active {
		if ctx.Err() != nil {
			return
		}

		c.logger.WithFields(
			logrus.Fields{
				"serverID":  active[idx].Urn.ResourceID,
				"condition": active[idx].Urn.ResourceType,
			},
		).Trace("ack ing condition event as active")

		if time.Since(active[idx].UpdatedAt) < AckActiveTimeout {
			continue
		}

		if err := active[idx].Msg.InProgress(); err != nil {
			c.logger.WithError(err).Error("error ack msg as in-progress")
		}
	}
}

func (c *Controller) processEvent(ctx context.Context, event events.Message) {
	data, err := event.Data()
	if err != nil {
		c.logger.WithFields(
			logrus.Fields{"err": err.Error(), "subject": event.Subject()},
		).Error("event unpack error")

		return
	}

	urn, err := event.SubjectURN(data)
	if err != nil {
		c.logger.WithFields(
			logrus.Fields{"err": err.Error(), "subject": event.Subject()},
		).Error("error parsing subject URN in msg")

		return
	}

	if !slices.Contains(actionableResourceTypes(), urn.ResourceType) {
		c.logger.Warn("ignored msg with unknown resource type: " + urn.ResourceType)

		c.ackComplete(event)

		return
	}

	task, err := NewTaskFromEvent(event, data, urn)
	if err != nil {
		c.logger.WithError(err).Warn("error creating task from msg")

		c.ackComplete(event)

		return
	}

	switch urn.Namespace {
	case controllerURNNamespace:
		c.handleEvent(ctx, task)
	default:
		c.logger.Warn("ignored msg with unknown subject URN namespace: " + urn.Namespace)

		c.ackComplete(event)
	}
}

func (c *Controller) handleEvent(ctx context.Context, task *Task) {
	switch task.Data.EventType {
	case string(cptypes.InventoryOutofband):
		c.collectOutofbandForTask(ctx, task)
	default:
		c.logger.Warn("ignored msg with unknown eventType: " + task.Data.EventType)
		return
	}
}
