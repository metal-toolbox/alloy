package serverservice

import (
	"context"
	"os"
	"reflect"
	"strings"

	"github.com/bmc-toolbox/common"
	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	r3diff "github.com/r3labs/diff/v3"
	"github.com/sanity-io/litter"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	serverserviceapi "go.hollow.sh/serverservice/pkg/api/v1"
)

var (

	// The serverservice asset getter tracer
	tracer trace.Tracer
)

func init() {
	tracer = otel.Tracer("store.serverservice")
}

// Store is an asset inventory store
type Store struct {
	*serverserviceapi.Client
	logger                       *logrus.Entry
	config                       *app.ServerserviceOptions
	slugs                        map[string]*serverserviceapi.ServerComponentType
	firmwares                    map[string][]*serverserviceapi.ComponentFirmwareVersion
	appKind                      model.AppKind
	attributeNS                  string
	firmwareVersionedAttributeNS string
	statusVersionedAttributeNS   string
	facilityCode                 string
}

// NewStore returns a serverservice store queryor to lookup and publish assets to, from the store.
func New(ctx context.Context, appKind model.AppKind, cfg *app.ServerserviceOptions, logger *logrus.Logger) (*Store, error) {
	loggerEntry := app.NewLogrusEntryFromLogger(
		logrus.Fields{"component": "store.serverservice"},
		logger,
	)

	apiclient, err := NewServerServiceClient(ctx, cfg, loggerEntry)
	if err != nil {
		return nil, err
	}

	s := &Store{
		Client:                       apiclient,
		appKind:                      appKind,
		logger:                       loggerEntry,
		config:                       cfg,
		slugs:                        make(map[string]*serverserviceapi.ServerComponentType),
		firmwares:                    make(map[string][]*serverserviceapi.ComponentFirmwareVersion),
		attributeNS:                  serverComponentAttributeNS(appKind),
		firmwareVersionedAttributeNS: serverComponentFirmwareNS(appKind),
		statusVersionedAttributeNS:   serverComponentStatusNS(appKind),
		facilityCode:                 cfg.FacilityCode,
	}

	// add component types if they don't exist
	if err := s.createServerComponentTypes(ctx); err != nil {
		return nil, err
	}

	if err := s.cacheServerComponentTypes(ctx); err != nil {
		return nil, err
	}

	if err := s.cacheServerComponentFirmwares(ctx); err != nil {
		return nil, err
	}

	if len(s.slugs) == 0 {
		return nil, errors.Wrap(ErrSlugs, "required component slugs not found in serverservice")
	}

	return s, nil
}

// Kind returns the repository store kind.
func (r *Store) Kind() model.StoreKind {
	return model.StoreKindServerservice
}

// assetByID queries serverService for the hardware asset by ID and returns an Asset object
func (r *Store) AssetByID(ctx context.Context, id string, fetchBmcCredentials bool) (*model.Asset, error) {
	// attach child span
	ctx, span := tracer.Start(ctx, "AssetByID()")
	defer span.End()

	sid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}

	// get server
	server, _, err := r.Get(ctx, sid)
	if err != nil {
		span.SetStatus(codes.Error, "Get() server failed")

		return nil, errors.Wrap(ErrServerServiceQuery, "error querying server attributes: "+err.Error())
	}

	var credential *serverserviceapi.ServerCredential

	if fetchBmcCredentials {
		var err error

		// get bmc credential
		credential, _, err = r.GetCredential(ctx, sid, serverserviceapi.ServerCredentialTypeBMC)
		if err != nil {
			span.SetStatus(codes.Error, "GetCredential() failed")

			return nil, errors.Wrap(ErrServerServiceQuery, "error querying BMC credentials: "+err.Error())
		}
	}

	return toAsset(server, credential, fetchBmcCredentials)
}

