package runner

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/store"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
)

var (
	// batchSize is the default number of assets to retrieve per request
	batchSize = 10

	// delay between server service requests.
	delayBetweenRequests = 2 * time.Second
	tracer               trace.Tracer

	ErrFetcherQuery   = errors.New("Error querying asset data from inventory")
	stageLabelFetcher = prometheus.Labels{"stage": "fetcher"}
)

type AssetFetcher struct {
	store   store.Repository
	assetCh chan<- *model.Asset
	logger  *logrus.Logger
}

// NewAssetFetcher returns an AssetFetcher with methods, and an asset channel over which assets can be read from.
func NewAssetFetcher(store store.Repository, logger *logrus.Logger) *AssetFetcher {
	return &AssetFetcher{store: store, logger: logger, assetCh: make(chan *model.Asset)}
}

// AssetChannel returns the channel to read assets from when the fetcher is invoked through its Iter* method.
func (s *AssetFetcher) AssetChannel() <-chan *model.Asset {
	return s.AssetChannel()
}

// IterInBatches queries the store for assets in batches, returning them over the assetCh
func (s *AssetFetcher) IterInBatches(ctx context.Context, pauser *Pauser) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "IterateInBatches()")
	defer span.End()

	// first request to figures out total items
	offset := 1

	assets, total, err := s.store.AssetsByOffsetLimit(ctx, offset, 1)
	if err != nil {
		// count serverService query errors
		if errors.Is(err, ErrFetcherQuery) {
			metrics.FetcherQueryErrorCount.With(stageLabelFetcher).Inc()
		}

		return err
	}

	// count assets retrieved
	metrics.ServerServiceAssetsRetrieved.With(stageLabelFetcher).Add(float64(len(assets)))

	// submit the assets collected in the first request
	for _, asset := range assets {
		s.assetCh <- asset

		// count assets sent to the collector
		metrics.AssetsSent.With(stageLabelFetcher).Inc()
	}

	if total <= 1 {
		return nil
	}

	var finalBatch bool

	// continue from offset 2
	offset = 2
	fetched := 1

	for {
		// final batch
		if total < batchSize {
			batchSize = total
			finalBatch = true
		}

		if (fetched + batchSize) >= total {
			finalBatch = true
		}

		// idle when pause flag is set and context isn't canceled.
		for pauser.Value() && ctx.Err() == nil {
			time.Sleep(1 * time.Second)
		}

		// context canceled
		if ctx.Err() != nil {
			break
		}

		// pause between spawning workers - skip delay for tests
		if os.Getenv("TEST_ENV") == "" {
			time.Sleep(delayBetweenRequests)
		}

		assets, _, err := s.store.AssetsByOffsetLimit(ctx, offset, batchSize)
		if err != nil {
			if errors.Is(err, ErrFetcherQuery) {
				metrics.FetcherQueryErrorCount.With(stageLabelFetcher).Inc()
			}

			s.logger.Warn(err)
		}

		s.logger.WithFields(logrus.Fields{
			"offset":  offset,
			"limit":   batchSize,
			"total":   total,
			"fetched": fetched,
			"got":     len(assets),
		}).Trace()

		// count assets retrieved
		metrics.ServerServiceAssetsRetrieved.With(stageLabelFetcher).Add(float64(len(assets)))

		for _, asset := range assets {
			s.assetCh <- asset

			// count assets sent to collector
			metrics.AssetsSent.With(stageLabelFetcher).Inc()
		}

		if finalBatch {
			break
		}

		offset++

		fetched += batchSize
	}

	close(s.assetCh)

	return nil
}

// ListByIDs implements the Getter interface to query the inventory for the assetIDs and return found assets over the asset channel.
func (s *AssetFetcher) ListByIDs(ctx context.Context, assetIDs []string, assetCh chan<- *model.Asset) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "ListByIDs()")
	defer span.End()

	// close assetCh to notify consumers
	//defer close(s.assetCh)

	// submit inventory collection to worker pool
	for _, assetID := range assetIDs {
		assetID := assetID

		// idle when pauser flag is set, unless context is canceled.
		//for s.pauser.Value() && ctx.Err() == nil {
		// 	time.Sleep(1 * time.Second)
		// }

		// context canceled
		if ctx.Err() != nil {
			break
		}

		// lookup asset by its ID from the inventory asset store
		asset, err := s.store.AssetByID(ctx, assetID, true)
		if err != nil {
			// count serverService query errors
			if errors.Is(err, ErrFetcherQuery) {
				metrics.FetcherQueryErrorCount.With(stageLabelFetcher).Inc()
			}

			s.logger.WithField("serverID", assetID).Warn(err)

			continue
		}

		// count assets retrieved
		metrics.ServerServiceAssetsRetrieved.With(stageLabelFetcher).Inc()

		// send asset for inventory collection
		assetCh <- asset

		// count assets sent to collector
		metrics.AssetsSent.With(stageLabelFetcher).Inc()
	}

	return nil
}
