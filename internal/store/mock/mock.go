package mock

import (
	"context"

	"github.com/metal-toolbox/alloy/internal/model"
)

// Mock is a mock store
type Mock struct {
	TotalAssets   int
	UpdatedAssets int
}

func New(totalAssets int) (*Mock, error) {
	return &Mock{TotalAssets: totalAssets}, nil
}

// Kind returns the repository store kind.
func (m *Mock) Kind() model.StoreKind {
	return model.StoreKindMock
}

// AssetByID returns one asset from the inventory identified by its identifier.
func (m *Mock) AssetByID(ctx context.Context, assetID string, fetchBmcCredentials bool) (*model.Asset, error) {
	return &model.Asset{ID: assetID}, nil
}

// AssetByOffsetLimit queries the inventory for the asset(s) at the given offset, limit values.
func (m *Mock) AssetsByOffsetLimit(ctx context.Context, offset, limit int) (assets []*model.Asset, totalAssets int, err error) {
	var count int

	if m.TotalAssets == 0 {
		return assets, m.TotalAssets, nil
	}

	for i := offset; i <= m.TotalAssets; i++ {
		assets = append(assets, &model.Asset{})
		count++

		if count >= limit {
			break
		}
	}

	return assets, m.TotalAssets, nil
}

// AssetUpdate inserts and updates collected data for the asset in the store.
func (m *Mock) AssetUpdate(ctx context.Context, asset *model.Asset) error {
	m.UpdatedAssets++

	return nil
}
