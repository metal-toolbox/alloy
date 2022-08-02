package publish

import (
	"context"
)

// Publisher defines an interface for device inventory to be published to remote endpoints.
type Publisher interface {
	// Run spawns a device inventory publisher
	Run(ctx context.Context) error
}
