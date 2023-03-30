package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	v1cclient "github.com/metal-toolbox/conditionorc/pkg/api/v1/client"
	v1ctypes "github.com/metal-toolbox/conditionorc/pkg/api/v1/types"
	cptypes "github.com/metal-toolbox/conditionorc/pkg/types"
)

var (
	ErrCheckpointSet    = errors.New("error setting task checkpoint")
	ErrConditionOrcResp = errors.New("error in conditionorc response")
)

// TaskCheckpointer checkpoints a task by updating its status
//
// Going ahead long running tasks are to set agreed 'checkpoints' where it can resume from in cases of failure.
type TaskCheckpointer interface {
	Set(ctx context.Context, task *Task, state cptypes.ConditionState, status string) error
	Get() error
}

// OrcCheckpointer implements the TaskCheckpointer interface for the conditionorc-api and the events subsystem.
type OrcCheckpointer struct {
	// tasksLocker is the list of tasks being worked on by the orchestrator under a mutex.
	tasksLocker *TasksLocker

	// orcclient is the condition orchestrator client
	orcclient *v1cclient.Client // condition orchestrator client
}

// NewTaskCheckpointer returns a task checkpointer to persist task progress.
func NewTaskCheckpointer(serverAddress string, tasks *TasksLocker) (TaskCheckpointer, error) {
	orcclient, err := v1cclient.NewClient(serverAddress)
	if err != nil {
		return nil, err
	}

	return &OrcCheckpointer{tasksLocker: tasks, orcclient: orcclient}, nil
}

// Get returns current checkpoint information on a task
//
// TO be implemented.
func (c *OrcCheckpointer) Get() error {
	return nil
}

// SetTaskProgress updates task progress in the events subsystem, the condition orchestrator and the local TasksLocker data under mutex.
//
// The NATS streaming subsystem has to be acked to makes sure it does not redeliver the same message.
func (c *OrcCheckpointer) Set(ctx context.Context, task *Task, state cptypes.ConditionState, status string) error {
	previousState := task.State
	task.State = state
	task.UpdatedAt = time.Now()

	if status != "" {
		task.Status = status
	}

	// update condition orchestrator
	if err := c.updateCondition(
		ctx,
		task.Urn.ResourceID,
		cptypes.ConditionKind(task.Data.EventType),
		state,
		statusInfoJSON(status),
	); err != nil {
		return err
	}

	switch task.State {
	case cptypes.Pending:
		// mark task as in progress in the events subsystem
		// resetting the event subsystem timer for this task.
		if err := task.Msg.InProgress(); err != nil {
			return err
		}

		c.tasksLocker.Add(*task)

	case cptypes.Active:
		// mark task as in progress in the events subsystem
		// resetting the event subsystem timer for this task.
		if err := task.Msg.InProgress(); err != nil {
			return err
		}

		if previousState != cptypes.Active {
			c.tasksLocker.Add(*task)
		} else {
			c.tasksLocker.Update(*task)
		}

	case cptypes.Succeeded, cptypes.Failed:
		if err := task.Msg.Ack(); err != nil {
			return err
		}

		c.tasksLocker.Purge(task.ID)
	}

	return nil
}

func statusInfoJSON(s string) json.RawMessage {
	return []byte(fmt.Sprintf("{%q: %q}", "output", s))
}

func (c *OrcCheckpointer) updateCondition(ctx context.Context, serverID uuid.UUID, kind cptypes.ConditionKind, state cptypes.ConditionState, status json.RawMessage) error {
	response, err := c.orcclient.ServerConditionGet(ctx, serverID, kind)
	if err != nil {
		return err
	}

	if response == nil || response.Record == nil || response.Record.Condition == nil {
		return ErrConditionOrcResp
	}

	update := v1ctypes.ConditionUpdate{
		State:           state,
		Status:          status,
		ResourceVersion: response.Record.Condition.ResourceVersion,
	}

	// TODO: add retries for resource version mismatch errors
	_, err = c.orcclient.ServerConditionUpdate(ctx, serverID, kind, update)
	if err != nil {
		return err
	}

	return nil
}
