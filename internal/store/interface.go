package store

import (
	"context"

	"github.com/metal-toolbox/alloy/internal/model"
)

type Repository interface {
	// AssetByID returns one asset from the inventory identified by its identifier.
	AssetByID(ctx context.Context, assetID string, fetchBmcCredentials bool) (*model.Asset, error)

	// AssetByOffsetLimit queries the inventory for the asset(s) at the given offset, limit values.
	AssetsByOffsetLimit(ctx context.Context, offset, limit int) (assets []*model.Asset, totalAssets int, err error)
}
