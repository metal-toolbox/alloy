package serverservice

import (
	"context"
	"encoding/json"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	serverserviceapi "go.hollow.sh/serverservice/pkg/api/v1"
)

const (
	// delay between server service requests.
	delayBetweenRequests = 2 * time.Second
	// SourceKindServerService identifies a server service source.
	SourceKindServerService = "serverService"

	// server service attribute to look up the BMC IP Address in
	bmcAttributeNamespace = "sr.hollow.bmc_info"

	// server server service BMC address attribute key found under the bmcAttributeNamespace
	bmcIPAddressAttributeKey = "address"
)

var (

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

// serverServiceStore is an asset inventory store
type serverServiceStore struct {
	apiclient *serverServiceClient
	logger    *logrus.Entry
	config    *app.Configuration
}

// NewServerServiceStore returns a serverservice store queryor to lookup and publish assets to, from the store.
func NewServerServiceStore(ctx context.Context, alloy *app.App) (*serverServiceStore, error) {
	logger := app.NewLogrusEntryFromLogger(
		logrus.Fields{"component": "getter-serverService"},
		alloy.Logger,
	)

	client, err := NewServerServiceClient(ctx, &alloy.Config.ServerserviceOptions, logger)
	if err != nil {
		return nil, err
	}

	s := &serverServiceStore{
		logger: logger,
		config: alloy.Config,
		apiclient: &serverServiceClient{
			client,
			logger,
			alloy.Config.ServerserviceOptions.FacilityCode,
		},
	}

	return s, nil
}

// AssetByID returns one asset from the inventory identified by its identifier.
func (s *serverServiceStore) AssetByID(ctx context.Context, assetID string, fetchBmcCredentials bool) (*model.Asset, error) {
	// attach child span
	ctx, span := tracer.Start(ctx, "AssetByID()")
	defer span.End()

	return s.apiclient.AssetByID(ctx, assetID, fetchBmcCredentials)
}

// AssetByID returns one asset from the inventory identified by its identifier.
func (s *serverServiceStore) AssetsByOffsetLimit(ctx context.Context, offset, limit int) (assets []*model.Asset, totalAssets int, err error) {
	// attach child span
	ctx, span := tracer.Start(ctx, "AssetByOffsetLimit()")
	defer span.End()

	return s.apiclient.AssetsByOffsetLimit(ctx, offset, limit)
}

// UpsertAsset inserts/updates asset data in the serverservice inventory store
func (s *serverServiceStore) UpsertAsset(ctx context.Context, asset *model.Asset) {
	// attach child span
	ctx, span := tracer.Start(ctx, "AssetByOffsetLimit()")
	defer span.End()

	s.apiclient.upsert(ctx, asset)
}

// serverServiceClient implements the serverServiceQueryor interface
type serverServiceClient struct {
	apiclient    *serverserviceapi.Client
	logger       *logrus.Entry
	facilityCode string
}

// assetByID queries serverService for the hardware asset by ID and returns an Asset object
func (r *serverServiceClient) AssetByID(ctx context.Context, id string, fetchBmcCredentials bool) (*model.Asset, error) {
	// attach child span
	ctx, span := tracer.Start(ctx, "AssetByID()")
	defer span.End()

	sid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}

	// get server
	server, _, err := r.apiclient.Get(ctx, sid)
	if err != nil {
		span.SetStatus(codes.Error, "Get() server failed")

		return nil, errors.Wrap(ErrServerServiceQuery, "error querying server attributes: "+err.Error())
	}

	var credential *serverserviceapi.ServerCredential

	if fetchBmcCredentials {
		var err error

		// get bmc credential
		credential, _, err = r.apiclient.GetCredential(ctx, sid, serverserviceapi.ServerCredentialTypeBMC)
		if err != nil {
			span.SetStatus(codes.Error, "GetCredential() failed")

			return nil, errors.Wrap(ErrServerServiceQuery, "error querying BMC credentials: "+err.Error())
		}
	}

	return toAsset(server, credential, fetchBmcCredentials)
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

	params := &serverserviceapi.ServerListParams{
		FacilityCode: r.facilityCode,
		AttributeListParams: []serverserviceapi.AttributeListParams{
			{
				Namespace: bmcAttributeNamespace,
			},
		},
		PaginationParams: &serverserviceapi.PaginationParams{
			Limit: limit,
			Page:  offset,
		},
	}

	// list servers
	servers, response, err := r.apiclient.List(ctx, params)
	if err != nil {
		span.SetStatus(codes.Error, "List() servers failed")

		return nil, 0, errors.Wrap(ErrServerServiceQuery, err.Error())
	}

	assets = make([]*model.Asset, 0, len(servers))

	// collect bmc secrets and structure as alloy asset
	for _, server := range serverPtrSlice(servers) {
		credential, _, err := r.apiclient.GetCredential(ctx, server.UUID, serverserviceapi.ServerCredentialTypeBMC)
		if err != nil {
			span.SetStatus(codes.Error, "GetCredential() failed")

			return nil, 0, errors.Wrap(ErrServerServiceQuery, err.Error())
		}

		asset, err := toAsset(server, credential, true)
		if err != nil {
			r.logger.Warn(err)
			continue
		}

		assets = append(assets, asset)
	}

	return assets, int(response.TotalRecordCount), nil
}

