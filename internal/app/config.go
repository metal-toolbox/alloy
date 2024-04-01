package app

import (
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jeremywohl/flatten"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.hollow.sh/toolbox/events"
)

var (
	ErrConfig = errors.New("configuration error")
)

const (
	DefaultCollectInterval = 72 * time.Hour
	DefaultCollectSplay    = 4 * time.Hour
)

// Configuration holds application configuration read from a YAML or set by env variables.
//
// nolint:govet // prefer readability over field alignment optimization for this case.
type Configuration struct {
	// LogLevel is the app verbose logging level.
	// one of - info, debug, trace
	LogLevel string `mapstructure:"log_level"`

	// AppKind is either inband or outofband
	AppKind model.AppKind `mapstructure:"app_kind"`

	// StoreKind declares the type of storage repository that holds asset inventory data.
	StoreKind model.StoreKind `mapstructure:"store_kind"`

	// CSV file path when StoreKind is set to csv.
	CsvFile string `mapstructure:"csv_file"`

	// FacilityCode limits this alloy to events in a facility.
	FacilityCode string `mapstructure:"facility_code"`

	// ServerserviceOptions defines the serverservice client configuration parameters
	//
	// This parameter is required when StoreKind is set to serverservice.
	ServerserviceOptions *ServerserviceOptions `mapstructure:"serverservice"`

	// ComponentInventoryAPIOptions defines the component inventory client
	// configuration parameters.
	ComponentInventoryAPIOptions *ComponentInventoryAPIOptions `mapstructure:"component_inventory"`

	// Controller Out of band collector concurrency
	Concurrency int `mapstructure:"concurrency"`

	CollectInterval time.Duration `mapstructure:"collect_interval"`

	CollectIntervalSplay time.Duration `mapstructure:"collect_interval_splay"`

	// EventsBrokerKind indicates the kind of event broker configuration to enable,
	//
	// Supported parameter value - nats
	EventsBorkerKind string `mapstructure:"events_broker_kind"`

	// NatsOptions defines the NATs events broker configuration parameters.
	//
	// This parameter is required when EventsBrokerKind is set to nats.
	NatsOptions *events.NatsOptions `mapstructure:"nats"`
}

// ServerserviceOptions defines configuration for the Serverservice client.
// https://github.com/metal-toolbox/hollow-serverservice
type ServerserviceOptions struct {
	EndpointURL          *url.URL
	FacilityCode         string   `mapstructure:"facility_code"`
	Endpoint             string   `mapstructure:"endpoint"`
	OidcIssuerEndpoint   string   `mapstructure:"oidc_issuer_endpoint"`
	OidcAudienceEndpoint string   `mapstructure:"oidc_audience_endpoint"`
	OidcClientSecret     string   `mapstructure:"oidc_client_secret"`
	OidcClientID         string   `mapstructure:"oidc_client_id"`
	OidcClientScopes     []string `mapstructure:"oidc_client_scopes"`
	DisableOAuth         bool     `mapstructure:"disable_oauth"`
}

