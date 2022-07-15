package asset

import (
	"context"
)

// Getter interface declares methods to be implemented for asset retrieval from the asset store.
type Getter interface {
	// All runs the asset getter which retrieves all assets in the inventory store and sends them over the asset channel.
	ListAll(ctx context.Context) error

	// ByIDs runs the asset getter which retrieves all assets based on the list of IDs and sends them over the asset channel.
	ListByIDs(ctx context.Context, assetIDs []string) error

	// SetClient sets the given client as the Getter client to enable mocking for tests.
	SetClient(interface{})
}
