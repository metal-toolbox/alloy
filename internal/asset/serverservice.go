package asset

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"sync"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/helpers"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

const (
	// delay between server service requests.
	delayBetweenRequests = 2 * time.Second
	// SourceKindServerService identifies a server service source.
	SourceKindServerService = "serverService"

	// server service attribute to look up the BMC IP Address in
	bmcAttributeNamespace = "sh.hollow.bmc_info"
	// server BMC address attribute key
	attributeKeyBMCIPAddress = "address"

	// server model attribute key
	attributeKeyModel = "Model"
	// server vendor attribute key
	attributeKeyVendor = "Vendor"
)

var (
	// batchSize is the default number of assets to retrieve per request
	batchSize = 10
	// ErrServerServiceQuery is returned when a server service query fails.
	ErrServerServiceQuery = errors.New("serverService query error")
	// ErrServerServiceObject is returned when a server service object is found to be missing attributes.
	ErrServerServiceObject = errors.New("serverService object error")
	// The serverservice asset getter tracer
	tracer trace.Tracer
)

func init() {
	tracer = otel.Tracer("getter-serverservice")
}

// serverServiceGetter is an inventory asset getter
type serverServiceGetter struct {
	pauser  *helpers.Pauser
	client  serverServiceRequestor
	logger  *logrus.Entry
	config  *model.Config
	syncWg  *sync.WaitGroup
	assetCh chan<- *model.Asset
	workers *workerpool.WorkerPool
}

// serverServiceRequestor interface defines methods to lookup inventory assets
//
// the methods are exported to enable mock implementations
type serverServiceRequestor interface {
	AssetByID(ctx context.Context, id string) (asset *model.Asset, err error)
	AssetsByOffsetLimit(ctx context.Context, offset, limit int) (assets []*model.Asset, totalAssets int, err error)
}

// NewServerServiceGetter returns an asset getter to retrieve asset information from serverService for inventory collection.
func NewServerServiceGetter(ctx context.Context, alloy *app.App) (Getter, error) {
	logger := alloy.Logger.WithField("component", "getter-serverService")

	client, err := helpers.NewServerServiceClient(alloy.Config, logger)
	if err != nil {
		return nil, err
	}

	if facility := os.Getenv("SERVERSERVICE_FACILITY_CODE"); facility != "" {
		alloy.Config.ServerService.FacilityCode = facility
	}

	if alloy.Config.ServerService.FacilityCode == "" {
		return nil, errors.Wrap(model.ErrConfig, "expected serverService facility code, got empty")
	}

	s := &serverServiceGetter{
		pauser:  alloy.AssetGetterPause,
		logger:  logger,
		syncWg:  alloy.SyncWg,
		config:  alloy.Config,
		assetCh: alloy.AssetCh,
		client:  &serverServiceClient{client, logger, alloy.Config.ServerService.FacilityCode},
	}

	if alloy.Config.ServerService.Concurrency == 0 {
		alloy.Config.ServerService.Concurrency = model.ConcurrencyDefault
	}

	s.workers = workerpool.New(alloy.Config.ServerService.Concurrency)

	return s, nil
}

// SetClient implements the Getter interface to set the serverServiceRequestor
func (s *serverServiceGetter) SetClient(c interface{}) {
	s.client = c.(serverServiceRequestor)
}

