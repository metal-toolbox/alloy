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
	fleetDBClient, err := fleetdb.New(ctx, appKind, cfg.ServerserviceOptions, logger)
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
	var errs error

	// fetch existing asset information from inventory
	loginInfo, err := c.fleetDBClient.BMCCredentials(ctx, assetID)
	if err != nil {
		errs = multierror.Append(errs, err)

		return errs
	}

	// collect inventory
	inventory, err := c.queryor.Inventory(ctx, loginInfo)
	if err != nil {
		errs = multierror.Append(errs, err)
	}

	biosCfg, err := c.queryor.BiosConfiguration(ctx, loginInfo)

	// collect BIOS configurations
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

	// upload to CIS
	if _, err = c.cisClient.UpdateInbandInventory(ctx, assetID, cisInventory); err != nil {
		errs = multierror.Append(errs, err)

		return errs
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
