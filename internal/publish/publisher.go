package publish

import (
	"context"

	"github.com/metal-toolbox/alloy/internal/model"
)

// Publisher defines an interface for device inventory to be published to remote endpoints.
type Publisher interface {
	// RunInventoryPublisher spawns a device inventory publisher that iterates over the device objects received
	// on the collector channel and publishes them to the configured publish target.
	RunInventoryPublisher(ctx context.Context) error

	// PublishOne publishes the given device information to the configured publish target
	PublishOne(ctx context.Context, device *model.Asset) error
}
