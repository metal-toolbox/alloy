package publish

import (
	"context"

	"github.com/metal-toolbox/alloy/internal/model"
)

// Publisher defines an interface for device inventory to be published to remote endpoints.
type Publisher interface {
	// Run spawns a device inventory publisher
	Run(ctx context.Context) error

	// PublishOne publishes the given device information to the configured publish target
	PublishOne(ctx context.Context, device *model.AssetDevice) error
}
