package controller

import (
	"context"

	"github.com/metal-toolbox/alloy/internal/collector"
	"github.com/metal-toolbox/alloy/internal/model"
	cptypes "github.com/metal-toolbox/conditionorc/pkg/types"
	"github.com/sirupsen/logrus"
)

func (c *Controller) inventoryOutofband(ctx context.Context, task *Task) {
	// init OOB collector
	oobcollector, err := collector.NewSingleDeviceCollectorWithRepository(
		ctx,
		c.repository,
		model.AppKindOutOfBand,
		c.logger,
	)
	if err != nil {
		if err := c.checkpointHelper.Set(ctx, task, cptypes.Failed, err.Error()); err != nil {
			c.logger.WithFields(
				logrus.Fields{
					"err":      err.Error(),
					"serverID": task.Urn.ResourceID.String(),
				},
			).Error("collection error")
		}

		return
	}

	if err := c.checkpointHelper.Set(ctx, task, cptypes.Active, "querying inventory for BMC credentials"); err != nil {
		c.logger.WithFields(
			logrus.Fields{
				"err":      err.Error(),
				"serverID": task.Urn.ResourceID.String(),
			},
		).Error("asset setting task checkpoint")
	}

	// fetch asset
	assetFetched, err := c.repository.AssetByID(ctx, task.Urn.ResourceID.String(), true)
	if err != nil {
		c.logger.WithFields(
			logrus.Fields{
				"err":      err.Error(),
				"serverID": task.Urn.ResourceID.String(),
			},
		).Error("asset lookup error")

		cause := "asset lookup error: " + err.Error()

		if err := c.checkpointHelper.Set(ctx, task, cptypes.Failed, cause); err != nil {
			c.logger.WithFields(
				logrus.Fields{
					"err":      err.Error(),
					"serverID": task.Urn.ResourceID.String(),
				},
			).Error("asset setting task checkpoint")
		}

		return
	}

	task.Asset = *assetFetched

	c.checkpointHelper.Set(ctx, task, cptypes.Active, "querying device BMC for inventory")

	// collect inventory from asset hardware
	if err := oobcollector.CollectOutofband(ctx, &task.Asset); err != nil {
		c.logger.WithFields(
			logrus.Fields{
				"serverID": &task.Asset.ID,
				"IP":       task.Asset.BMCAddress.String(),
				"err":      err,
			}).Warn("inventory collect error")

		cause := "inventory collect error: " + err.Error()

		if err := c.checkpointHelper.Set(ctx, task, cptypes.Failed, cause); err != nil {
			c.logger.WithFields(
				logrus.Fields{
					"err":      err.Error(),
					"serverID": task.Urn.ResourceID.String(),
				},
			).Error("asset setting task checkpoint")
		}

		return
	}

	c.logger.WithFields(
		logrus.Fields{
			"serverID": &task.Asset.ID,
			"IP":       task.Asset.BMCAddress.String(),
		},
	).Info("collection complete")

	c.checkpointHelper.Set(ctx, task, cptypes.Failed, "all done _o/")
}
