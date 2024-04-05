package fleetdb

import (
	"context"
	"net"

	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	fleetdbapi "github.com/metal-toolbox/fleetdb/pkg/api/v1"
)

const (
	pkgName = "internal/store"
)

// Store is an asset inventory store
type Client struct {
	*fleetdbapi.Client
	logger       *logrus.Logger
	facilityCode string
}

// NewStore returns a serverservice store queryor to lookup and publish assets to, from the store.
func New(ctx context.Context, _ model.AppKind, cfg *app.ServerserviceOptions, logger *logrus.Logger) (*Client, error) {
	logger.Info("fleetdb store ctor")

	apiclient, err := NewFleetDBClient(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	s := &Client{
		Client:       apiclient,
		logger:       logger,
		facilityCode: cfg.FacilityCode,
	}

	return s, nil
}

// BMCCredentials fetches BMC credentials for login to the assert.
func (fc *Client) BMCCredentials(ctx context.Context, id string) (*model.LoginInfo, error) {
	ctx, span := otel.Tracer(pkgName).Start(ctx, "Serverservice.BMCCredentials")
	defer span.End()

	sid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}

	// get server
	server, _, err := fc.Client.Get(ctx, sid)
	if err != nil {
		span.SetStatus(codes.Error, "Get() server failed")

		return nil, errors.Wrap(model.ErrInventoryQuery, "error querying server attributes: "+err.Error())
	}

	// get vendor and model
	serverAttributes, err := serverAttributes(server.Attributes, true)
	if err != nil {
		return nil, errors.Wrap(ErrServerServiceObject, err.Error())
	}

	// get bmc credential
	credential, _, err := fc.Client.GetCredential(ctx, sid, fleetdbapi.ServerCredentialTypeBMC)
	if err != nil {
		span.SetStatus(codes.Error, "GetCredential() failed")

		return nil, errors.Wrap(model.ErrInventoryQuery, "error querying BMC credentials: "+err.Error())
	}

	return &model.LoginInfo{
		ID:          id,
		Model:       serverAttributes[serverModelAttributeKey],
		Vendor:      serverAttributes[serverVendorAttributeKey],
		BMCAddress:  net.ParseIP(serverAttributes[bmcIPAddressAttributeKey]),
		BMCUsername: credential.Username,
		BMCPassword: credential.Password,
	}, nil
}
