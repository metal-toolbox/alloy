package model

import (
	"net"
	"time"

	"github.com/bmc-toolbox/common"
)

type (
	AppKind   string
	StoreKind string

	// LogLevel is the logging level string.
	LogLevel string
)

const (
	AppName                  = "alloy"
	AppKindInband    AppKind = "inband"
	AppKindOutOfBand AppKind = "outofband"

	StoreKindCsv           StoreKind = "csv"
	StoreKindServerservice StoreKind = "serverservice"
	StoreKindMock          StoreKind = "mock"

	LogLevelInfo  LogLevel = "info"
	LogLevelDebug LogLevel = "debug"
	LogLevelTrace LogLevel = "trace"

	ConcurrencyDefault = 5
	ProfilingEndpoint  = "localhost:9091"
	MetricsEndpoint    = "0.0.0.0:9090"
	// EnvVarDumpFixtures when enabled, will dump data for assets, to be used as fixture data.
	EnvVarDumpFixtures = "DEBUG_DUMP_FIXTURES"
	// EnvVarDumpDiffers when enabled, will dump component differ data for debugging
	// differences identified in component objects in the publish package.
	EnvVarDumpDiffers = "DEBUG_DUMP_DIFFERS"
)

// Asset represents attributes of an asset retrieved from the asset store
type Asset struct {
	// Inventory collected from the device
	Inventory *common.Device
	// The device metadata attribute
	Metadata map[string]string
	// BIOS configuration
	BiosConfig map[string]string
	// The device ID from the inventory store
	ID string
	// The device vendor attribute
	Vendor string
	// The device model attribute
	Model string
	// The device serial attribute
	Serial string
	// The datacenter facility attribute from the configuration
	Facility string
	// Username is the BMC login username from the inventory store
	BMCUsername string
	// Password is the BMC login password from the inventory store
	BMCPassword string
	// Errors is a map of errors,
	// where the key is the stage at which the error occurred,
	// and the value is the error.
	Errors map[string]string
	// Address is the BMC IP address from the inventory store
	BMCAddress net.IP
}

// IncludeError includes the given error key and value in the asset
// which is then available to the publisher for reporting.
func (a *Asset) IncludeError(key, value string) {
	if a.Errors == nil {
		a.Errors = map[string]string{}
	}

	a.Errors[key] = value
}

// Config holds application configuration read from a YAML or set by env variables.
//
// nolint:govet // prefer readability over field alignment optimization for this case.
type Config struct {
	// File is the configuration file path
	File string
	// LogLevel is the app verbose logging level.
	LogLevel int
	// AppKind is one of inband, outofband
	AppKind string `mapstructure:"app_kind"`

	// Out of band collector configuration
	CollectorOutofband struct {
		Concurrency int `mapstructure:"concurrency"`
	} `mapstructure:"collector_outofband"`

	// ServerService is the Hollow server inventory store
	// https://github.com/metal-toolbox/hollow-serverservice
	ServerService struct {
		Endpoint             string   `mapstructure:"endpoint"`
		OidcProviderEndpoint string   `mapstructure:"oidc_provider_endpoint"`
		AudienceEndpoint     string   `mapstructure:"audience_endpoint"`
		ClientSecret         string   `mapstructure:"client_secret"`
		ClientID             string   `mapstructure:"client_id"`
		ClientScopes         []string `mapstructure:"client_scopes"` // []string{"read:server", ..}
		FacilityCode         string   `mapstructure:"facility_code"`
		Concurrency          int      `mapstructure:"concurrency"`
	} `mapstructure:"serverService"`

	// AssetGetter is where alloy looks up assets information like BMC credentials
	// to collect inventory.
	AssetGetter struct {
		// supported parameters: csv OR serverService
		Kind string `mapstructure:"kind"`

		// Csv is the CSV asset getter type configuration.
		Csv struct {
			File string `mapstructure:"file"`
		} `mapstructure:"csv"`
	} `mapstructure:"asset_getter"`

	// Publisher is the inventory store where alloy writes collected inventory data
	InventoryPublisher struct {
		// supported parameters: stdout, serverService
		Kind string `mapstructure:"kind"`
	} `mapstructure:"inventory_publisher"`

	// EventsBrokerKind indicates the kind of event broker configuration to enable,
	//
	// Supported parameter value - nats
	EventsBorkerKind string `mapstructure:"events_broker_kind"`

	// NatsOptions defines the NATs events broker configuration parameters.
	//
	// This parameter is required when EventsBrokerKind is set to nats.
	NatsOptions NatsOptions `mapstructure:"nats_options"`
}

type NatsOptions struct {
	StreamURL          string `mapstructure:"stream_url"`
	StreamUser         string `mapstructure:"stream_user"`
	StreamPass         string `mapstructure:"stream_pass"`
	CredsFile          string `mapstructure:"creds_file"`
	StreamName         string `mapstructure:"stream_name"`
	StreamPrefix       string `mapstructure:"stream_prefix"`
	StreamURNNamespace string `mapstructure:"stream_urn_ns"`
	// PublisherSubjects sets the subjects that can be published on the added stream.
	PublisherSubjects []string `mapstructure:"publisher_subjects"`
	// SubscriberSubjects when defined will result in the event broker subscribing to given streams.
	SubscriberSubjects []string      `mapstructure:"subscriber_subjects"`
	ConnectTimeout     time.Duration `mapstructure:"connect_timeout"`
}
