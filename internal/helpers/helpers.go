package helpers

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2/clientcredentials"

	// nolint:gosec // pprof path is only exposed over localhost
	_ "net/http/pprof"

	serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

// EnablePProfile enables the profiling endpoint
func EnablePProfile() {
	go func() {
		server := &http.Server{
			Addr:              model.ProfilingEndpoint,
			ReadHeaderTimeout: 2 * time.Second, // nolint:gomnd // time duration value is clear as is.
		}

		if err := server.ListenAndServe(); err != nil {
			log.Println(err)
		}
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

	// clientID defaults to 'alloy'
	clientID := "alloy"

	if cfg.ServerService.ClientID != "" {
		clientID = cfg.ServerService.ClientID
	}

	// setup oauth configuration
	oauthConfig := clientcredentials.Config{
		ClientID:       clientID,
		ClientSecret:   cfg.ServerService.ClientSecret,
		TokenURL:       provider.Endpoint().TokenURL,
		Scopes:         cfg.ServerService.ClientScopes,
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

func MapsAreEqual(currentMap, newMap map[string]string) bool {
	if len(currentMap) != len(newMap) {
		return false
	}

	for k, currVal := range currentMap {
		newVal, keyExists := newMap[k]
		if !keyExists {
			return false
		}

		if newVal != currVal {
			return false
		}
	}

	return true
}

func WriteDebugFile(name, dump string) {
	// nolint:gomnd // file permission is clear as is
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}

	defer f.Close()

	_, _ = f.WriteString(dump)
}
