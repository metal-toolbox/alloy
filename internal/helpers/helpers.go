package helpers

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/coreos/go-oidc"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2/clientcredentials"

	// nolint:gosec,G108 // pprof path is only exposed over localhost
	_ "net/http/pprof"

	serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

// EnablePProfile enables the profiling endpoint
func EnablePProfile() {
	go func() {
		log.Println(http.ListenAndServe(model.ProfilingEndpoint, nil))
	}()

	log.Println("profiling enabled: " + model.ProfilingEndpoint + "/debug/pprof")
}

// NewServerServiceClient instantiates and returns a serverService client
func NewServerServiceClient(ctx context.Context, cfg *model.Config, logger *logrus.Entry) (*serverservice.Client, error) {
	if cfg.ServerService.Concurrency == 0 {
		cfg.ServerService.Concurrency = model.ConcurrencyDefault
	}

	// load configuration parameters from env variables
	cfg.LoadServerServiceEnvVars()

	// validate parameters
	endpointURL, err := cfg.ValidateServerServiceParams()
	if err != nil {
		return nil, err
	}

	if os.Getenv(model.EnvVarServerserviceSkipOAuth) == "true" {
		return newServerserviceClientWithOtel(cfg, endpointURL.String(), logger)
	}

	return newServerserviceClientWithOAuthOtel(ctx, cfg, endpointURL.String(), logger)
}

// returns a serverservice retryable client with Otel
func newServerserviceClientWithOtel(cfg *model.Config, endpoint string, logger *logrus.Entry) (*serverservice.Client, error) {
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

	return serverservice.NewClientWithToken(
		"dummy",
		endpoint,
		retryableClient.StandardClient(),
	)
}

// returns a serverservice retryable http client with Otel and Oauth wrapped in
func newServerserviceClientWithOAuthOtel(ctx context.Context, cfg *model.Config, endpoint string, logger *logrus.Entry) (*serverservice.Client, error) {
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
	provider, err := oidc.NewProvider(ctx, cfg.ServerService.OidcProviderEndpoint)
	if err != nil {
		return nil, err
	}

	// OAuth scopes expected
	scopes := []string{
		"read:server",
		"read:server-component-types",
		"read:server:credentials",
		"create:server:component",
		"create:server:attributes",
		"create:server:versioned-attributes",
		"update:server:component",
		"update:server:attributes",
	}

	// setup oauth configuration
	oauthConfig := clientcredentials.Config{
		ClientID:       "alloy",
		ClientSecret:   cfg.ServerService.ClientSecret,
		TokenURL:       provider.Endpoint().TokenURL,
		Scopes:         scopes,
		EndpointParams: url.Values{"audience": []string{cfg.ServerService.AudienceEndpoint}},
	}

	// wrap OAuth transport, cookie jar in the retryable client
	oAuthclient := oauthConfig.Client(ctx)

	retryableClient.HTTPClient.Transport = oAuthclient.Transport
	retryableClient.HTTPClient.Jar = oAuthclient.Jar

	return serverservice.NewClientWithToken(
		cfg.ServerService.ClientSecret,
		endpoint,
		retryableClient.StandardClient(),
	)
}
