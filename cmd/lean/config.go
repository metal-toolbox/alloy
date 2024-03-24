package lean

import (
	"os"

	"github.com/spf13/viper"
)

type Configuration struct {
	// ComponentInventory defines the component inventory client
	// configuration parameters.
	ComponentInventory ComponentInventoryConfig `mapstructure:"component_inventory"`
}

// ComponentInventoryConfig defines configuration for the
// ComponentInventory client.
// https://github.com/metal-toolbox/component-inventory
type ComponentInventoryConfig struct {
	Endpoint             string   `mapstructure:"endpoint"`
	OidcIssuerEndpoint   string   `mapstructure:"oidc_issuer_endpoint"`
	OidcAudienceEndpoint string   `mapstructure:"oidc_audience_endpoint"`
	OidcClientSecret     string   `mapstructure:"oidc_client_secret"`
	OidcClientID         string   `mapstructure:"oidc_client_id"`
	OidcClientScopes     []string `mapstructure:"oidc_client_scopes"`
	DisableOAuth         bool     `mapstructure:"disable_oauth"`
}

// LoadConfiguration loads application configuration
//
// Reads in the cfgFile when available.
// May support overriding from environment variables in the future.
func loadConfiguration(cfgFile string) (*Configuration, error) {
	v := viper.New()
	v.SetConfigType("yaml")

	if cfgFile != "" {
		fh, err := os.Open(cfgFile)
		if err != nil {
			return nil, err
		}

		if err := v.ReadConfig(fh); err != nil {
			return nil, err
		}
	}

	v.SetDefault("log.level", "info")

	config := Configuration{}

	if err := v.Unmarshal(&config); err != nil {
		return nil, err
	}

	return &config, nil
}
