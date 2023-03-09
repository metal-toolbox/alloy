package controller

import (
	"context"

	"github.com/sirupsen/logrus"
)

func (c *Controller) inventoryOutofband(ctx context.Context, task *Task) {
	c.SetTaskProgress(ctx, task, Active, "querying inventory for BMC credentials")

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
		c.SetTaskProgress(ctx, task, Failed, cause)

		return
	}

	task.Asset = *assetFetched
	c.SetTaskProgress(ctx, task, Active, "querying device BMC for inventory")

	// collect inventory from asset hardware
	if err := c.collector.ForAsset(ctx, &task.Asset); err != nil {
		c.logger.WithFields(
			logrus.Fields{
				"serverID": &task.Asset.ID,
				"IP":       task.Asset.BMCAddress.String(),
				"err":      err,
			}).Warn("inventory collect error")

		cause := "inventory collect error: " + err.Error()
		c.SetTaskProgress(ctx, task, Failed, cause)

		return
	}

	c.SetTaskProgress(ctx, task, Active, "publishing collected data")

	// publish collected inventory
	if err := c.publisher.PublishOne(ctx, &task.Asset); err != nil {
		c.logger.WithFields(
			logrus.Fields{
				"serverID": &task.Asset.ID,
				"IP":       task.Asset.BMCAddress.String(),
				"err":      err,
			}).Warn("inventory publish error")

		cause := "inventory publish error: " + err.Error()
		c.SetTaskProgress(ctx, task, Failed, cause)

		return
	}

	c.logger.WithFields(
		logrus.Fields{
			"serverID": &task.Asset.ID,
			"IP":       task.Asset.BMCAddress.String(),
		},
	).Trace("collection complete")

	c.SetTaskProgress(ctx, task, Failed, "all done _o/")
}