// assetByID queries serverService for the hardware asset by ID and returns an Asset object
func (r *Store) AssetsByOffsetLimit(ctx context.Context, offset, limit int) (assets []*model.Asset, totalAssets int, err error) {
	// attach child span
	ctx, span := tracer.Start(ctx, "AssetsByOffsetLimit()")

	span.SetAttributes(
		attribute.Int("offset", offset),
		attribute.Int("limit", limit),
	)

	defer span.End()

	params := &serverserviceapi.ServerListParams{
		FacilityCode: r.facilityCode,
		AttributeListParams: []serverserviceapi.AttributeListParams{
			{
				Namespace: bmcAttributeNamespace,
			},
		},
		PaginationParams: &serverserviceapi.PaginationParams{
			Limit: limit,
			Page:  offset,
		},
	}

	// list servers
	servers, response, err := r.List(ctx, params)
	if err != nil {
		span.SetStatus(codes.Error, "List() servers failed")

		return nil, 0, errors.Wrap(ErrServerServiceQuery, err.Error())
	}

	assets = make([]*model.Asset, 0, len(servers))

	// collect bmc secrets and structure as alloy asset
	for _, server := range serverPtrSlice(servers) {
		credential, _, err := r.GetCredential(ctx, server.UUID, serverserviceapi.ServerCredentialTypeBMC)
		if err != nil {
			span.SetStatus(codes.Error, "GetCredential() failed")

			return nil, 0, errors.Wrap(ErrServerServiceQuery, err.Error())
		}

		asset, err := toAsset(server, credential, true)
		if err != nil {
			r.logger.Warn(err)
			continue
		}

		assets = append(assets, asset)
	}

	return assets, int(response.TotalRecordCount), nil
}

// AssetUpdate inserts/updates the asset data in the serverservice store
func (r *Store) AssetUpdate(ctx context.Context, asset *model.Asset) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "AssetUpdate()")
	defer span.End()

	if asset == nil {
		return errors.Wrap(ErrAssetObject, "is nil")
	}

	id, err := uuid.Parse(asset.ID)
	if err != nil {
		return errors.Wrap(ErrAssetObject, "invalid device ID")
	}

	// 1. retrieve current server object.
	server, hr, err := r.Get(ctx, id)
	if err != nil {
		r.logger.WithFields(
			logrus.Fields{
				"err":      err,
				"id":       id,
				"response": hr,
			}).Warn("server service server query returned error")

		// set span status
		span.SetStatus(codes.Error, "Get() server failed")

		// count error
		metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()

		return errors.Wrap(ErrServerServiceQuery, err.Error())
	}

	if server == nil {
		r.logger.WithFields(
			logrus.Fields{
				"id": id,
				"hr": hr,
			}).Warn("server service server query returned nil object")

		return errors.Wrap(ErrServerServiceQuery, "got nil Server object")
	}

	// create/update inventory.
	if errInventory := r.createUpdateInventory(ctx, asset, server); errInventory != nil {
		r.logger.WithFields(
			logrus.Fields{
				"id":  id,
				"hr":  hr,
				"err": errInventory.Error(),
			}).Warn("inventory asset insert/update error")

		metricInventorized.With(prometheus.Labels{"status": "failed"}).Add(1)
	} else {
		// count devices with no errors
		metricInventorized.With(prometheus.Labels{"status": "success"}).Add(1)
	}

	// Don't publish bios config if there's no data
	if len(asset.BiosConfig) != 0 {
		err = r.createUpdateServerBIOSConfiguration(ctx, server.UUID, asset.BiosConfig)
		if err != nil {
			metricBiosCfgCollected.With(prometheus.Labels{"status": "failed"}).Add(1)

			r.logger.WithFields(
				logrus.Fields{
					"id":  server.UUID.String(),
					"err": err,
				}).Warn("error in server bios configuration versioned attribute update")
		}

		metricInventorized.With(prometheus.Labels{"status": "success"}).Add(1)
	}

	return nil
}

