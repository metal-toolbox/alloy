package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/go-multierror"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/device"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/store"
	ci "github.com/metal-toolbox/alloy/internal/store/componentinventory"
	"github.com/metal-toolbox/alloy/internal/store/serverservice"
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
	queryor    device.Queryor
	repository *serverservice.Store
	cisClient  cisclient.Client
	kind       model.AppKind
	log        *logrus.Logger
}

// NewDeviceCollector is a constructor method to return a inventory, bios configuration data collector.
func NewDeviceCollector(ctx context.Context, storeKind model.StoreKind, appKind model.AppKind, cfg *app.Configuration, logger *logrus.Logger) (*DeviceCollector, error) {
	repository, err := store.NewRepository(ctx, storeKind, appKind, cfg, logger)
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
		kind:       appKind,
		queryor:    queryor,
		cisClient:  cisClient,
		repository: repository.(*serverservice.Store),
		log:        logger,
	}, nil
}

// NewDeviceCollectorWithStore is a constructor method that accepts an initialized store repository - to return a inventory, bios configuration data collector.
func NewDeviceCollectorWithStore(ctx context.Context, repository store.Repository, appKind model.AppKind, cfg *app.Configuration, logger *logrus.Logger) (*DeviceCollector, error) {
	queryor, err := device.NewQueryor(appKind, logger)
	if err != nil {
		return nil, err
	}

	cisClient, err := ci.NewComponentInventoryClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &DeviceCollector{
		kind:       appKind,
		queryor:    queryor,
		cisClient:  cisClient,
		repository: repository.(*serverservice.Store),
		log:        logger,
	}, nil
}

// CollectOutofbandAndUploadToCIS querys inventory and bios configuration data for a device through its BMC.
func (c *DeviceCollector) CollectOutofbandAndUploadToCIS(ctx context.Context, assetID string, outputStdout bool) error {
	var errs error

	// fetch existing asset information from inventory
	loginInfo, err := c.repository.BMCCredentials(ctx, assetID)
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
	if _, err = c.cisClient.UpdateOutOfbandInventory(ctx, assetID, cisInventory); err != nil {
		errs = multierror.Append(errs, err)

		return errs
	}

	return nil
}

// CollectInband querys inventory and bios configuration data for a device through the host OS
// this expects Alloy is running within the alloy-inband docker image based on ironlib.
func (c *DeviceCollector) CollectInband(ctx context.Context, asset *model.Asset, outputStdout bool) error {
	var errs error

	// XXX: This is duplicative! The asset is fetched again prior to updating serverservice.
	// fetch existing asset information from inventory
	existing, err := c.repository.AssetByID(ctx, asset.ID, c.kind == model.AppKindOutOfBand)
	if err != nil {
		c.log.WithError(err).Warn("getting asset by ID")
		errs = multierror.Append(errs, err)
	}

	c.log.WithFields(logrus.Fields{
		"found_existing": strconv.FormatBool(existing != nil),
	}).Info("asset by id complete")

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

	if _, err = c.cisClient.UpdateInbandInventory(ctx, asset.ID, cisInventory); err != nil {
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

// AssetIterCollector is not used by anyone based on https://github.com/search?q=repo%3Ametal-toolbox%2Falloy%20AssetIterCollector&type=code
//
// type AssetIterCollector struct {
// 	assetIterator AssetIterator
// 	queryor       device.Queryor
// 	repository    store.Repository
// 	syncWG        *sync.WaitGroup
// 	logger        *logrus.Logger
// 	concurrency   int32
// }