func toAsset(server *serverserviceapi.Server, credential *serverserviceapi.ServerCredential, expectCredentials bool) (*model.Asset, error) {
	if err := validateRequiredAttributes(server, credential, expectCredentials); err != nil {
		return nil, errors.Wrap(ErrServerServiceObject, err.Error())
	}

	serverAttributes, err := serverAttributes(server.Attributes, expectCredentials)
	if err != nil {
		return nil, errors.Wrap(ErrServerServiceObject, err.Error())
	}

	serverMetadataAttributes, err := serverMetadataAttributes(server.Attributes)
	if err != nil {
		return nil, errors.Wrap(ErrServerServiceObject, err.Error())
	}

	asset := &model.Asset{
		ID:       server.UUID.String(),
		Serial:   serverAttributes[model.ServerSerialAttributeKey],
		Model:    serverAttributes[model.ServerModelAttributeKey],
		Vendor:   serverAttributes[model.ServerVendorAttributeKey],
		Metadata: serverMetadataAttributes,
		Facility: server.FacilityCode,
	}

	if credential != nil {
		asset.BMCUsername = credential.Username
		asset.BMCPassword = credential.Password
		asset.BMCAddress = net.ParseIP(serverAttributes[bmcIPAddressAttributeKey])
	}

	return asset, nil
}

// serverMetadataAttributes parses the server service server metdata attribute data
// and returns a map containing the server metadata
func serverMetadataAttributes(attributes []serverserviceapi.Attributes) (map[string]string, error) {
	metadata := map[string]string{}

	for _, attribute := range attributes {
		// bmc address attribute
		if attribute.Namespace == model.ServerMetadataAttributeNS {
			if err := json.Unmarshal(attribute.Data, &metadata); err != nil {
				return nil, errors.Wrap(ErrServerServiceObject, "server metadata attribute: "+err.Error())
			}
		}
	}

	return metadata, nil
}

// serverAttributes parses the server service attribute data
// and returns a map containing the bmc address, server serial, vendor, model attributes
// and optionally the BMC address and attributes.
func serverAttributes(attributes []serverserviceapi.Attributes, wantBmcCredentials bool) (map[string]string, error) {
	// returned server attributes map
	sAttributes := map[string]string{}

	// bmc IP Address attribute data is unpacked into this map
	bmcData := map[string]string{}

	// server vendor, model attribute data is unpacked into this map
	serverVendorData := map[string]string{}

	for _, attribute := range attributes {
		// bmc address attribute
		if wantBmcCredentials && (attribute.Namespace == bmcAttributeNamespace) {
			if err := json.Unmarshal(attribute.Data, &bmcData); err != nil {
				return nil, errors.Wrap(ErrServerServiceObject, "bmc address attribute: "+err.Error())
			}
		}

		// server vendor, model attributes
		if attribute.Namespace == model.ServerVendorAttributeNS {
			if err := json.Unmarshal(attribute.Data, &serverVendorData); err != nil {
				return nil, errors.Wrap(ErrServerServiceObject, "server vendor attribute: "+err.Error())
			}
		}
	}

	if wantBmcCredentials {
		if len(bmcData) == 0 {
			return nil, errors.New("expected server attributes with BMC address, got none")
		}

		// set bmc address attribute
		sAttributes[bmcIPAddressAttributeKey] = bmcData[bmcIPAddressAttributeKey]
		if sAttributes[bmcIPAddressAttributeKey] == "" {
			return nil, errors.New("expected BMC address attribute empty")
		}
	}

	// set server vendor, model attributes in the returned map
	serverAttributes := []string{
		model.ServerSerialAttributeKey,
		model.ServerModelAttributeKey,
		model.ServerVendorAttributeKey,
	}

	for _, key := range serverAttributes {
		sAttributes[key] = serverVendorData[key]
		if sAttributes[key] == "" {
			sAttributes[key] = "unknown"
		}
	}

	return sAttributes, nil
}

func validateRequiredAttributes(server *serverserviceapi.Server, credential *serverserviceapi.ServerCredential, expectCredentials bool) error {
	if server == nil {
		return errors.New("server object nil")
	}

	if expectCredentials && credential == nil {
		return errors.New("server credential object nil")
	}

	if len(server.Attributes) == 0 {
		return errors.New("server attributes slice empty")
	}

	if expectCredentials && credential.Username == "" {
		return errors.New("BMC username field empty")
	}

	if expectCredentials && credential.Password == "" {
		return errors.New("BMC password field empty")
	}

	return nil
}

// serverPtrSlice returns a slice of pointers to serverserviceapi.Server
//
// The server service server list methods return a slice of server objects,
// this helper method is to reduce the amount of copying of component objects (~176 bytes each) when passed around between methods and range loops,
// while it seems like a minor optimization, it also keeps the linter happy.
func serverPtrSlice(servers []serverserviceapi.Server) []*serverserviceapi.Server {
	returned := make([]*serverserviceapi.Server, 0, len(servers))

	// nolint:gocritic // the copying has to be done somewhere
	for _, s := range servers {
		s := s
		returned = append(returned, &s)
	}

	return returned
}
