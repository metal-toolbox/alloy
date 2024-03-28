package inband

import (
	"context"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/ironlib"
	"github.com/metal-toolbox/ironlib/actions"
	"github.com/sirupsen/logrus"
)

// Inband collector collects hardware, firmware inventory inband
type Queryor struct {
	deviceManager actions.DeviceManager
	logger        *logrus.Entry
	mock          bool
}

// New returns an inband inventory collector
func NewQueryor(logger *logrus.Logger) *Queryor {
	loggerEntry := app.NewLogrusEntryFromLogger(
		logrus.Fields{"component": "queryor.inband"},
		logger,
	)

	return &Queryor{
		logger: loggerEntry,
	}
}

// Inventory implements the Queryor interface to collect inventory inband.
func (i *Queryor) Inventory(ctx context.Context, _ *model.LoginInfo) (*common.Device, error) {
	if !i.mock {
		var err error

		i.deviceManager, err = ironlib.New(i.logger.Logger)
		if err != nil {
			return nil, err
		}
	}

	device, err := i.deviceManager.GetInventory(ctx)
	if err != nil {
		return nil, err
	}

	device.Vendor = common.FormatVendorName(device.Vendor)

	return device, nil
}

// BiosConfiguration implements the Queryor interface to collect BIOS configuration inband.
//
// The given asset object is updated with the collected information.
func (i *Queryor) BiosConfiguration(ctx context.Context, _ *model.LoginInfo) (map[string]string, error) {
	if !i.mock {
		var err error

		i.deviceManager, err = ironlib.New(i.logger.Logger)
		if err != nil {
			return nil, err
		}
	}

	return i.deviceManager.GetBIOSConfiguration(ctx)
}
