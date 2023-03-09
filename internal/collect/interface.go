package collect

import (
	"context"

	"github.com/metal-toolbox/alloy/internal/model"
)

// Collector interface defines methods to collect device inventory
type Collector interface {
	// InventoryRemote spawns an iterator that listens on the AssetCh and
	// collects inventory remotely (through its BMC) for each of the assets until the AssetCh is closed,
	// the collected inventory is then sent on the collector channel to be published.
	InventoryRemote(ctx context.Context) error

	// InventoryLocal collects and returns inventory and bios configuration on the local host for the given asset.
	InventoryLocal(ctx context.Context) (*model.Asset, error)

	// ForAsset runs the asset inventory collection for the given asset and updates the asset object with collected inventory or an error if any.
	//
	// This method is to replace the Inventory* methods.
	ForAsset(ctx context.Context, asset *model.Asset) error

	// SetMockGetter sets a mock device inventory getter to be used for tests.
	SetMockGetter(getter interface{})

	// SetAssetChannel sets/overrides the asset channel on the collector
	SetAssetChannel(assetCh chan *model.Asset)
}
