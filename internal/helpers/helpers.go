package helpers

import (
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

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
func NewServerServiceClient(cfg *model.Config, logger *logrus.Entry) (*serverservice.Client, error) {
	// env var auth token
	if authToken := os.Getenv("SERVERSERVICE_AUTH_TOKEN"); authToken != "" {
		cfg.ServerService.AuthToken = authToken
	}

	if cfg.ServerService.AuthToken == "" {
		return nil, errors.Wrap(model.ErrConfig, "expected serverService auth token, got empty")
	}

	// env var serverService endpoint
	if endpoint := os.Getenv("SERVERSERVICE_ENDPOINT"); endpoint != "" {
		cfg.ServerService.Endpoint = endpoint
	}

	if cfg.ServerService.Endpoint == "" {
		return nil, errors.Wrap(model.ErrConfig, "expected serverService endpoint, got empty")
	}

	endpoint, err := url.Parse(cfg.ServerService.Endpoint)
	if err != nil {
		return nil, errors.Wrap(model.ErrConfig, "error in serverService endpoint URL: "+err.Error())
	}

	if cfg.ServerService.Concurrency == 0 {
		cfg.ServerService.Concurrency = model.ConcurrencyDefault
	}

	// init retryable http client
	retryableClient := retryablehttp.NewClient()

	// disable default debug logging on the retryable client
	if logger.Level < logrus.DebugLevel {
		retryableClient.Logger = nil
	} else {
		retryableClient.Logger = logger
	}

	return serverservice.NewClientWithToken(
		cfg.ServerService.AuthToken,
		endpoint.String(),
		retryableClient.StandardClient(),
	)
}
