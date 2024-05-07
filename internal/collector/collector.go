package collector

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/metal-toolbox/alloy/internal/app"
	ci "github.com/metal-toolbox/alloy/internal/backend/componentinventory"
	fleetdb "github.com/metal-toolbox/alloy/internal/backend/fleetdb"
	"github.com/metal-toolbox/alloy/internal/device"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/types"
	cisclient "github.com/metal-toolbox/component-inventory/pkg/api/client"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	ErrBiosCfgCollect   = errors.New("error collecting BIOS configuration")
	ErrInventoryCollect = errors.New("error collecting inventory data")
)

// DeviceCollector holds attributes to collect inventory, bios configuration data from a single device.
type DeviceCollector struct {
	queryor       device.Queryor
	fleetDBClient *fleetdb.Client
	cisClient     cisclient.Client
	kind          model.AppKind
	log           *logrus.Logger
}

// NewDeviceCollector is a constructor method to return a inventory, bios configuration data collector.
func NewDeviceCollector(ctx context.Context, appKind model.AppKind, cfg *app.Configuration, logger *logrus.Logger) (*DeviceCollector, error) {
	fleetDBClient, err := fleetdb.New(ctx, appKind, cfg.FleetDBOptions, logger)
	if err != nil {
		return nil, err
	}

	queryor, err := device.NewQueryor(appKind, logger)
	if err != nil {
		return nil, err
	}

	cisClient, err := ci.NewComponentInventoryClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &DeviceCollector{
		kind:          appKind,
		queryor:       queryor,
		cisClient:     cisClient,
		fleetDBClient: fleetDBClient,
		log:           logger,
	}, nil
}

// NewDeviceCollectorWithStore is a constructor method that accepts an initialized store repository - to return a inventory, bios configuration data collector.
func NewDeviceCollectorWithStore(ctx context.Context, fleetDBClient *fleetdb.Client, appKind model.AppKind, cfg *app.Configuration, logger *logrus.Logger) (*DeviceCollector, error) {
	queryor, err := device.NewQueryor(appKind, logger)
	if err != nil {
		return nil, err
	}

	cisClient, err := ci.NewComponentInventoryClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &DeviceCollector{
		kind:          appKind,
		queryor:       queryor,
		cisClient:     cisClient,
		fleetDBClient: fleetDBClient,
		log:           logger,
	}, nil
}

// CollectOutofbandAndUploadToCIS querys inventory and bios configuration data for a device through its BMC.
func (c *DeviceCollector) CollectOutofbandAndUploadToCIS(ctx context.Context, assetID string, outputStdout bool) error {
	// fetch existing asset information from FleetDB
	loginInfo, err := c.fleetDBClient.BMCCredentials(ctx, assetID)
	if err != nil {
		c.log.WithField("error", err).Warn("getting BMC credentials")
		return err
	}

	// collect inventory
	inventory, err := c.queryor.Inventory(ctx, loginInfo)
	if err != nil {
		c.log.WithField("error", err).Warn("collecting inventory out of band")
		return err
	}

	biosCfg, err := c.queryor.BiosConfiguration(ctx, loginInfo)
	// collect BIOS configurations
	if err != nil {
		//c.log.WithField("error", err).Info("collecting bios configuration")
		// getting the bios configuration on inventory is an elective
		err = nil
	}

	cisInventory := &types.InventoryDevice{
		Inv:     inventory,
		BiosCfg: biosCfg,
	}

	if outputStdout {
		return c.prettyPrintJSON(cisInventory)
	}

	if inventory.BIOS.Firmware == nil {
		c.log.Warn("BIOS firmware is nil")
	} else {
		c.log.WithFields(logrus.Fields{
			"installed": inventory.BIOS.Firmware.Installed,
		}).Info("collected bios firmware")
	}

	// upload to CIS
	if _, err = c.cisClient.UpdateOutOfbandInventory(ctx, assetID, cisInventory); err != nil {
		c.log.WithFields(logrus.Fields{
			"error": err,
		}).Warn("unable to update component-inventory")
		return err
	}

	return nil
}

// CollectInbandAndUploadToCIS querys inventory and bios configuration data for a device through the host OS
// this expects Alloy is running within the alloy-inband docker image based on ironlib.
func (c *DeviceCollector) CollectInbandAndUploadToCIS(ctx context.Context, assetID string, outputStdout bool) error {
	var errs error

	// collect inventory
	inventory, err := c.queryor.Inventory(ctx, nil)
	if err != nil {
		errs = multierror.Append(errs, err)
	}

	// collect BIOS configurations
	biosCfg, err := c.queryor.BiosConfiguration(ctx, nil)
	if err != nil {
		errs = multierror.Append(errs, err)
	}

	cisInventory := &types.InventoryDevice{
		Inv:     inventory,
		BiosCfg: biosCfg,
	}

	if outputStdout {
		if err != nil {
			return err
		}

		return c.prettyPrintJSON(cisInventory)
	}

	if _, err = c.cisClient.UpdateInbandInventory(ctx, assetID, cisInventory); err != nil {
		errs = multierror.Append(errs, err)

		return errs
	}

	return nil
}

func (c *DeviceCollector) prettyPrintJSON(asset *types.InventoryDevice) error {
	b, err := json.MarshalIndent(asset, "", " ")
	if err != nil {
		return err
	}

	fmt.Print(string(b))

	return nil
}
