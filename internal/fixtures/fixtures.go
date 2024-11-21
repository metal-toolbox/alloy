package fixtures

import (
	"github.com/jinzhu/copier"
	common "github.com/metal-toolbox/bmc-common"
	fleetdbapi "github.com/metal-toolbox/fleetdb/pkg/api/v1"
)

// CopyDevice returns a pointer to a copy of the given ironlib device object
func CopyDevice(src *common.Device) *common.Device {
	dst := &common.Device{}

	copyOptions := copier.Option{IgnoreEmpty: true, DeepCopy: true}

	err := copier.CopyWithOption(&dst, &src, copyOptions)
	if err != nil {
		panic(err)
	}

	return dst
}

// CopyFleetDBComponentSlice returns a pointer to a copy of the server service components slice
func CopyFleetDBComponentSlice(src fleetdbapi.ServerComponentSlice) fleetdbapi.ServerComponentSlice {
	dst := fleetdbapi.ServerComponentSlice{}

	copyOptions := copier.Option{IgnoreEmpty: true, DeepCopy: true}

	err := copier.CopyWithOption(&dst, src, copyOptions)
	if err != nil {
		panic(err)
	}

	return dst
}
