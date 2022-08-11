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

	// InventoryLocal collects and returns inventory on the local host for the given asset.
	InventoryLocal(ctx context.Context) (*model.AssetDevice, error)

	// SetMockGetter sets a mock device inventory getter to be used for tests.
	SetMockGetter(getter interface{})
}
