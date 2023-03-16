package serverservice

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/coreos/go-oidc"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2/clientcredentials"
)

// TODO move this under an interface

// NewServerServiceClient instantiates and returns a serverService client
func NewServerServiceClient(ctx context.Context, cfg *app.ServerserviceOptions, logger *logrus.Entry) (*serverserviceapi.Client, error) {
	if cfg.DisableOAuth {
		return newServerserviceClientWithOtel(cfg, cfg.Endpoint, logger)
	}

	return newServerserviceClientWithOAuthOtel(ctx, cfg, cfg.Endpoint, logger)
}

// returns a serverservice retryable client with Otel
func newServerserviceClientWithOtel(cfg *app.ServerserviceOptions, endpoint string, logger *logrus.Entry) (*serverserviceapi.Client, error) {
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

	return serverserviceapi.NewClientWithToken(
		"dummy",
		endpoint,
		retryableClient.StandardClient(),
	)
}

// returns a serverservice retryable http client with Otel and Oauth wrapped in
func newServerserviceClientWithOAuthOtel(ctx context.Context, cfg *app.ServerserviceOptions, endpoint string, logger *logrus.Entry) (*serverserviceapi.Client, error) {
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

	return serverserviceapi.NewClientWithToken(
		cfg.OidcClientSecret,
		endpoint,
		retryableClient.StandardClient(),
	)
}
