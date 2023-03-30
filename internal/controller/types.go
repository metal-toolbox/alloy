package controller

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/model"
	"go.hollow.sh/toolbox/events"
	"go.infratographer.com/x/pubsubx"
	"go.infratographer.com/x/urnx"

	cptypes "github.com/metal-toolbox/conditionorc/pkg/types"
)

// Task represents a task the controller works on
//
// nolint:govet // fieldalignment - struct layout is preferred as is.
type Task struct {
	// ID is the task identifier
	ID uuid.UUID

	// State identifies the task state value
	State cptypes.ConditionState

	// Status holds information on the task state
	Status string

	// Msg is the original message that created this task.
	// This is here so that the events subsystem can be acked/notified as the task makes progress.
	Msg events.Message

	// Data is the data parsed from Msg for the task runner.
	Data pubsubx.Message

	// Request is the condition parsed from the Data in the Msg
	// the condition defines the kind of work to be performed.
	Request *cptypes.Condition

	// Urn is the URN parsed from Msg for the task runner.
	Urn urnx.URN

	// Asset is the hardware this task is dealing with.
	Asset model.Asset

	// CreatedAt is the timestamp this task was created.
	CreatedAt time.Time

	// UpdatedAt is the timestamp this task was updated.
	UpdatedAt time.Time
}

// TasksLocker holds the list of tasks a controller is dealing with.
type TasksLocker struct {
	tasks map[uuid.UUID]*Task
	mu    sync.RWMutex
}

func NewTasksLocker() *TasksLocker {
	return &TasksLocker{tasks: make(map[uuid.UUID]*Task)}
}

// nolint:gocritic // task passed by value to be stored under lock.
func (a *TasksLocker) Add(task Task) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.tasks[task.ID] = &task
}

// nolint:gocritic // task passed by value to be stored under lock.
func (a *TasksLocker) Update(task Task) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.tasks[task.ID] = &task
}

func (a *TasksLocker) Purge(id uuid.UUID) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.tasks, id)
}

func (a *TasksLocker) List() []Task {
	a.mu.RLock()
	defer a.mu.RUnlock()

	tasks := make([]Task, 0, len(a.tasks))

	for _, task := range a.tasks {
		tasks = append(tasks, *task)
	}

	return tasks
}
