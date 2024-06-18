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

	// FleetDBAPIOptions defines the fleetdb API client configuration parameters
	//
	// This parameter is required when StoreKind is set to fleetdb.
	FleetDBAPIOptions *FleetDBAPIOptions `mapstructure:"fleetdb"`

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

// FleetDBAPIOptions defines configuration for the fleetdb client.
// https://github.com/metal-toolbox/fleetdb
type FleetDBAPIOptions struct {
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
	a.Config.FleetDBAPIOptions = &FleetDBAPIOptions{}
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

	if storeKind == model.StoreKindFleetDB {
		if err := a.envVarFleetDBAPIOverrides(); err != nil {
			return errors.Wrap(ErrConfig, "fleetdb env overrides error:"+err.Error())
		}
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
func (a *App) envVarFleetDBAPIOverrides() error {
	if a.Config.FleetDBAPIOptions == nil {
		a.Config.FleetDBAPIOptions = &FleetDBAPIOptions{}
	}

	if a.v.GetString("fleetdb.endpoint") != "" {
		a.Config.FleetDBAPIOptions.Endpoint = a.v.GetString("fleetdb.endpoint")
	}

	if a.v.GetString("fleetdb.facility.code") != "" {
		a.Config.FleetDBAPIOptions.FacilityCode = a.v.GetString("fleetdb.facility.code")
	}

	if a.Config.FleetDBAPIOptions.FacilityCode == "" {
		return errors.New("fleetdb facility code not defined")
	}

	endpointURL, err := url.Parse(a.Config.FleetDBAPIOptions.Endpoint)
	if err != nil {
		return errors.New("fleetdb endpoint URL error: " + err.Error())
	}

	a.Config.FleetDBAPIOptions.EndpointURL = endpointURL

	if a.v.GetString("fleetdb.disable.oauth") != "" {
		a.Config.FleetDBAPIOptions.DisableOAuth = a.v.GetBool("fleetdb.disable.oauth")
	}

	if a.Config.FleetDBAPIOptions.DisableOAuth {
		return nil
	}

	if a.v.GetString("fleetdb.oidc.issuer.endpoint") != "" {
		a.Config.FleetDBAPIOptions.OidcIssuerEndpoint = a.v.GetString("fleetdb.oidc.issuer.endpoint")
	}

	if a.Config.FleetDBAPIOptions.OidcIssuerEndpoint == "" {
		return errors.New("fleetdb oidc.issuer.endpoint not defined")
	}

	if a.v.GetString("fleetdb.oidc.audience.endpoint") != "" {
		a.Config.FleetDBAPIOptions.OidcAudienceEndpoint = a.v.GetString("fleetdb.oidc.audience.endpoint")
	}

	if a.Config.FleetDBAPIOptions.OidcAudienceEndpoint == "" {
		return errors.New("fleetdb oidc.audience.endpoint not defined")
	}

	if a.v.GetString("fleetdb.oidc.client.secret") != "" {
		a.Config.FleetDBAPIOptions.OidcClientSecret = a.v.GetString("fleetdb.oidc.client.secret")
	}

	if a.Config.FleetDBAPIOptions.OidcClientSecret == "" {
		return errors.New("fleetdb.oidc.client.secret not defined")
	}

	if a.v.GetString("fleetdb.oidc.client.id") != "" {
		a.Config.FleetDBAPIOptions.OidcClientID = a.v.GetString("fleetdb.oidc.client.id")
	}

	if a.Config.FleetDBAPIOptions.OidcClientID == "" {
		return errors.New("fleetdb.oidc.client.id not defined")
	}

	if a.v.GetString("fleetdb.oidc.client.scopes") != "" {
		a.Config.FleetDBAPIOptions.OidcClientScopes = a.v.GetStringSlice("fleetdb.oidc.client.scopes")
	}

	if len(a.Config.FleetDBAPIOptions.OidcClientScopes) == 0 {
		return errors.New("fleetdb oidc.client.scopes not defined")
	}

	return nil
}