// ListByIDs implements the Getter interface to query the inventory for the assetIDs and return found assets over the asset channel.
func (s *serverServiceGetter) ListByIDs(ctx context.Context, assetIDs []string) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "ListByIDs()")
	defer span.End()

	// close assetCh to notify consumers
	defer close(s.assetCh)

	// submit inventory collection to worker pool
	for _, assetID := range assetIDs {
		assetID := assetID

		// idle when pauser flag is set, unless context is canceled.
		for s.pauser.Value() && ctx.Err() == nil {
			time.Sleep(1 * time.Second)
		}

		// context canceled
		if ctx.Err() != nil {
			break
		}

		// lookup asset by its ID from the inventory asset store
		asset, err := s.client.AssetByID(ctx, assetID)
		if err != nil {
			// count serverService query errors
			if errors.Is(err, ErrServerServiceQuery) {
				metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()
			}

			s.logger.WithField("serverID", assetID).Warn(err)

			continue
		}

		// count assets retrieved
		metrics.ServerServiceAssetsRetrieved.With(stageLabel).Inc()

		// send asset for inventory collection
		s.assetCh <- asset

		// count assets sent to collector
		metrics.AssetsSent.With(stageLabel).Inc()
	}

	return nil
}

// ListAll implements the Getter interface to query the inventory and return assets over the asset channel.
func (s *serverServiceGetter) ListAll(ctx context.Context) error {
	// add child span
	ctx, span := tracer.Start(ctx, "ListAll()")
	defer span.End()

	// close assetCh to notify consumers
	defer close(s.assetCh)

	// count tasks dispatched
	metrics.TasksDispatched.With(stageLabel).Inc()

	err := s.dispatchQueries(ctx)
	if err != nil {
		s.logger.Warn(err)
	}

	return nil
}

