package publish

import (
	"context"

	"github.com/metal-toolbox/alloy/internal/model"
)

// Publisher defines an interface for device inventory to be published to remote endpoints.
type Publisher interface {
	// Publish publishes the given device information to the configured publish target
	Publish(ctx context.Context, device *model.Asset) error
}
