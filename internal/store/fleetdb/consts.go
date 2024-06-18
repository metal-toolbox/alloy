package fleetdb

import (
	"fmt"
	"os"

	"github.com/metal-toolbox/alloy/internal/model"
)

// TODO: move these consts into the hollow-toolbox to share between controllers.

const (
	// fleetdb attribute to look up the BMC IP Address in
	bmcAttributeNamespace = "sh.hollow.bmc_info"

	// fleetdb service BMC address attribute key found under the bmcAttributeNamespace
	bmcIPAddressAttributeKey = "address"

	// fleetdb namespace prefix the data is stored in.
	fleetDBNSPrefix = "sh.hollow.alloy"

	// server vendor, model attributes are stored in this namespace.
	serverVendorAttributeNS = fleetDBNSPrefix + ".server_vendor_attributes"

	// additional server metadata are stored in this namespace.
	serverMetadataAttributeNS = fleetDBNSPrefix + ".server_metadata_attributes"

	// errors that occurred when connecting/collecting inventory from the bmc are stored here.
	serverBMCErrorsAttributeNS = fleetDBNSPrefix + ".server_bmc_errors"

	// ƒleetdb server serial attribute key
	serverSerialAttributeKey = "serial"

	// fleetdb server model attribute key
	serverModelAttributeKey = "model"

	// fleetdb server vendor attribute key
	serverVendorAttributeKey = "vendor"
)

// serverBIOSConfigNS returns the namespace server bios configuration are stored in.
func serverBIOSConfigNS(appKind model.AppKind) string {
	if biosConfigNS := os.Getenv("ALLOY_FLEETDB_BIOS_CONFIG_NS"); biosConfigNS != "" {
		return biosConfigNS
	}

	return fmt.Sprintf("%s.%s.bios_configuration", fleetDBNSPrefix, appKind)
}

// serverServiceAttributeNS returns the namespace server component attributes are stored in.
func serverComponentAttributeNS(appKind model.AppKind) string {
	return fmt.Sprintf("%s.%s.metadata", fleetDBNSPrefix, appKind)
}

// serverComponentFirmwareNS returns the namespace server component firmware attributes are stored in.
func serverComponentFirmwareNS(appKind model.AppKind) string {
	return fmt.Sprintf("%s.%s.firmware", fleetDBNSPrefix, appKind)
}

// serverComponentStatusNS returns the namespace server component statuses are stored in.
func serverComponentStatusNS(appKind model.AppKind) string {
	return fmt.Sprintf("%s.%s.status", fleetDBNSPrefix, appKind)
}
