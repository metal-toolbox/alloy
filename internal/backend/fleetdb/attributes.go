package fleetdb

import (
	"encoding/json"

	fleetdbapi "github.com/metal-toolbox/fleetdb/pkg/api/v1"
	consts "github.com/metal-toolbox/rivets/fleetdb"
	"github.com/pkg/errors"
)

// serverAttributes parses the fleetdb attribute data
// and returns a map containing the bmc address, server serial, vendor, model attributes
// and optionally the BMC address and attributes.
func serverAttributes(attributes []fleetdbapi.Attributes, wantBmcCredentials bool) (map[string]string, error) {
	// returned server attributes map
	sAttributes := map[string]string{}

	// bmc IP Address attribute data is unpacked into this map
	bmcData := map[string]string{}

	// server vendor, model attribute data is unpacked into this map
	serverVendorData := map[string]string{}

	for _, attribute := range attributes {
		// bmc address attribute
		if wantBmcCredentials && (attribute.Namespace == consts.ServerAttributeNSBmcAddress) {
			if err := json.Unmarshal(attribute.Data, &bmcData); err != nil {
				return nil, errors.Wrap(ErrFleetDBObject, "bmc address attribute: "+err.Error())
			}
		}

		// server vendor, model attributes
		if attribute.Namespace == consts.ServerVendorAttributeNS {
			if err := json.Unmarshal(attribute.Data, &serverVendorData); err != nil {
				return nil, errors.Wrap(ErrFleetDBObject, "server vendor attribute: "+err.Error())
			}
		}
	}

	if wantBmcCredentials {
		if len(bmcData) == 0 {
			return nil, errors.New("expected server attributes with BMC address, got none")
		}

		// set bmc address attribute
		sAttributes[bmcIPAddressAttributeKey] = bmcData[bmcIPAddressAttributeKey]
		if sAttributes[bmcIPAddressAttributeKey] == "" {
			return nil, errors.New("expected BMC address attribute empty")
		}
	}

	// set server vendor, model attributes in the returned map
	serverAttributes := []string{
		serverSerialAttributeKey,
		serverModelAttributeKey,
		serverVendorAttributeKey,
	}

	for _, key := range serverAttributes {
		sAttributes[key] = serverVendorData[key]
		if sAttributes[key] == "" {
			sAttributes[key] = "unknown"
		}
	}

	return sAttributes, nil
}
