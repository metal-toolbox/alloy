package collect

import (
	"context"
	"os"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/ironlib"
	ironlibm "github.com/metal-toolbox/ironlib/model"
	"github.com/sirupsen/logrus"
)

// Inband collector collects hardware, firmware inventory inband
type InbandCollector struct {
	deviceManager ironlibm.DeviceManager
	logger        *logrus.Entry
	collectorCh   chan<- *model.AssetDevice
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
	i.deviceManager = getter.(ironlibm.DeviceManager)
	i.mock = true
}

// Inventory implements the Collector interface to collect inventory inband
func (i *InbandCollector) Inventory(ctx context.Context) error {
	if !i.mock {
		var err error

		i.deviceManager, err = ironlib.New(i.logger.Logger)
		if err != nil {
			return err
		}
	}

	defer close(i.collectorCh)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			device, err := i.deviceManager.GetInventory(ctx)
			if err != nil {
				return err
			}

			i.collectorCh <- &model.AssetDevice{ID: "", Device: device}

			return nil
		}
	}
}
