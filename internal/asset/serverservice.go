package asset

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/helpers"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

const (
	// delay between server service requests.
	delayBetweenRequests = 2 * time.Second
	// SourceKindServerService identifies a server service source.
	SourceKindServerService = "serverService"

	// server service attribute namespace to look up
	attributeNamespace = "servers"
	// server model attribute key
	attributeKeyModel = "Model"
	// server vendor attribute key
	attributeKeyVendor = "Vendor"
	// server BMC address attribute key
	attributeKeyBMCIPAddress = "BMCIPAddress"
)

var (
	// batchSize is the default number of assets to retrieve per request
	batchSize = 5
	// ErrServerServiceQuery is returned when a server service query fails.
	ErrServerServiceQuery = errors.New("serverService query error")
	// ErrServerServiceObject is returned when a server service object is found to be missing attributes.
	ErrServerServiceObject = errors.New("serverService object error")
)

// serverServiceGetter is an inventory asset getter
type serverServiceGetter struct {
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
	logger := alloy.Logger.WithField("component", "getter.serverService")

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
	// close assetCh to notify consumers
	defer close(s.assetCh)

	// channel for routines spawned to indicate completion
	doneCh := make(chan struct{})

	// count of routines spawned to retrieve assets
	var dispatched int32

	// submit inventory collection to worker pool
	for _, assetID := range assetIDs {
		assetID := assetID

		// increment wait group
		s.syncWg.Add(1)

		// increment spawned count
		atomic.AddInt32(&dispatched, 1)

		s.workers.Submit(
			func() {
				defer s.syncWg.Done()
				defer func() { doneCh <- struct{}{} }()

				// lookup asset by its ID from the inventory asset store
				asset, err := s.client.AssetByID(ctx, assetID)
				if err != nil {
					s.logger.Warn(err)
				}

				// send asset for inventory collection
				s.assetCh <- asset
			},
		)
	}

	for dispatched > 0 {
		<-doneCh
		atomic.AddInt32(&dispatched, ^int32(0))
	}

	return nil
}

// ListAll implements the Getter interface to query the inventory and return assets over the asset channel.
func (s *serverServiceGetter) ListAll(ctx context.Context) error {
	// close assetCh to notify consumers
	defer close(s.assetCh)

	// channel for routines spawned to indicate completion
	doneCh := make(chan struct{})

	// count of routines spawned to retrieve assets
	var dispatched int32

	// increment wait group
	s.syncWg.Add(1)

	// increment spawned count
	atomic.AddInt32(&dispatched, 1)

	func(dispatched *int32) {
		s.workers.Submit(

			func() {
				defer s.syncWg.Done()
				defer func() { doneCh <- struct{}{} }()

				err := s.dispatcher(ctx, dispatched, doneCh)
				if err != nil {
					s.logger.Warn(err)
				}
			},
		)
	}(&dispatched)

	for dispatched > 0 {
		<-doneCh
		atomic.AddInt32(&dispatched, ^int32(0))
	}

	return nil
}

