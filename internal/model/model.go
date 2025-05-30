package model

import (
	"errors"
	"net"

	common "github.com/metal-toolbox/bmc-common"
	rctypes "github.com/metal-toolbox/rivets/v2/condition"
)

type (
	AppKind   string
	StoreKind string

	InventoryMethod string

	// LogLevel is the logging level string.
	LogLevel string

	CollectorError string
)

var (
	ErrInventoryQuery = errors.New("inventory query returned error")
)

const (
	AppName                  = "alloy"
	AppKindInband    AppKind = "inband"
	AppKindOutOfBand AppKind = "outofband"

	// conditions fulfilled by this worker
	Inventory rctypes.Kind = "inventory"

	StoreKindCsv     StoreKind = "csv"
	StoreKindFleetDB StoreKind = "fleetdb"
	StoreKindMock    StoreKind = "mock"

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

// AppendError includes the given error key and value in the asset
// which is then available to the publisher for reporting.
func (a *Asset) AppendError(key CollectorError, value string) {
	if a.Errors == nil {
		a.Errors = map[string]string{}
	}

	a.Errors[string(key)] = value
}

func (a *Asset) HasError(cErr CollectorError) bool {
	_, exists := a.Errors[string(cErr)]
	return exists
}
