package lean

import (
	"os"
	"time"

	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/spf13/viper"
)

type Configuration struct {
	// LogLevel is the app verbose logging level.
	// one of - info, debug, trace
	LogLevel string `mapstructure:"log_level"`

	CollectInterval time.Duration `mapstructure:"collect_interval"`

	CollectIntervalSplay time.Duration `mapstructure:"collect_interval_splay"`

	// CSV file path when StoreKind is set to csv.
	CsvFile string `mapstructure:"csv_file"`

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
	v.SetEnvPrefix(model.AppName)

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

	envVarAppOverrides(v, &config)

	return &config, nil
}

func envVarAppOverrides(v *viper.Viper, config *Configuration) {
	if v.GetString("log.level") != "" {
		config.LogLevel = v.GetString("log.level")
	}

	if v.GetDuration("collect.interval") != 0 {
		config.CollectInterval = v.GetDuration("collect.interval")
	}

	if v.GetDuration("collect.interval.splay") != 0 {
		config.CollectIntervalSplay = v.GetDuration("collect.interval.splay")
	}

	if v.GetString("csv.file") != "" {
		config.CsvFile = v.GetString("csv.file")
	}
}
