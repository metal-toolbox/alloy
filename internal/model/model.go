package model

import (
	"net"

	"github.com/bmc-toolbox/common"
)

const (
	LogLevelInfo       = 0
	LogLevelDebug      = 1
	LogLevelTrace      = 2
	ConcurrencyDefault = 5
	ProfilingEndpoint  = "localhost:9091"
)

var (
	// App logging level
	LogLevel = 0
)

// Asset represents attributes of an asset retrieved from the asset store
type Asset struct {
	// ID is the asset ID
	ID string
	// Vendor is the asset vendor
	Vendor string
	// Model is the asset model
	Model string
	// The datacenter facility
	Facility string
	// Username is the BMC login username
	BMCUsername string
	// Password is the BMC login password
	BMCPassword string
	// Address is the BMC IP address
	BMCAddress net.IP
}

// AssetDevice embeds a common device along with its Asset ID
type AssetDevice struct {
	*common.Device
	ID string
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
		Endpoint     string `mapstructure:"endpoint"`
		AuthToken    string `mapstructure:"auth_token"`
		FacilityCode string `mapstructure:"facility_code"`
		Concurrency  int    `mapstructure:"concurrency"`
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
}
