package worker

import (
	"time"

	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/model"
	rctypes "github.com/metal-toolbox/rivets/v2/condition"
)

// Task represents a task the controller works on
//
// nolint:govet // fieldalignment - struct layout is preferred as is.
type Task struct {
	// ID is the task identifier
	ID uuid.UUID

	// state identifies the task state value
	state rctypes.State

	// Status holds information on the task state
	Status string

	// Request is the condition parsed from the Data in the Msg
	// the condition defines the kind of work to be performed.
	Request *rctypes.Condition

	// Asset is the hardware this task is dealing with.
	Asset model.Asset

	// Parameters for this task
	Parameters rctypes.InventoryTaskParameters

	// Revision is updated by the status publisher.
	Revision uint64

	// CreatedAt is the timestamp this task was created.
	CreatedAt time.Time

	// UpdatedAt is the timestamp this task was updated.
	UpdatedAt time.Time
}

func (t *Task) SetState(state rctypes.State) {
	t.state = state
}

func (t *Task) State() rctypes.State {
	return t.state
}
