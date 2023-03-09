package asset

import (
	"context"

	"github.com/metal-toolbox/alloy/internal/model"
)

// Getter interface declares methods to be implemented for asset retrieval from the asset store.
type Getter interface {
	// All runs the asset getter which retrieves all assets in the inventory store and sends them over the asset channel.
	ListAll(ctx context.Context) error

	// ByIDs runs the asset getter which retrieves all assets based on the list of IDs and sends them over the asset channel.
	ListByIDs(ctx context.Context, assetIDs []string) error

	// AssetByID returns one asset from the inventory identified by its identifier.
	AssetByID(ctx context.Context, assetID string, fetchBmcCredentials bool) (*model.Asset, error)

	// SetClient sets the given client as the Getter client to enable mocking for tests.
	SetClient(interface{})

	// SetAssetChannel sets/overrides the asset channel on the asset getter
	SetAssetChannel(assetCh chan *model.Asset)
}
