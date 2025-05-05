package inband

import (
	"context"

	common "github.com/metal-toolbox/bmc-common"
	"github.com/metal-toolbox/ironlib"
	"github.com/metal-toolbox/ironlib/actions"
	"github.com/sirupsen/logrus"

	"github.com/metal-toolbox/alloy/internal/model"
)

// Inband collector collects hardware, firmware inventory inband
type Queryor struct {
	deviceManager actions.DeviceManager
	logger        *logrus.Entry
	mock          bool
}

// New returns an inband inventory collector
func NewQueryor(logger *logrus.Logger) *Queryor {
	return &Queryor{
		logger: logger.WithFields(logrus.Fields{"component": "queryor.inband"}),
	}
}

// Inventory implements the Queryor interface to collect inventory inband.
//
// The given asset object is updated with the collected information.
func (i *Queryor) Inventory(ctx context.Context, asset *model.Asset) error {
	if !i.mock {
		var err error

		i.deviceManager, err = ironlib.New(i.logger.Logger)
		if err != nil {
			return err
		}
	}

	i.logger.Info("inband: GetInventory launching...")
	device, err := i.deviceManager.GetInventory(ctx)
	if err != nil {
		i.logger.WithFields(logrus.Fields{"err": err}).Error("inband: GetInventory error")
		return err
	}

	device.Vendor = common.FormatVendorName(device.Vendor)

	// The "unknown" valued attributes here are to be filled in by the caller,
	// with the data from the inventory source when its available.
	asset.Inventory = device
	asset.Vendor = "unknown"
	asset.Model = "unknown"
	asset.Serial = "unknown"

	i.logger.Info("inband: GetInventory success")
	return nil
}

// BiosConfiguration implements the Queryor interface to collect BIOS configuration inband.
//
// The given asset object is updated with the collected information.
func (i *Queryor) BiosConfiguration(ctx context.Context, asset *model.Asset) error {
	if !i.mock {
		var err error

		i.deviceManager, err = ironlib.New(i.logger.Logger)
		if err != nil {
			return err
		}
	}

	i.logger.Info("inband: GetBIOSConfiguration launching...")
	biosConfig, err := i.deviceManager.GetBIOSConfiguration(ctx)
	if err != nil {
		i.logger.WithFields(logrus.Fields{"err": err}).Error("inband: GetBIOSConfiguration error")
		return err
	}

	asset.BiosConfig = biosConfig

	i.logger.Info("inband: GetBIOSConfiguration success")
	return nil
}
