package worker

import (
	"encoding/json"
	"time"

	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/pkg/errors"
	"github.com/metal-toolbox/rivets/events"

	rctypes "github.com/metal-toolbox/rivets/condition"
)

func (w *Worker) concurrencyLimit() bool {
	return int(w.dispatched) >= w.concurrency
}

func conditionFromEvent(e events.Message) (*rctypes.Condition, error) {
	data := e.Data()
	if data == nil {
		return nil, errors.New("data field empty")
	}

	condition := &rctypes.Condition{}
	if err := json.Unmarshal(data, condition); err != nil {
		return nil, errors.Wrap(errConditionDeserialize, err.Error())
	}

	return condition, nil
}

// NewTaskFromEvent returns a new task object with the given parameters
func newTaskFromCondition(condition *rctypes.Condition) (*Task, error) {
	parameters := &rctypes.InventoryTaskParameters{}
	if err := json.Unmarshal(condition.Parameters, parameters); err != nil {
		return nil, errors.Wrap(errInitTask, "Inventory task parameters error: "+err.Error())
	}

	return &Task{
		ID:         condition.ID,
		state:      rctypes.Pending,
		Request:    condition,
		Parameters: *parameters,
		CreatedAt:  time.Now(),
	}, nil
}

func (w *Worker) eventAckInProgress(event events.Message) {
	if err := event.InProgress(); err != nil {
		metrics.NATSError("ack-in-progress")
		w.logger.WithError(err).Warn("event Ack Inprogress error")
	}
}

func (w *Worker) eventAckComplete(event events.Message) {
	if err := event.Ack(); err != nil {
		metrics.NATSError("ack")
		w.logger.WithError(err).Warn("event Ack error")
	}
}

func (w *Worker) eventNak(event events.Message) {
	if err := event.Nak(); err != nil {
		metrics.NATSError("nak")
		w.logger.WithError(err).Warn("event Nak error")
	}
}
