package model

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/pkg/errors"
)

const (
	// EnvServerserviceSkipOAuth when set to true will skip server service OAuth.
	EnvVarServerserviceSkipOAuth = "SERVERSERVICE_SKIP_OAUTH"

	// serverservice namespace prefix the data is stored in.
	ServerServiceNSPrefix = "sh.hollow.alloy"

	// server vendor, model attributes are stored in this namespace.
	ServerVendorAttributeNS = ServerServiceNSPrefix + ".server_vendor_attributes"

	// additional server metadata are stored in this namespace.
	ServerMetadataAttributeNS = ServerServiceNSPrefix + ".server_metadata_attributes"

	// errors that occurred when connecting/collecting inventory from the bmc are stored here.
	ServerBMCErrorsAttributeNS = ServerServiceNSPrefix + ".server_bmc_errors"

	// server service server serial attribute key
	ServerSerialAttributeKey = "serial"

	// server service server model attribute key
	ServerModelAttributeKey = "model"

	// server service server vendor attribute key
	ServerVendorAttributeKey = "vendor"
)

// ServerBIOSConfigNS returns the namespace server bios configuration are stored in.
func ServerBIOSConfigNS(appKind string) string {
	if biosConfigNS := os.Getenv("SERVERSERVICE_BIOS_CONFIG_NS"); biosConfigNS != "" {
		return biosConfigNS
	}

	return fmt.Sprintf("%s.%s.bios_configuration", ServerServiceNSPrefix, appKind)
}

// ServerServiceAttributeNS returns the namespace server component attributes are stored in.
func ServerComponentAttributeNS(appKind string) string {
	return fmt.Sprintf("%s.%s.metadata", ServerServiceNSPrefix, appKind)
}

// ServerComponentFirmwareNS returns the namespace server component firmware attributes are stored in.
func ServerComponentFirmwareNS(appKind string) string {
	return fmt.Sprintf("%s.%s.firmware", ServerServiceNSPrefix, appKind)
}

// ServerComponentStatusNS returns the namespace server component statuses are stored in.
func ServerComponentStatusNS(appKind string) string {
	return fmt.Sprintf("%s.%s.status", ServerServiceNSPrefix, appKind)
}

// LoadServerServiceEnvVars sets any env SERVERSERVICE_* configuration parameters
func (c *Config) LoadServerServiceEnvVars() {
	if facility := os.Getenv("SERVERSERVICE_FACILITY_CODE"); facility != "" {
		c.ServerService.FacilityCode = facility
	}

	// env var serverService endpoint
	if endpoint := os.Getenv("SERVERSERVICE_ENDPOINT"); endpoint != "" {
		c.ServerService.Endpoint = endpoint
	}

	// OIDC provider endpoint
	if oidcProviderEndpoint := os.Getenv("SERVERSERVICE_OIDC_PROVIDER_ENDPOINT"); oidcProviderEndpoint != "" {
		c.ServerService.OidcProviderEndpoint = oidcProviderEndpoint
	}

	// Audience endpoint
	if audienceEndpoint := os.Getenv("SERVERSERVICE_AUDIENCE_ENDPOINT"); audienceEndpoint != "" {
		c.ServerService.AudienceEndpoint = audienceEndpoint
	}

	// env var OAuth client secret
	if clientSecret := os.Getenv("SERVERSERVICE_CLIENT_SECRET"); clientSecret != "" {
		c.ServerService.ClientSecret = clientSecret
	}

	// env var OAuth client ID
	if clientID := os.Getenv("SERVERSERVICE_CLIENT_ID"); clientID != "" {
		c.ServerService.ClientID = clientID
	}

	// env var OAuth client scopes
	if clientScopes := os.Getenv("SERVERSERVICE_CLIENT_SCOPES"); clientScopes != "" {
		c.ServerService.ClientScopes = c.serverserviceScopesFromEnvVar(clientScopes)
	}
}

// parses comma separated scope values and trims any spaces in them.
func (c *Config) serverserviceScopesFromEnvVar(v string) []string {
	scopes := []string{}
	tokens := strings.Split(v, ",")

	for _, t := range tokens {
		scopes = append(scopes, strings.TrimSpace(t))
	}

	return scopes
}

// ValidateServerServiceParams checks required serverservice configuration parameters are present
// and returns the serverservice URL endpoint
func (c *Config) ValidateServerServiceParams() (*url.URL, error) {
	if c.ServerService.FacilityCode == "" {
		return nil, errors.Wrap(ErrConfig, "serverService facility code not defined")
	}

	if c.ServerService.Endpoint == "" {
		return nil, errors.Wrap(ErrConfig, "serverService endpoint not defined")
	}

	endpoint, err := url.Parse(c.ServerService.Endpoint)
	if err != nil {
		return nil, errors.Wrap(ErrConfig, "error in serverService endpoint URL: "+err.Error())
	}

	if os.Getenv(EnvVarServerserviceSkipOAuth) == "true" {
		return endpoint, nil
	}

	if c.ServerService.OidcProviderEndpoint == "" {
		return nil, errors.Wrap(ErrConfig, "serverService OIDC provider endpoint not defined")
	}

	if c.ServerService.AudienceEndpoint == "" {
		return nil, errors.Wrap(ErrConfig, "serverService Audience endpoint not defined")
	}

	if c.ServerService.ClientSecret == "" {
		return nil, errors.Wrap(ErrConfig, "serverService client secret not defined")
	}

	if c.ServerService.ClientID == "" {
		return nil, errors.Wrap(ErrConfig, "serverService client ID not defined")
	}

	if len(c.ServerService.ClientScopes) == 0 {
		return nil, errors.Wrap(ErrConfig, "serverService client scopes not defined")
	}

	return endpoint, nil
}