// ComponentInventoryAPIOptions defines configuration for the
// ComponentInventory client.
// https://github.com/metal-toolbox/component-inventory
type ComponentInventoryAPIOptions struct {
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
// Reads in the cfgFile when available and overrides from environment variables.
func (a *App) LoadConfiguration(cfgFile string, storeKind model.StoreKind) error {
	a.v.SetConfigType("yaml")
	a.v.SetEnvPrefix(model.AppName)
	a.v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	a.v.AutomaticEnv()

	// these are initialized here so viper can read in configuration from env vars
	// once https://github.com/spf13/viper/pull/1429 is merged, this can go.
	a.Config.ServerserviceOptions = &ServerserviceOptions{}
	a.Config.NatsOptions = &events.NatsOptions{
		Stream:   &events.NatsStreamOptions{},
		Consumer: &events.NatsConsumerOptions{},
	}

	if cfgFile != "" {
		fh, err := os.Open(cfgFile)
		if err != nil {
			return errors.Wrap(ErrConfig, err.Error())
		}

		if err = a.v.ReadConfig(fh); err != nil {
			return errors.Wrap(ErrConfig, "ReadConfig error:"+err.Error())
		}
	}

	a.v.SetDefault("log.level", "info")
	a.v.SetDefault("collect.interval", DefaultCollectInterval)
	a.v.SetDefault("collect.interval.splay", DefaultCollectSplay)

	if err := a.envBindVars(a.Config); err != nil {
		return errors.Wrap(ErrConfig, "env var bind error:"+err.Error())
	}

	if err := a.v.Unmarshal(a.Config); err != nil {
		return errors.Wrap(ErrConfig, "Unmarshal error: "+err.Error())
	}

	a.envVarAppOverrides()

	if a.Config.EventsBorkerKind == "nats" {
		if err := a.envVarNatsOverrides(); err != nil {
			return errors.Wrap(ErrConfig, "nats env overrides error:"+err.Error())
		}
	}

	if storeKind == model.StoreKindServerservice {
		if err := a.envVarServerserviceOverrides(); err != nil {
			return errors.Wrap(ErrConfig, "serverservice env overrides error:"+err.Error())
		}
	}

	if err := a.envVarComponentInventoryOverrides(); err != nil {
		return errors.Wrap(ErrConfig, "component-inventory env overrides error:"+err.Error())
	}

	return nil
}

func (a *App) envVarAppOverrides() {
	if a.v.GetString("log.level") != "" {
		a.Config.LogLevel = a.v.GetString("log.level")
	}

	if a.v.GetDuration("collect.interval") != 0 {
		a.Config.CollectInterval = a.v.GetDuration("collect.interval")
	}

	if a.v.GetDuration("collect.interval.splay") != 0 {
		a.Config.CollectIntervalSplay = a.v.GetDuration("collect.interval.splay")
	}

	if a.v.GetString("csv.file") != "" {
		a.Config.CsvFile = a.v.GetString("csv.file")
	}
}

// envBindVars binds environment variables to the struct
// without a configuration file being unmarshalled,
// this is a workaround for a viper bug,
//
// This can be replaced by the solution in https://github.com/spf13/viper/pull/1429
// once that PR is merged.
func (a *App) envBindVars(_ *Configuration) error {
	envKeysMap := map[string]interface{}{}
	if err := mapstructure.Decode(a.Config, &envKeysMap); err != nil {
		return err
	}

	// Flatten nested conf map
	flat, err := flatten.Flatten(envKeysMap, "", flatten.DotStyle)
	if err != nil {
		return errors.Wrap(err, "Unable to flatten config")
	}

	for k := range flat {
		if err := a.v.BindEnv(k); err != nil {
			return errors.Wrap(ErrConfig, "env var bind error: "+err.Error())
		}
	}

	return nil
}

// NATs streaming configuration
var (
	defaultNatsConnectTimeout = 100 * time.Millisecond
)

// nolint:gocyclo // nats env config load is cyclomatic
func (a *App) envVarNatsOverrides() error {
	if a.Config.NatsOptions == nil {
		a.Config.NatsOptions = &events.NatsOptions{}
	}

	if a.v.GetString("nats.url") != "" {
		a.Config.NatsOptions.URL = a.v.GetString("nats.url")
	}

	if a.Config.NatsOptions.URL == "" {
		return errors.New("missing parameter: nats.url")
	}

	if a.v.GetString("nats.publisherSubjectPrefix") != "" {
		a.Config.NatsOptions.PublisherSubjectPrefix = a.v.GetString("nats.publisherSubjectPrefix")
	}

	if a.Config.NatsOptions.PublisherSubjectPrefix == "" {
		return errors.New("missing parameter: nats.publisherSubjectPrefix")
	}

	if a.v.GetString("nats.stream.user") != "" {
		a.Config.NatsOptions.StreamUser = a.v.GetString("nats.stream.user")
	}

	if a.v.GetString("nats.stream.pass") != "" {
		a.Config.NatsOptions.StreamPass = a.v.GetString("nats.stream.pass")
	}

	if a.v.GetString("nats.creds.file") != "" {
		a.Config.NatsOptions.CredsFile = a.v.GetString("nats.creds.file")
	}

	if a.v.GetString("nats.stream.name") != "" {
		if a.Config.NatsOptions.Stream == nil {
			a.Config.NatsOptions.Stream = &events.NatsStreamOptions{}
		}

		a.Config.NatsOptions.Stream.Name = a.v.GetString("nats.stream.name")
	}

	if a.Config.NatsOptions.Stream.Name == "" {
		return errors.New("A stream name is required")
	}

	if a.v.GetString("nats.consumer.name") != "" {
		if a.Config.NatsOptions.Consumer == nil {
			a.Config.NatsOptions.Consumer = &events.NatsConsumerOptions{}
		}

		a.Config.NatsOptions.Consumer.Name = a.v.GetString("nats.consumer.name")
	}

	if len(a.v.GetStringSlice("nats.consumer.subscribeSubjects")) != 0 {
		a.Config.NatsOptions.Consumer.SubscribeSubjects = a.v.GetStringSlice("nats.consumer.subscribeSubjects")
	}

	if len(a.Config.NatsOptions.Consumer.SubscribeSubjects) == 0 {
		return errors.New("missing parameter: nats.consumer.subscribeSubjects")
	}

	if a.v.GetString("nats.consumer.filterSubject") != "" {
		a.Config.NatsOptions.Consumer.FilterSubject = a.v.GetString("nats.consumer.filterSubject")
	}

	if a.Config.NatsOptions.Consumer.FilterSubject == "" {
		return errors.New("missing parameter: nats.consumer.filterSubject")
	}

	if a.v.GetDuration("nats.connect.timeout") != 0 {
		a.Config.NatsOptions.ConnectTimeout = a.v.GetDuration("nats.connect.timeout")
	}

	if a.Config.NatsOptions.ConnectTimeout == 0 {
		a.Config.NatsOptions.ConnectTimeout = defaultNatsConnectTimeout
	}

	return nil
}

// Server service configuration options

// nolint:gocyclo // parameter validation is cyclomatic
func (a *App) envVarServerserviceOverrides() error {
	if a.Config.ServerserviceOptions == nil {
		a.Config.ServerserviceOptions = &ServerserviceOptions{}
	}

	if a.v.GetString("serverservice.endpoint") != "" {
		a.Config.ServerserviceOptions.Endpoint = a.v.GetString("serverservice.endpoint")
	}

	if a.v.GetString("serverservice.facility.code") != "" {
		a.Config.ServerserviceOptions.FacilityCode = a.v.GetString("serverservice.facility.code")
	}

	if a.Config.ServerserviceOptions.FacilityCode == "" {
		return errors.New("serverservice facility code not defined")
	}

	endpointURL, err := url.Parse(a.Config.ServerserviceOptions.Endpoint)
	if err != nil {
		return errors.New("serverservice endpoint URL error: " + err.Error())
	}

	a.Config.ServerserviceOptions.EndpointURL = endpointURL

	if a.v.GetString("serverservice.disable.oauth") != "" {
		a.Config.ServerserviceOptions.DisableOAuth = a.v.GetBool("serverservice.disable.oauth")
	}

	if a.Config.ServerserviceOptions.DisableOAuth {
		return nil
	}

	if a.v.GetString("serverservice.oidc.issuer.endpoint") != "" {
		a.Config.ServerserviceOptions.OidcIssuerEndpoint = a.v.GetString("serverservice.oidc.issuer.endpoint")
	}

	if a.Config.ServerserviceOptions.OidcIssuerEndpoint == "" {
		return errors.New("serverservice oidc.issuer.endpoint not defined")
	}

	if a.v.GetString("serverservice.oidc.audience.endpoint") != "" {
		a.Config.ServerserviceOptions.OidcAudienceEndpoint = a.v.GetString("serverservice.oidc.audience.endpoint")
	}

	if a.Config.ServerserviceOptions.OidcAudienceEndpoint == "" {
		return errors.New("serverservice oidc.audience.endpoint not defined")
	}

	if a.v.GetString("serverservice.oidc.client.secret") != "" {
		a.Config.ServerserviceOptions.OidcClientSecret = a.v.GetString("serverservice.oidc.client.secret")
	}

	if a.Config.ServerserviceOptions.OidcClientSecret == "" {
		return errors.New("serverservice.oidc.client.secret not defined")
	}

	if a.v.GetString("serverservice.oidc.client.id") != "" {
		a.Config.ServerserviceOptions.OidcClientID = a.v.GetString("serverservice.oidc.client.id")
	}

	if a.Config.ServerserviceOptions.OidcClientID == "" {
		return errors.New("serverservice.oidc.client.id not defined")
	}

	if a.v.GetString("serverservice.oidc.client.scopes") != "" {
		a.Config.ServerserviceOptions.OidcClientScopes = a.v.GetStringSlice("serverservice.oidc.client.scopes")
	}

	if len(a.Config.ServerserviceOptions.OidcClientScopes) == 0 {
		return errors.New("serverservice oidc.client.scopes not defined")
	}

	return nil
}

// Component inventory configuration options

// nolint:gocyclo // parameter validation is cyclomatic
func (a *App) envVarComponentInventoryOverrides() error {
	if a.Config.ComponentInventoryAPIOptions == nil {
		a.Config.ComponentInventoryAPIOptions = &ComponentInventoryAPIOptions{}
	}

	if a.v.GetString("component_inventory.endpoint") != "" {
		a.Config.ComponentInventoryAPIOptions.Endpoint = a.v.GetString("component_inventory.endpoint")
	}

	if a.v.GetString("component_inventory.disable.oauth") != "" {
		a.Config.ComponentInventoryAPIOptions.DisableOAuth = a.v.GetBool("component_inventory.disable.oauth")
	}

	if a.Config.ComponentInventoryAPIOptions.DisableOAuth {
		return nil
	}

	if a.v.GetString("component_inventory.oidc.issuer.endpoint") != "" {
		a.Config.ComponentInventoryAPIOptions.OidcIssuerEndpoint = a.v.GetString("component_inventory.oidc.issuer.endpoint")
	}

	if a.Config.ComponentInventoryAPIOptions.OidcIssuerEndpoint == "" {
		return errors.New("component_inventory oidc.issuer.endpoint not defined")
	}

	if a.v.GetString("component_inventory.oidc.audience.endpoint") != "" {
		a.Config.ComponentInventoryAPIOptions.OidcAudienceEndpoint = a.v.GetString("component_inventory.oidc.audience.endpoint")
	}

	if a.Config.ComponentInventoryAPIOptions.OidcAudienceEndpoint == "" {
		return errors.New("component_inventory oidc.audience.endpoint not defined")
	}

	if a.v.GetString("component_inventory.oidc.client.secret") != "" {
		a.Config.ComponentInventoryAPIOptions.OidcClientSecret = a.v.GetString("component_inventory.oidc.client.secret")
	}

	if a.Config.ComponentInventoryAPIOptions.OidcClientSecret == "" {
		return errors.New("component_inventory.oidc.client.secret not defined")
	}

	if a.v.GetString("component_inventory.oidc.client.id") != "" {
		a.Config.ComponentInventoryAPIOptions.OidcClientID = a.v.GetString("component_inventory.oidc.client.id")
	}

	if a.Config.ComponentInventoryAPIOptions.OidcClientID == "" {
		return errors.New("component_inventory.oidc.client.id not defined")
	}

	if a.v.GetString("component_inventory.oidc.client.scopes") != "" {
		a.Config.ComponentInventoryAPIOptions.OidcClientScopes = a.v.GetStringSlice("component_inventory.oidc.client.scopes")
	}

	if len(a.Config.ComponentInventoryAPIOptions.OidcClientScopes) == 0 {
		return errors.New("component_inventory oidc.client.scopes not defined")
	}

	return nil
}