func (r *Store) createUpdateInventory(ctx context.Context, device *model.Asset, server *serverserviceapi.Server) error {
	// create/update server bmc error attributes - for out of band data collection
	if r.appKind == model.AppKindOutOfBand && len(device.Errors) > 0 {
		if err := r.createUpdateServerBMCErrorAttributes(
			ctx,
			server.UUID,
			attributeByNamespace(serverBMCErrorsAttributeNS, server.Attributes),
			device,
		); err != nil {
			return errors.Wrap(ErrServerServiceQuery, "BMC error attribute create/update error: "+err.Error())
		}
	}

	// create/update server serial, vendor, model attributes
	if err := r.createUpdateServerAttributes(ctx, server, device); err != nil {
		return errors.Wrap(ErrServerServiceQuery, "Server Vendor attribute create/update error: "+err.Error())
	}

	// create update server metadata attributes
	if err := r.createUpdateServerMetadataAttributes(ctx, server.UUID, device); err != nil {
		return errors.Wrap(ErrServerServiceQuery, "Server Metadata attribute create/update error: "+err.Error())
	}

	// create update server component
	if err := r.createUpdateServerComponents(ctx, server.UUID, device); err != nil {
		return errors.Wrap(ErrServerServiceQuery, "Server Component create/update error: "+err.Error())
	}

	return nil
}

// createUpdateServerComponents compares the current object in serverService with the device data and creates/updates server component data.
//
// nolint:gocyclo // the method caries out all steps to have device data compared and registered, for now its accepted as cyclomatic.
func (r *Store) createUpdateServerComponents(ctx context.Context, serverID uuid.UUID, device *model.Asset) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "createUpdateServerComponents()")
	defer span.End()

	if device.Inventory == nil {
		return nil
	}

	// convert model.AssetDevice to server service component slice
	newInventory, err := r.toComponentSlice(serverID, device)
	if err != nil {
		return errors.Wrap(ErrAssetObjectConversion, err.Error())
	}

	// measure number of components identified by the publisher in a device.
	metricAssetComponentsIdentified.With(
		metrics.AddLabels(
			stageLabel,
			prometheus.Labels{"vendor": device.Vendor, "model": device.Model},
		),
	).Add(float64(len(newInventory)))

	// retrieve current inventory from server service
	currentInventory, _, err := r.GetComponents(ctx, serverID, &serverserviceapi.PaginationParams{})
	if err != nil {
		// count error
		metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()

		// set span status
		span.SetStatus(codes.Error, "GetComponents() failed")

		return errors.Wrap(ErrServerServiceQuery, err.Error())
	}

	// For debugging and to capture test fixtures data.
	if os.Getenv(model.EnvVarDumpFixtures) == "true" {
		current := serverID.String() + ".current.components.fixture"
		r.logger.Info("components current fixture dumped as file: " + current)

		// nolint:gomnd // file permissions are clearer in this form.
		_ = os.WriteFile(current, []byte(litter.Sdump(currentInventory)), 0o600)

		newc := serverID.String() + ".new.components.fixture"
		r.logger.Info("components new fixture dumped as file: " + newc)

		// nolint:gomnd // file permissions are clearer in this form.
		_ = os.WriteFile(newc, []byte(litter.Sdump(newInventory)), 0o600)
	}

	// convert to a pointer slice since this data is passed around
	currentInventoryPtrSlice := componentPtrSlice(currentInventory)

	// in place filter attributes, versioned attributes that are not of relevance to this instance of Alloy.
	r.filterByAttributeNamespace(currentInventoryPtrSlice)

	// identify changes to be applied
	add, update, remove, err := serverServiceChangeList(ctx, currentInventoryPtrSlice, newInventory)
	if err != nil {
		return errors.Wrap(ErrServerServiceQuery, err.Error())
	}

	if len(add) == 0 && len(update) == 0 && len(remove) == 0 {
		r.logger.WithField("serverID", serverID).Debug("no changes identified to register.")

		return nil
	}

	r.logger.WithFields(
		logrus.Fields{
			"serverID":           serverID,
			"components-added":   len(add),
			"components-updated": len(update),
			"components-removed": len(remove),
		}).Info("device inventory changes to be registered")

	// apply added component changes
	if len(add) > 0 {
		// count components added
		metricServerServiceDataChanges.With(
			metrics.AddLabels(
				stageLabel,
				prometheus.Labels{
					"change_kind": "components-added",
				},
			),
		).Add(float64(len(add)))

		_, err = r.CreateComponents(ctx, serverID, add)
		if err != nil {
			// count error
			metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()

			// set span status
			span.SetStatus(codes.Error, "CreateComponents() failed")

			return errors.Wrap(ErrServerServiceQuery, "CreateComponents: "+err.Error())
		}
	}

	// apply updated component changes
	if len(update) > 0 {
		// count components updated
		metricServerServiceDataChanges.With(
			metrics.AddLabels(
				stageLabel,
				prometheus.Labels{
					"change_kind": "components-updated",
				},
			),
		).Add(float64(len(update)))

		_, err = r.UpdateComponents(ctx, serverID, update)
		if err != nil {
			// count error
			metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()

			// set span status
			span.SetStatus(codes.Error, "UpdateComponents() failed")

			return errors.Wrap(ErrServerServiceQuery, "UpdateComponents: "+err.Error())
		}
	}

	// Alloy should not be removing components based on diff data
	// this is because data collected out of band differs from data collected inband
	// and so this remove section is not going to be implemented.
	//
	// What can be done instead is that Alloy touches a timestamp value
	// on each component and if that timestamp value has not been updated
	// for a certain amount of time, compared to the rest of the component timestamps
	// then the component may be eligible for removal.
	if len(remove) > 0 {
		// count components removed
		metricServerServiceDataChanges.With(
			metrics.AddLabels(
				stageLabel,
				prometheus.Labels{
					"change_kind": "components-removed",
				},
			),
		).Add(float64(len(remove)))
	}

	r.logger.WithFields(
		logrus.Fields{
			"serverID":      serverID,
			"added":         len(add),
			"updated":       len(update),
			"(not) removed": len(remove),
		}).Debug("registered inventory changes with server service")

	return nil
}

