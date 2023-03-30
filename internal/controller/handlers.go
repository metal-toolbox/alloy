package controller

import (
	"context"
	"time"

	"github.com/metal-toolbox/alloy/internal/collector"
	"github.com/metal-toolbox/alloy/internal/model"
	cptypes "github.com/metal-toolbox/conditionorc/pkg/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	ErrHandlerTaskCheckpoint = errors.New("handler task checkpoint error")
	ErrHandlerStoreQuery     = errors.New("handler store query error")
	ErrHandlerDeviceQuery    = errors.New("handler device query error")
)

// iterCollectOutofband collects inventory, bios configuation data for all store assets out of band.
func (c *Controller) iterCollectOutofband(ctx context.Context) {
	if c.iterCollectActive {
		c.logger.Warn("iterCollectOutofband currently running, skipped re-run")
		return
	}

	c.iterCollectActive = true
	defer func() { c.iterCollectActive = false }()

	iterCollector, err := collector.NewAssetIterCollectorWithStore(
		ctx,
		model.AppKindOutOfBand,
		c.repository,
		// int32(c.cfg.Concurrency),TODO (joel): revert after testing
		1,
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

// checkpointTaskFailure checkpoints the task failure in the store.
func (c *Controller) checkpointTaskFailure(ctx context.Context, task *Task, state cptypes.ConditionState, err error) {
	if errC := c.checkpointHelper.Set(ctx, task, state, err.Error()); errC != nil {
		c.logger.WithFields(
			logrus.Fields{
				"err":      errC.Error(),
				"serverID": task.Urn.ResourceID.String(),
			},
		).Error(ErrHandlerTaskCheckpoint)
	}
}

// checkpointTaskUpdate checkpoints the task progress in the store,
// this method is invoked when the task has a successful/informational transition to be recorded.
//
// If the checkpoint set in this method fails and the condition has `FailOnCheckpointError“ set to true, this method returns an error.
func (c *Controller) checkpointTaskUpdate(ctx context.Context, task *Task, state cptypes.ConditionState, info string) error {
	if err := c.checkpointHelper.Set(ctx, task, state, info); err != nil {
		c.logger.WithFields(
			logrus.Fields{
				"err":      err.Error(),
				"serverID": task.Urn.ResourceID.String(),
			},
		).Error(ErrHandlerTaskCheckpoint)
	}

	if task.Request.FailOnCheckpointError {
		return ErrHandlerTaskCheckpoint
	}

	return nil
}

// collectOutofbandForTask collects inventory, bios configuration out of band for the given task.
func (c *Controller) collectOutofbandForTask(ctx context.Context, task *Task) {
	c.logger.WithFields(
		logrus.Fields{
			"serverID": task.Urn.ResourceID.String(),
		},
	).Info("processing condition event inventoryOutofband")

	startTS := time.Now()
	// init OOB collector
	oobcollector, err := collector.NewDeviceCollectorWithStore(
		ctx,
		c.repository,
		model.AppKindOutOfBand,
		c.logger,
	)
	if err != nil {
		c.checkpointTaskFailure(ctx, task, cptypes.Failed, err)
		return
	}

	err = c.checkpointTaskUpdate(ctx, task, cptypes.Active, "querying store for BMC credentials")
	if err != nil {
		return
	}

	// fetch asset
	assetFetched, err := c.repository.AssetByID(ctx, task.Urn.ResourceID.String(), true)
	if err != nil {
		c.logger.WithFields(
			logrus.Fields{
				"err":      err.Error(),
				"serverID": task.Urn.ResourceID.String(),
			},
		).Error(ErrHandlerStoreQuery)

		c.checkpointTaskFailure(ctx, task, cptypes.Failed, errors.Wrap(ErrHandlerStoreQuery, err.Error()))

		return
	}

	task.Asset = *assetFetched

	err = c.checkpointTaskUpdate(ctx, task, cptypes.Active, "querying device BMC for inventory, bios configuration")
	if err != nil {
		return
	}

	// collect inventory from asset hardware
	if errCollect := oobcollector.CollectOutofband(ctx, &task.Asset); errCollect != nil {
		c.logger.WithFields(
			logrus.Fields{
				"serverID": &task.Asset.ID,
				"IP":       task.Asset.BMCAddress.String(),
				"err":      errCollect,
			}).Warn(ErrHandlerDeviceQuery)

		c.checkpointTaskFailure(ctx, task, cptypes.Failed, errors.Wrap(ErrHandlerDeviceQuery, errCollect.Error()))

		return
	}

	c.logger.WithFields(
		logrus.Fields{
			"serverID": &task.Asset.ID,
			"IP":       task.Asset.BMCAddress.String(),
		},
	).Info("collection complete")

	err = c.checkpointTaskUpdate(ctx, task, cptypes.Succeeded, "completed in "+time.Since(startTS).String())
	if err != nil {
		return
	}
}
