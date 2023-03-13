package controller

import (
	"context"

	cptypes "github.com/metal-toolbox/conditionorc/pkg/types"
	"github.com/sirupsen/logrus"
)

func (c *Controller) inventoryOutofband(ctx context.Context, task *Task) {
	if err := c.checkpointHelper.Set(ctx, task, cptypes.Active, "querying inventory for BMC credentials"); err != nil {
		c.logger.WithFields(
			logrus.Fields{
				"err":      err.Error(),
				"serverID": task.Urn.ResourceID.String(),
			},
		).Error("asset setting task checkpoint")
	}

	// fetch asset
	assetFetched, err := c.assetGetter.AssetByID(ctx, task.Urn.ResourceID.String(), true)
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
	if err := c.collector.ForAsset(ctx, &task.Asset); err != nil {
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

	c.checkpointHelper.Set(ctx, task, cptypes.Active, "publishing collected data")

	// publish collected inventory
	if err := c.publisher.PublishOne(ctx, &task.Asset); err != nil {
		c.logger.WithFields(
			logrus.Fields{
				"serverID": &task.Asset.ID,
				"IP":       task.Asset.BMCAddress.String(),
				"err":      err,
			}).Warn("inventory publish error")

		cause := "inventory publish error: " + err.Error()
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