// diffFilter is a filter passed to the r3 diff filter method for comparing structs
//
// nolint:gocritic // r3diff requires the field attribute to be passed by value
func diffFilter(_ []string, _ reflect.Type, field reflect.StructField) bool {
	switch field.Name {
	case "CreatedAt", "UpdatedAt", "LastReportedAt":
		return false
	default:
		return true
	}
}

// serverServiceChangeList compares the current vs newer slice of server components
// and returns 3 lists - add, update, remove.
func serverServiceChangeList(ctx context.Context, currentObjs, newObjs []*serverserviceapi.ServerComponent) (add, update, remove serverserviceapi.ServerComponentSlice, err error) {
	// attach child span
	_, span := tracer.Start(ctx, "serverServiceChangeList()")
	defer span.End()

	// 1. list updated and removed objects
	for _, currentObj := range currentObjs {
		// changeObj is the component changes to be registered
		changeObj := componentBySlugSerial(currentObj.ComponentTypeSlug, currentObj.Serial, newObjs)

		// component not found - add to remove list
		if changeObj == nil {
			remove = append(remove, *currentObj)
			continue
		}

		updated, err := serverServiceComponentsUpdated(currentObj, changeObj)
		if err != nil {
			return add, update, remove, err
		}

		// no objects with changes identified
		if updated == nil {
			continue
		}

		// updates identified, include object as an update
		update = append(update, *updated)
	}

	// 2. list new objects
	for _, newObj := range newObjs {
		changeObj := componentBySlugSerial(newObj.ComponentTypeSlug, newObj.Serial, currentObjs)

		if changeObj == nil {
			add = append(add, *newObj)
		}
	}

	return add, update, remove, nil
}

