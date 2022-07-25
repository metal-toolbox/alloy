package collect

import (
	"context"
)

// Collector interface defines methods to collect device inventory
type Collector interface {
	// Inventory listens on the AssetCh collects inventory for all assets until the AssetCh is closed.
	Inventory(ctx context.Context) error
	// SetMockGetter sets a mock device inventory getter to be used for tests.
	SetMockGetter(getter interface{})
}
