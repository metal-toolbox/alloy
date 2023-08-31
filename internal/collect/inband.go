package collect

import (
	"context"
	"os"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/ironlib"
	"github.com/metal-toolbox/ironlib/actions"
	"github.com/sirupsen/logrus"
)

// Inband collector collects hardware, firmware inventory inband
type InbandCollector struct {
	deviceManager actions.DeviceManager
	logger        *logrus.Entry
	collectorCh   chan<- *model.Asset
	termCh        <-chan os.Signal
	mock          bool
}

// New returns an inband inventory collector
func NewInbandCollector(alloy *app.App) Collector {
	logger := app.NewLogrusEntryFromLogger(logrus.Fields{"component": "collector.inband"}, alloy.Logger)

	return &InbandCollector{
		logger:      logger,
		collectorCh: alloy.CollectorCh,
		termCh:      alloy.TermCh,
	}
}

func (i *InbandCollector) SetMockGetter(getter interface{}) {
	i.deviceManager = getter.(actions.DeviceManager)
	i.mock = true
}

// InventoryLocal implements the Collector interface to collect inventory and bios configuration locally (inband).
func (i *InbandCollector) InventoryLocal(ctx context.Context) (*model.Asset, error) {
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

	biosConfig, err := i.deviceManager.GetBIOSConfiguration(ctx)
	if err != nil {
		i.logger.WithFields(
			logrus.Fields{
				"err": err,
			}).Warn("error collecting server bios configuration")
	}

	device.Vendor = common.FormatVendorName(device.Vendor)

	// The "unknown" valued attributes here are to be filled in by the caller,
	// with the data from the inventory source when its available.
	return &model.Asset{Inventory: device, BiosConfig: biosConfig, Vendor: "unknown", Model: "unknown", Serial: "unknown"}, nil
}

// InventoryRemote implements is present here to satisfy the Collector interface.
func (i *InbandCollector) InventoryRemote(_ context.Context) error {
	return nil
}