func serverServiceComponentsUpdated(currentObj, newObj *serverserviceapi.ServerComponent) (*serverserviceapi.ServerComponent, error) {
	differ, err := r3diff.NewDiffer(r3diff.Filter(diffFilter))
	if err != nil {
		return nil, err
	}

	objChanges, err := differ.Diff(currentObj, newObj)
	if err != nil {
		return nil, err
	}

	// no changes in object
	if len(objChanges) == 0 {
		return nil, nil
	}

	// For debugging dump differ data
	if os.Getenv(model.EnvVarDumpDiffers) == "true" {
		objChangesf := currentObj.ServerUUID.String() + ".objchanges.diff"

		// nolint:gomnd // file permissions are clearer in this form.
		_ = os.WriteFile(objChangesf, []byte(litter.Sdump(objChanges)), 0o600)
	}

	// compare attributes, versioned attributes
	attributes, versionedAttributes, err := diffComponentObjectsAttributes(currentObj, newObj)
	if err != nil {
		return nil, err
	}

	// no changes in attributes, versioned attributes
	if len(attributes) == 0 && len(versionedAttributes) == 0 {
		return nil, nil
	}

	newObj.Attributes = nil
	newObj.VersionedAttributes = nil

	if len(attributes) > 0 {
		newObj.Attributes = attributes
	}

	if len(versionedAttributes) > 0 {
		newObj.VersionedAttributes = versionedAttributes
	}

	newObj.UUID = currentObj.UUID

	return newObj, nil
}

func (r *Store) cacheServerComponentFirmwares(ctx context.Context) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "cacheServerComponentFirmwares()")
	defer span.End()

	// Query ServerService for all firmware
	firmwares, _, err := r.ListServerComponentFirmware(ctx, nil)
	if err != nil {
		// count error
		metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()

		// set span status
		span.SetStatus(codes.Error, "ListServerComponentFirmware() failed")

		return err
	}

	for idx := range firmwares {
		vendor := firmwares[idx].Vendor
		r.firmwares[vendor] = append(r.firmwares[vendor], &firmwares[idx])
	}

	return nil
}

func (r *Store) cacheServerComponentTypes(ctx context.Context) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "cacheServerComponentTypes()")
	defer span.End()

	serverComponentTypes, _, err := r.ListServerComponentTypes(ctx, nil)
	if err != nil {
		// count error
		metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()

		// set span status
		span.SetStatus(codes.Error, "ListServerComponentTypes() failed")

		return err
	}

	for _, ct := range serverComponentTypes {
		r.slugs[ct.Slug] = ct
	}

	return nil
}

func (r *Store) createServerComponentTypes(ctx context.Context) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "createServerComponentTypes()")
	defer span.End()

	existing, _, err := r.ListServerComponentTypes(ctx, nil)
	if err != nil {
		// count error
		metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()

		// set span status
		span.SetStatus(codes.Error, "ListServerComponentTypes() failed")

		return err
	}

	if len(existing) > 0 {
		return nil
	}

	componentSlugs := []string{
		common.SlugBackplaneExpander,
		common.SlugChassis,
		common.SlugTPM,
		common.SlugGPU,
		common.SlugCPU,
		common.SlugPhysicalMem,
		common.SlugStorageController,
		common.SlugBMC,
		common.SlugBIOS,
		common.SlugDrive,
		common.SlugDriveTypePCIeNVMEeSSD,
		common.SlugDriveTypeSATASSD,
		common.SlugDriveTypeSATAHDD,
		common.SlugNIC,
		common.SlugPSU,
		common.SlugCPLD,
		common.SlugEnclosure,
		common.SlugUnknown,
		common.SlugMainboard,
	}

	for _, slug := range componentSlugs {
		sct := serverserviceapi.ServerComponentType{
			Name: slug,
			Slug: strings.ToLower(slug),
		}

		_, err := r.CreateServerComponentType(ctx, sct)
		if err != nil {
			return err
		}
	}

	return nil
}