// dispatchQueries spawns workers to fetch assets
//
// nolint:gocyclo // this method has various cases to consider and shared context information which is ideal to keep together.
func (s *serverServiceGetter) dispatchQueries(ctx context.Context) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "dispatcher()")
	defer span.End()

	// first request to figures out total items
	offset := 1

	assets, total, err := s.client.AssetsByOffsetLimit(ctx, offset, 1)
	if err != nil {
		// count serverService query errors
		if errors.Is(err, ErrServerServiceQuery) {
			metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()
		}

		return err
	}

	// count assets retrieved
	metrics.ServerServiceAssetsRetrieved.With(stageLabel).Add(float64(len(assets)))

	// submit the assets collected in the first request
	for _, asset := range assets {
		s.assetCh <- asset

		// count assets sent to the collector
		metrics.AssetsSent.With(stageLabel).Inc()
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
		for s.pauser.Value() && ctx.Err() == nil {
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

		assets, _, err := s.client.AssetsByOffsetLimit(ctx, offset, batchSize)
		if err != nil {
			if errors.Is(err, ErrServerServiceQuery) {
				metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()
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
		metrics.ServerServiceAssetsRetrieved.With(stageLabel).Add(float64(len(assets)))

		for _, asset := range assets {
			s.assetCh <- asset

			// count assets sent to collector
			metrics.AssetsSent.With(stageLabel).Inc()
		}

		if finalBatch {
			break
		}

		offset++

		fetched += batchSize
	}

	return nil
}

// serverServiceClient implements the serverServiceRequestor interface
type serverServiceClient struct {
	client       *serverservice.Client
	logger       *logrus.Entry
	facilityCode string
}

// assetByID queries serverService for the hardware asset by ID and returns an Asset object
func (r *serverServiceClient) AssetByID(ctx context.Context, id string) (*model.Asset, error) {
	// attach child span
	ctx, span := tracer.Start(ctx, "AssetByID()")
	defer span.End()

	sid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}

	// get server
	server, _, err := r.client.Get(ctx, sid)
	if err != nil {
		span.SetStatus(codes.Error, "Get() server failed")

		return nil, errors.Wrap(ErrServerServiceQuery, err.Error())
	}

	// get bmc secret
	secret, _, err := r.client.GetCredential(ctx, sid, serverservice.ServerCredentialTypeBMC)
	if err != nil {
		span.SetStatus(codes.Error, "GetSecret() failed")

		return nil, errors.Wrap(ErrServerServiceQuery, err.Error())
	}

	return toAsset(server, secret)
}

// assetByID queries serverService for the hardware asset by ID and returns an Asset object
func (r *serverServiceClient) AssetsByOffsetLimit(ctx context.Context, offset, limit int) (assets []*model.Asset, totalAssets int, err error) {
	// attach child span
	ctx, span := tracer.Start(ctx, "AssetsByOffsetLimit()")
	span.SetAttributes(
		attribute.Int("offset", offset),
		attribute.Int("limit", limit),
	)

	defer span.End()

	params := &serverservice.ServerListParams{
		FacilityCode: r.facilityCode,
		AttributeListParams: []serverservice.AttributeListParams{
			{
				Namespace: bmcAttributeNamespace,
			},
		},
		PaginationParams: &serverservice.PaginationParams{
			Limit: limit,
			Page:  offset,
		},
	}

	// list servers
	servers, response, err := r.client.List(ctx, params)
	if err != nil {
		span.SetStatus(codes.Error, "List() servers failed")

		return nil, 0, errors.Wrap(ErrServerServiceQuery, err.Error())
	}

	assets = make([]*model.Asset, 0, len(servers))

	// collect bmc secrets and structure as alloy asset
	for _, server := range serverPtrSlice(servers) {
		secret, _, err := r.client.GetCredential(ctx, server.UUID, serverservice.ServerCredentialTypeBMC)
		if err != nil {
			span.SetStatus(codes.Error, "GetCredential() failed")

			return nil, 0, errors.Wrap(ErrServerServiceQuery, err.Error())
		}

		asset, err := toAsset(server, secret)
		if err != nil {
			r.logger.Warn(err)
			continue
		}

		assets = append(assets, asset)
	}

	return assets, int(response.TotalRecordCount), nil
}

func toAsset(server *serverservice.Server, secret *serverservice.ServerCredential) (*model.Asset, error) {
	// attribute data is unpacked into this map
	data := map[string]string{}

	for _, attribute := range server.Attributes {
		if attribute.Namespace == bmcAttributeNamespace {
			if err := json.Unmarshal(attribute.Data, &data); err != nil {
				return nil, errors.Wrap(ErrServerServiceObject, "bmc address attribute: "+err.Error())
			}
		}
	}

	if len(data) == 0 {
		return nil, errors.Wrap(ErrServerServiceObject, "expected server attributes with BMC address, got none")
	}

	if data[attributeKeyBMCIPAddress] == "" {
		return nil, errors.Wrap(ErrServerServiceObject, "expected BMC address attribute empty")
	}

	if err := validateRequiredAttributes(server, secret); err != nil {
		return nil, errors.Wrap(ErrServerServiceObject, err.Error())
	}

	return &model.Asset{
		ID:          server.UUID.String(),
		Model:       data[attributeKeyModel],
		Vendor:      data[attributeKeyVendor],
		Facility:    server.FacilityCode,
		BMCUsername: secret.Username,
		BMCPassword: secret.Password,
		BMCAddress:  net.ParseIP(data[attributeKeyBMCIPAddress]),
	}, nil
}

func validateRequiredAttributes(server *serverservice.Server, secret *serverservice.ServerCredential) error {
	if server == nil {
		return errors.New("server object nil")
	}

	if secret == nil {
		return errors.New("server secret object nil")
	}

	if len(server.Attributes) == 0 {
		return errors.New("server attributes slice empty")
	}

	if secret.Username == "" {
		return errors.New("BMC username field empty")
	}

	if secret.Password == "" {
		return errors.New("BMC password field empty")
	}

	return nil
}

// serverPtrSlice returns a slice of pointers to serverservice.Server
//
// The server service server list methods return a slice of server objects,
// this helper method is to reduce the amount of copying of component objects (~176 bytes each) when passed around between methods and range loops,
// while it seems like a minor optimization, it also keeps the linter happy.
func serverPtrSlice(servers []serverservice.Server) []*serverservice.Server {
	returned := make([]*serverservice.Server, 0, len(servers))

	// nolint:gocritic // the copying has to be done somewhere
	for _, s := range servers {
		s := s
		returned = append(returned, &s)
	}

	return returned
}
