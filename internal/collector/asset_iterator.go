package collector

import (
	"context"
	"errors"
	"time"

	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/store"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
)

// TODO: this should run as a separate process -
// the iterator can be run as the alloy-scheduler, which periodically
// sets conditions for data collection on servers, and the collector just full fills the conditions.

var (
	ErrFetcherQuery   = errors.New("error querying asset data from store")
	stageLabelFetcher = prometheus.Labels{"stage": "fetcher"}
)

// AssetIterator holds methods to recurse over assets in a store and return them over the asset channel.
type AssetIterator struct {
	store   store.Repository
	assetCh chan *model.Asset
	logger  *logrus.Logger
}

// NewAssetIterator is a constructor method that returns an AssetIterator.
//
// The returned AssetIterator will recurse over all assets in the store and send them over the asset channel,
// The caller of this method should invoke AssetChannel() to retrieve the channel to read assets from.
func NewAssetIterator(repository store.Repository, logger *logrus.Logger) *AssetIterator {
	return &AssetIterator{store: repository, logger: logger, assetCh: make(chan *model.Asset, 1)}
}

// Channel returns the channel to read assets from when the fetcher is invoked through its Iter* method.
func (s *AssetIterator) Channel() <-chan *model.Asset {
	return s.assetCh
}

// IterInBatches queries the store for assets in batches, returning them over the assetCh
//
// nolint:gocyclo // for now it makes sense to have the iter method logic in one method
func (s *AssetIterator) IterInBatches(ctx context.Context, batchSize int, pauser *Pauser) {
	defer close(s.assetCh)

	tracer := otel.Tracer("collector.AssetIterator")
	ctx, span := tracer.Start(ctx, "IterInBatches()")

	defer span.End()

	assets, total, err := s.store.AssetsByOffsetLimit(ctx, 1, batchSize)
	if err != nil {
		// count serverService query errors
		if errors.Is(err, ErrFetcherQuery) {
			metrics.FleetDBAPIQueryErrorCount.With(stageLabelFetcher).Inc()
		}

		s.logger.WithError(err).Error(ErrFetcherQuery)

		return
	}

	// count assets retrieved
	metrics.FleetDBAPIAssetsRetrieved.With(stageLabelFetcher).Add(float64(len(assets)))

	// submit the assets collected in the first request
	for _, asset := range assets {
		s.assetCh <- asset

		// count assets sent to the collector
		metrics.AssetsSent.With(stageLabelFetcher).Inc()
	}

	// all assets fetched in first query
	if len(assets) == total || total <= batchSize {
		return
	}

	iterations := total / batchSize
	limit := batchSize

	s.logger.WithFields(logrus.Fields{
		"total":      total,
		"iterations": iterations,
		"limit":      limit,
	}).Trace()

	// continue from offset 2
	for offset := 2; offset < iterations+1; offset++ {
		// idle when pause flag is set and context isn't canceled.
		for pauser.Value() && ctx.Err() == nil {
			time.Sleep(1 * time.Second)
		}

		// context canceled
		if ctx.Err() != nil {
			s.logger.WithError(err).Error("aborting collection")

			break
		}

		assets, _, err := s.store.AssetsByOffsetLimit(ctx, offset, limit)
		if err != nil {
			if errors.Is(err, ErrFetcherQuery) {
				metrics.FleetDBAPIQueryErrorCount.With(stageLabelFetcher).Inc()
			}

			s.logger.WithError(err).Warn(ErrFetcherQuery)
		}

		s.logger.WithFields(logrus.Fields{
			"offset": offset,
			"limit":  limit,
			"total":  total,
			"got":    len(assets),
		}).Trace()

		if len(assets) == 0 {
			break
		}

		// count assets retrieved
		metrics.FleetDBAPIAssetsRetrieved.With(stageLabelFetcher).Add(float64(len(assets)))

		for _, asset := range assets {
			s.assetCh <- asset

			// count assets sent to collector
			metrics.AssetsSent.With(stageLabelFetcher).Inc()
		}
	}
}
