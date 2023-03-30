package store

import (
	"context"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/store/csv"
	"github.com/metal-toolbox/alloy/internal/store/mock"
	"github.com/metal-toolbox/alloy/internal/store/serverservice"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	ErrStore = errors.New("store error")
)

type Repository interface {
	// Kind returns the repository store kind.
	Kind() model.StoreKind

	// AssetByID returns one asset from the inventory identified by its identifier.
	AssetByID(ctx context.Context, assetID string, fetchBmcCredentials bool) (*model.Asset, error)

	// AssetByOffsetLimit queries the inventory for the asset(s) at the given offset, limit values.
	AssetsByOffsetLimit(ctx context.Context, offset, limit int) (assets []*model.Asset, totalAssets int, err error)

	// AssetUpdate inserts and updates collected data for the asset in the store.
	AssetUpdate(ctx context.Context, asset *model.Asset) error
}

func NewRepository(ctx context.Context, storeKind model.StoreKind, appKind model.AppKind, cfg *app.Configuration, logger *logrus.Logger) (Repository, error) {
	switch storeKind {
	case model.StoreKindServerservice:
		return serverservice.New(ctx, appKind, cfg.ServerserviceOptions, logger)

	case model.StoreKindCsv:
		return csv.New(ctx, cfg.CsvFile, logger)

	case model.StoreKindMock:
		assets := 10
		return mock.New(assets)

	default:
		return nil, errors.Wrap(ErrStore, "unsupported store kind: "+string(storeKind))
	}
}
