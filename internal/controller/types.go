package controller

import (
	"time"

	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/model"
	"go.hollow.sh/toolbox/events"
	"go.infratographer.com/x/pubsubx"
	"go.infratographer.com/x/urnx"
)

// TaskState is type that specifies the state of a task.
type TaskState string

// Defines values for TaskState.
const (
	Pending   TaskState = "pending"
	Active    TaskState = "active"
	Failed    TaskState = "failed"
	Succeeded TaskState = "succeeded"
)

// Task represents a task the controller works on
type Task struct {
	// ID is the task identifier
	ID uuid.UUID

	// State identifies the task state value
	State TaskState

	// Status holds information on the task state
	Status string

	// Msg is the original message that created this task.
	// This is here so that the events subsystem can be acked/notified as the task makes progress.
	Msg events.Message

	// Data is the data parsed from Msg for the task runner.
	Data pubsubx.Message

	// Urn is the URN parsed from Msg for the task runner.
	Urn urnx.URN

	// Asset is the hardware this task is dealing with.
	Asset model.Asset

	// CreatedAt is the timestamp this task was created.
	CreatedAt time.Time

	// UpdatedAt is the timestamp this task was updated.
	UpdatedAt time.Time
}

// NewTask returns a new task object with the given parameters
func NewTask(msg events.Message, msgData *pubsubx.Message, urn *urnx.URN) *Task {
	return &Task{
		ID:        uuid.New(),
		State:     Pending,
		Msg:       msg,
		Data:      *msgData,
		Urn:       *urn,
		CreatedAt: time.Now(),
	}
}