// dispatcher spawns workers to fetch assets
//
// nolint:gocyclo // this method has various cases to consider and shared context information which is ideal to keep together.
func (s *serverServiceGetter) dispatcher(ctx context.Context, dispatched *int32, doneCh chan<- struct{}) error {
	// first request to figures out total items
	offset := 1

	assets, total, err := s.client.AssetsByOffsetLimit(ctx, offset, 1)
	if err != nil {
		return err
	}

	// submit the assets collected in the first request
	for _, asset := range assets {
		s.assetCh <- asset
	}

	if total <= 1 {
		return nil
	}

	var finalBatch bool

	// continue from offset 2
	offset = 2
	fetched := 1

	testingLimit := 5

	for {
		// final batch
		if total < batchSize {
			batchSize = total
			finalBatch = true
		}

		if (fetched + batchSize) >= total {
			finalBatch = true
		}

		for s.workers.WaitingQueueSize() > s.config.ServerService.Concurrency {
			// context canceled
			if ctx.Err() != nil {
				break
			}

			s.logger.WithFields(logrus.Fields{
				"queue size":  s.workers.WaitingQueueSize(),
				"concurrency": s.config.ServerService.Concurrency,
			}).Debug("delay for queue size to drop..")

			// nolint:gomnd // delay is a magic number
			time.Sleep(5 * time.Second)
		}

		// context canceled
		if ctx.Err() != nil {
			break
		}

		// increment wait group
		s.syncWg.Add(1)

		// increment spawned count
		atomic.AddInt32(dispatched, 1)

		// pause between spawning workers - skip delay for tests
		if os.Getenv("TEST_ENV") == "" {
			time.Sleep(delayBetweenRequests)
		}

		// spawn worker with the offset, limit parameters
		// this is done within a closure to capture the offset, limit values
		func(pOffset, limit int) {
			s.workers.Submit(
				func() {
					defer s.syncWg.Done()
					defer func() { doneCh <- struct{}{} }()

					assets, _, err := s.client.AssetsByOffsetLimit(ctx, pOffset, limit)
					if err != nil {
						s.logger.Warn(err)
					}

					s.logger.WithFields(logrus.Fields{
						"offset":  pOffset,
						"limit":   limit,
						"total":   total,
						"fetched": fetched,
						"got":     len(assets),
					}).Trace()

					for _, asset := range assets {
						s.assetCh <- asset
					}
				},
			)
		}(offset, batchSize)

		if finalBatch {
			break
		}

		// For testing
		if fetched > testingLimit {
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
	sid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}

	// get server
	server, _, err := r.client.Get(ctx, sid)
	if err != nil {
		return nil, errors.Wrap(ErrServerServiceQuery, err.Error())
	}

	// get bmc secret
	secret, _, err := r.client.GetSecret(ctx, sid, serverservice.ServerSecretTypeBMC)
	if err != nil {
		return nil, errors.Wrap(ErrServerServiceQuery, err.Error())
	}

	return toAsset(server, secret)
}

// assetByID queries serverService for the hardware asset by ID and returns an Asset object
func (r *serverServiceClient) AssetsByOffsetLimit(ctx context.Context, offset, limit int) (assets []*model.Asset, totalAssets int, err error) {
	params := &serverservice.ServerListParams{
		FacilityCode: r.facilityCode,
		AttributeListParams: []serverservice.AttributeListParams{
			{
				Namespace: attributeNamespace,
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
		return nil, 0, errors.Wrap(ErrServerServiceQuery, err.Error())
	}

	assets = make([]*model.Asset, 0, len(servers))

	// collect bmc secrets and structure as alloy asset
	for _, server := range serverPtrSlice(servers) {
		secret, _, err := r.client.GetSecret(ctx, server.UUID, serverservice.ServerSecretTypeBMC)
		if err != nil {
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

func toAsset(server *serverservice.Server, secret *serverservice.ServerSecret) (*model.Asset, error) {
	// attribute data is unpacked into this map
	data := map[string]string{}

	for _, attribute := range server.Attributes {
		if attribute.Namespace == attributeNamespace {
			if err := json.Unmarshal(attribute.Data, &data); err != nil {
				return nil, errors.Wrap(ErrServerServiceObject, err.Error())
			}
		}
	}

	if len(data) == 0 {
		return nil, errors.Wrap(ErrServerServiceObject, "expected server attributes, got none")
	}

	if data[attributeKeyBMCIPAddress] == "" {
		return nil, errors.Wrap(ErrServerServiceObject, "expected attribute empty: "+attributeKeyBMCIPAddress)
	}

	if err := validateRequiredAttributes(server, secret); err != nil {
		return nil, errors.Wrap(ErrServerServiceObject, err.Error())
	}

	secretParts := strings.Split(secret.Value, ":")

	return &model.Asset{
		ID:          server.UUID.String(),
		Model:       data[attributeKeyModel],
		Vendor:      data[attributeKeyVendor],
		Facility:    server.FacilityCode,
		BMCUsername: secretParts[0],
		BMCPassword: secretParts[1],
		BMCAddress:  net.ParseIP(data[attributeKeyBMCIPAddress]),
	}, nil
}

func validateRequiredAttributes(server *serverservice.Server, secret *serverservice.ServerSecret) error {
	if server == nil {
		return errors.New("server object nil")
	}

	if secret == nil {
		return errors.New("server secret object nil")
	}

	if len(server.Attributes) == 0 {
		return errors.New("server attributes slice empty")
	}

	if secret.Value == "" {
		return errors.New("BMC secret empty")
	}

	if !strings.Contains(secret.Value, ":") {
		return errors.New("invalid BMC secret format, want <username>:<password>")
	}

	secretParts := strings.Split(secret.Value, ":")
	if len(secretParts) <= 1 {
		return errors.New("invalid BMC secret format, want <username>:<password>")
	}

	if secretParts[0] == "" {
		return errors.New("invalid BMC secret, username empty")
	}

	if secretParts[1] == "" {
		return errors.New("invalid BMC secret, password empty")
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
