package componentinventory

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/metal-toolbox/alloy/internal/app"
	cisclient "github.com/metal-toolbox/component-inventory/pkg/api/client"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2/clientcredentials"
)

var (
	// timeout for requests made by this client.
	timeout = 30 * time.Second

	ErrConfigurationNil = errors.New("configuration is nil")
)

// NewComponentInventory instantiates and returns a componentinventory client
func NewComponentInventoryClient(ctx context.Context, cfg *app.Configuration) (cisclient.Client, error) {
	if cfg == nil {
		return nil, ErrConfigurationNil
	}
	cisConfig := cfg.ComponentInventoryAPIOptions

	fmt.Printf("cisConfig.DisableOAuth =  %v\n", cisConfig.DisableOAuth)

	if cisConfig.DisableOAuth {
		client, err := cisclient.NewClient(cisConfig.Endpoint)
		if err != nil {
			// TODO: find a way to handle errors gracefully.
			panic(err)
		}
		return client, nil
	}

	return NewComponentInventoryWithOAuth(ctx, cfg)
}

// NewComponentInventoryWithOAuth returns a componentinventory retryable http client with Otel and Oauth wrapped in
func NewComponentInventoryWithOAuth(ctx context.Context, cfg *app.Configuration) (cisclient.Client, error) {
	if cfg == nil {
		return nil, ErrConfigurationNil
	}
	cisConfig := cfg.ComponentInventoryAPIOptions

	// init retryable http client
	retryableClient := retryablehttp.NewClient()

	// set retryable HTTP client to be the otel http client to collect telemetry
	retryableClient.HTTPClient = otelhttp.DefaultClient

	// setup oidc provider
	provider, err := oidc.NewProvider(ctx, cisConfig.OidcIssuerEndpoint)
	if err != nil {
		return nil, err
	}

	// clientID defaults to 'alloy'
	clientID := "alloy"

	if cisConfig.OidcClientID != "" {
		clientID = cisConfig.OidcClientID
	}

	// setup oauth configuration
	oauthConfig := clientcredentials.Config{
		ClientID:       clientID,
		ClientSecret:   cisConfig.OidcClientSecret,
		TokenURL:       provider.Endpoint().TokenURL,
		Scopes:         cisConfig.OidcClientScopes,
		EndpointParams: url.Values{"audience": []string{cisConfig.OidcAudienceEndpoint}},
	}

	// wrap OAuth transport, cookie jar in the retryable client
	oAuthclient := oauthConfig.Client(ctx)

	retryableClient.HTTPClient.Transport = oAuthclient.Transport
	retryableClient.HTTPClient.Jar = oAuthclient.Jar

	// requests taking longer than timeout value should be canceled.
	client := retryableClient.StandardClient()
	client.Timeout = timeout

	return cisclient.NewClient(
		cisConfig.Endpoint,
		cisclient.WithAuthToken(cisConfig.OidcClientSecret),
		cisclient.WithHTTPClient(client),
	)
}
