package serverservice

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2/clientcredentials"

	serverserviceapi "go.hollow.sh/serverservice/pkg/api/v1"
)

var (
	// timeout for requests made by this client.
	timeout   = 30 * time.Second
	ErrConfig = errors.New("error in serverservice client configuration")
)

// TODO move this under an interface

// NewServerServiceClient instantiates and returns a serverService client
func NewServerServiceClient(ctx context.Context, cfg *app.ServerserviceOptions, logger *logrus.Entry) (*serverserviceapi.Client, error) {
	if cfg == nil {
		return nil, errors.Wrap(ErrConfig, "configuration is nil")
	}

	if cfg.DisableOAuth {
		return newServerserviceClientWithOtel(cfg, cfg.Endpoint, logger)
	}

	return newServerserviceClientWithOAuthOtel(ctx, cfg, cfg.Endpoint, logger)
}

// returns a serverservice retryable client with Otel
func newServerserviceClientWithOtel(cfg *app.ServerserviceOptions, endpoint string, logger *logrus.Entry) (*serverserviceapi.Client, error) {
	if cfg == nil {
		return nil, errors.Wrap(ErrConfig, "configuration is nil")
	}

	// init retryable http client
	retryableClient := retryablehttp.NewClient()

	// log hook fo 500 errors since the the retryablehttp client masks them
	logHookFunc := func(l retryablehttp.Logger, r *http.Response) {
		if r.StatusCode == http.StatusInternalServerError {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				logger.Warn("serverservice query returned 500 error, got error reading body: ", err.Error())
				return
			}

			logger.Warn("serverservice query returned 500 error, body: ", string(b))
		}
	}

	retryableClient.ResponseLogHook = logHookFunc

	// set retryable HTTP client to be the otel http client to collect telemetry
	retryableClient.HTTPClient = otelhttp.DefaultClient

	// disable default debug logging on the retryable client
	if logger.Level < logrus.DebugLevel {
		retryableClient.Logger = nil
	} else {
		retryableClient.Logger = logger
	}

	// requests taking longer than timeout value should be canceled.
	client := retryableClient.StandardClient()
	client.Timeout = timeout

	return serverserviceapi.NewClientWithToken(
		"dummy",
		endpoint,
		client,
	)
}

// returns a serverservice retryable http client with Otel and Oauth wrapped in
func newServerserviceClientWithOAuthOtel(ctx context.Context, cfg *app.ServerserviceOptions, endpoint string, logger *logrus.Entry) (*serverserviceapi.Client, error) {
	if cfg == nil {
		return nil, errors.Wrap(ErrConfig, "configuration is nil")
	}

	// init retryable http client
	retryableClient := retryablehttp.NewClient()

	// set retryable HTTP client to be the otel http client to collect telemetry
	retryableClient.HTTPClient = otelhttp.DefaultClient

	// disable default debug logging on the retryable client
	if logger.Level < logrus.DebugLevel {
		retryableClient.Logger = nil
	} else {
		retryableClient.Logger = logger
	}

	// setup oidc provider
	provider, err := oidc.NewProvider(ctx, cfg.OidcIssuerEndpoint)
	if err != nil {
		return nil, err
	}

	// clientID defaults to 'alloy'
	clientID := "alloy"

	if cfg.OidcClientID != "" {
		clientID = cfg.OidcClientID
	}

	// setup oauth configuration
	oauthConfig := clientcredentials.Config{
		ClientID:       clientID,
		ClientSecret:   cfg.OidcClientSecret,
		TokenURL:       provider.Endpoint().TokenURL,
		Scopes:         cfg.OidcClientScopes,
		EndpointParams: url.Values{"audience": []string{cfg.OidcAudienceEndpoint}},
	}

	// wrap OAuth transport, cookie jar in the retryable client
	oAuthclient := oauthConfig.Client(ctx)

	retryableClient.HTTPClient.Transport = oAuthclient.Transport
	retryableClient.HTTPClient.Jar = oAuthclient.Jar

	// requests taking longer than timeout value should be canceled.
	client := retryableClient.StandardClient()
	client.Timeout = timeout

	return serverserviceapi.NewClientWithToken(
		cfg.OidcClientSecret,
		endpoint,
		client,
	)
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

func toAsset(server *serverserviceapi.Server, credential *serverserviceapi.ServerCredential, expectCredentials bool) (*model.Asset, error) {
	if err := validateRequiredAttributes(server, credential, expectCredentials); err != nil {
		return nil, errors.Wrap(ErrServerServiceObject, err.Error())
	}

	serverAttributes, err := serverAttributes(server.Attributes, expectCredentials)
	if err != nil {
		fmt.Println(err.Error())
		return nil, errors.Wrap(ErrServerServiceObject, err.Error())
	}

	serverMetadataAttributes, err := serverMetadataAttributes(server.Attributes)
	if err != nil {
		return nil, errors.Wrap(ErrServerServiceObject, err.Error())
	}

	asset := &model.Asset{
		ID:       server.UUID.String(),
		Serial:   serverAttributes[serverSerialAttributeKey],
		Model:    serverAttributes[serverModelAttributeKey],
		Vendor:   serverAttributes[serverVendorAttributeKey],
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
