package publish

import (
	"context"
	"os"
	"reflect"
	"sync"

	"github.com/gammazero/workerpool"
	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/collect"
	"github.com/metal-toolbox/alloy/internal/helpers"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sanity-io/litter"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	r3diff "github.com/r3labs/diff/v3"
	serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

const (
	KindServerService = "serverService"
)

var (
	// concurrent requests
	concurrency = 5

	ErrSlugs                   = errors.New("slugs error")
	ErrServerServiceQuery      = errors.New("error in server service query")
	ErrRegisterChanges         = errors.New("error in server service register changes")
	ErrAssetObjectConversion   = errors.New("error converting asset object")
	ErrChangeList              = errors.New("error building change list")
	ErrServerServiceAttrObject = errors.New("error in server service attribute object")
	// The serverservice publisher tracer
	tracer trace.Tracer
)

func init() {
	tracer = otel.Tracer("publisher-serverservice")
}

// serverServicePublisher publishes asset inventory to serverService
type serverServicePublisher struct {
	logger                       *logrus.Entry
	config                       *model.Config
	syncWg                       *sync.WaitGroup
	collectorCh                  <-chan *model.Asset
	termCh                       <-chan os.Signal
	workers                      *workerpool.WorkerPool
	client                       *serverservice.Client
	slugs                        map[string]*serverservice.ServerComponentType
	firmwares                    map[string][]*serverservice.ComponentFirmwareVersion
	attributeNS                  string
	firmwareVersionedAttributeNS string
	statusVersionedAttributeNS   string
}

// NewServerServicePublisher returns a serverService publisher to submit inventory data.
func NewServerServicePublisher(ctx context.Context, alloy *app.App) (Publisher, error) {
	logger := app.NewLogrusEntryFromLogger(logrus.Fields{"component": "publisher-serverService"}, alloy.Logger)

	client, err := helpers.NewServerServiceClient(ctx, alloy.Config, logger)
	if err != nil {
		return nil, err
	}

	p := &serverServicePublisher{
		logger:                       logger,
		config:                       alloy.Config,
		syncWg:                       alloy.SyncWg,
		collectorCh:                  alloy.CollectorCh,
		termCh:                       alloy.TermCh,
		workers:                      workerpool.New(concurrency),
		client:                       client,
		slugs:                        make(map[string]*serverservice.ServerComponentType),
		firmwares:                    make(map[string][]*serverservice.ComponentFirmwareVersion),
		attributeNS:                  model.ServerComponentAttributeNS(alloy.Config.AppKind),
		firmwareVersionedAttributeNS: model.ServerComponentFirmwareNS(alloy.Config.AppKind),
		statusVersionedAttributeNS:   model.ServerComponentStatusNS(alloy.Config.AppKind),
	}

	return p, nil
}

// PublishOne publishes the given asset to the server service asset store.
//
// PublishOne implements the Publisher interface.
func (h *serverServicePublisher) PublishOne(ctx context.Context, device *model.Asset) error {
	if device == nil {
		return nil
	}

	// attach child span
	ctx, span := tracer.Start(ctx, "PublishOne()")
	defer span.End()

	// cache server component types for lookups
	err := h.cacheServerComponentTypes(ctx)
	if err != nil {
		return err
	}

	// cache server component firmwares for lookups
	err = h.cacheServerComponentFirmwares(ctx)
	if err != nil {
		return err
	}

	h.publish(ctx, device)

	return nil
}

// RunInventoryPublisher spawns a device inventory publisher that iterates over the device objects received
// on the collector channel and publishes them to the server service asset store.
//
// RunInventoryPublisher implements the Publisher interface.
func (h *serverServicePublisher) RunInventoryPublisher(ctx context.Context) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "RunInventoryPublisher()")
	defer span.End()

	// cache server component types for lookups
	err := h.cacheServerComponentTypes(ctx)
	if err != nil {
		return err
	}

	// cache server component firmwares for lookups
	err = h.cacheServerComponentFirmwares(ctx)
	if err != nil {
		return err
	}

	for asset := range h.collectorCh {
		if asset == nil || asset.Inventory == nil {
			continue
		}

		// count asset received on the collector channel
		metrics.AssetsReceived.With(stageLabel).Inc()

		// count dispatched worker task
		metrics.TasksDispatched.With(stageLabel).Inc()

		h.publish(ctx, asset)
	}

	return nil
}

func (h *serverServicePublisher) cacheServerComponentFirmwares(ctx context.Context) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "cacheServerComponentFirmwares()")
	defer span.End()

	// Query ServerService for all firmware
	firmwares, _, err := h.client.ListServerComponentFirmware(ctx, nil)
	if err != nil {
		// count error
		metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()

		// set span status
		span.SetStatus(codes.Error, "ListServerComponentFirmware() failed")

		return err
	}

	for idx := range firmwares {
		vendor := firmwares[idx].Vendor
		h.firmwares[vendor] = append(h.firmwares[vendor], &firmwares[idx])
	}

	return nil
}

// publish device information with hollow server service
// nolint:gocyclo // accept cyclomatic complexity here since most of the branches are error checks.
func (h *serverServicePublisher) publish(ctx context.Context, device *model.Asset) {
	// attach child span
	ctx, span := tracer.Start(ctx, "publish()")
	defer span.End()

	if device == nil {
		h.logger.Warn("nil device ignored")

		return
	}

	id, err := uuid.Parse(device.ID)
	if err != nil {
		h.logger.WithField("err", err).Warn("invalid device ID")

		return
	}

	// 1. retrieve server object, no action if the server doesn't exist
	server, hr, err := h.client.Get(ctx, id)
	if err != nil {
		h.logger.WithFields(
			logrus.Fields{
				"err":      err,
				"id":       id,
				"response": hr,
			}).Warn("server service server query returned error")

		// set span status
		span.SetStatus(codes.Error, "Get() server failed")

		// count error
		metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()

		return
	}

	if server == nil {
		h.logger.WithFields(
			logrus.Fields{
				"id": id,
				"hr": hr,
			}).Warn("server service server query returned nil object")

		return
	}

	// create/update server bmc error attributes - for out of band data collection
	if h.config.AppKind == model.AppKindOutOfBand && len(device.Errors) > 0 {
		err = h.createUpdateServerBMCErrorAttributes(
			ctx,
			server.UUID,
			attributeByNamespace(model.ServerBMCErrorsAttributeNS, server.Attributes),
			device,
		)

		if err != nil {
			h.logger.WithFields(
				logrus.Fields{
					"id":  id,
					"err": err,
				}).Warn("error in server bmc error attributes update")
		}

		// both inventory and BIOS configuration collection failed on a login failure
		if device.HasError(collect.OutofbandLoginError) {
			// count devices with errors
			metricInventorized.With(prometheus.Labels{"status": "failed"}).Add(1)
			return
		}
	}

	// count devices with no errors
	// TODO: metric label has to be updated to status-inventory, status-bioscfgcollection
	metricInventorized.With(prometheus.Labels{"status": "success"}).Add(1)

	if !device.HasError(collect.OutofbandInventoryError) {
		h.publishServerInventory(ctx, server, device)
	}

	// Don't publish bios config if there's no data or theres was an error in collection
	if len(device.BiosConfig) == 0 || device.HasError(collect.OutofbandGetBiosConfigError) {
		return
	}

	err = h.createUpdateServerBIOSConfiguration(ctx, server.UUID, device.BiosConfig)
	if err != nil {
		h.logger.WithFields(
			logrus.Fields{
				"id":  server.UUID.String(),
				"err": err,
			}).Warn("error in server bios configuration versioned attribute update")
	}
}

func (h *serverServicePublisher) publishServerInventory(ctx context.Context, server *serverservice.Server, device *model.Asset) {
	// create/update server serial, vendor, model attributes
	err := h.createUpdateServerAttributes(ctx, server, device)
	if err != nil {
		h.logger.WithFields(
			logrus.Fields{
				"id":  server.UUID.String(),
				"err": err,
			}).Warn("error in server attributes update")
	}

	// create update server metadata attributes
	err = h.createUpdateServerMetadataAttributes(ctx, server.UUID, device)
	if err != nil {
		h.logger.WithFields(
			logrus.Fields{
				"id":  server.UUID.String(),
				"err": err,
			}).Warn("error in server metadata attributes update")
	}

	// create update server component
	err = h.createUpdateServerComponents(ctx, server.UUID, device)
	if err != nil {
		h.logger.WithFields(
			logrus.Fields{
				"id":  server.UUID.String(),
				"err": err,
			}).Warn("error converting device object")
	}
}

// createUpdateServerComponents compares the current object in serverService with the device data and creates/updates server component data.
//
// nolint:gocyclo // the method caries out all steps to have device data compared and registered, for now its accepted as cyclomatic.
func (h *serverServicePublisher) createUpdateServerComponents(ctx context.Context, serverID uuid.UUID, device *model.Asset) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "createUpdateServerComponents()")
	defer span.End()

	// convert model.AssetDevice to server service component slice
	newInventory, err := h.toComponentSlice(serverID, device)
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
	currentInventory, _, err := h.client.GetComponents(ctx, serverID, &serverservice.PaginationParams{})
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
		h.logger.Info("components current fixture dumped as file: " + current)

		// nolint:gomnd // file permissions are clearer in this form.
		_ = os.WriteFile(current, []byte(litter.Sdump(currentInventory)), 0o600)

		newc := serverID.String() + ".new.components.fixture"
		h.logger.Info("components new fixture dumped as file: " + newc)

		// nolint:gomnd // file permissions are clearer in this form.
		_ = os.WriteFile(newc, []byte(litter.Sdump(newInventory)), 0o600)
	}

	// convert to a pointer slice since this data is passed around
	currentInventoryPtrSlice := componentPtrSlice(currentInventory)

	// in place filter attributes, versioned attributes that are not of relevance to this instance of Alloy.
	h.filterByAttributeNamespace(currentInventoryPtrSlice)

	// identify changes to be applied
	add, update, remove, err := serverServiceChangeList(ctx, currentInventoryPtrSlice, newInventory)
	if err != nil {
		return errors.Wrap(ErrRegisterChanges, err.Error())
	}

	if len(add) == 0 && len(update) == 0 && len(remove) == 0 {
		h.logger.WithField("serverID", serverID).Debug("no changes identified to register.")

		return nil
	}

	h.logger.WithFields(
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

		_, err = h.client.CreateComponents(ctx, serverID, add)
		if err != nil {
			// count error
			metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()

			// set span status
			span.SetStatus(codes.Error, "CreateComponents() failed")

			return errors.Wrap(ErrRegisterChanges, "CreateComponents: "+err.Error())
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

		_, err = h.client.UpdateComponents(ctx, serverID, update)
		if err != nil {
			// count error
			metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()

			// set span status
			span.SetStatus(codes.Error, "UpdateComponents() failed")

			return errors.Wrap(ErrRegisterChanges, "UpdateComponents: "+err.Error())
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

	h.logger.WithFields(
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
func diffFilter(path []string, parent reflect.Type, field reflect.StructField) bool {
	switch field.Name {
	case "CreatedAt", "UpdatedAt", "LastReportedAt":
		return false
	default:
		return true
	}
}

// serverServiceChangeList compares the current vs newer slice of server components
// and returns 3 lists - add, update, remove.
func serverServiceChangeList(ctx context.Context, currentObjs, newObjs []*serverservice.ServerComponent) (add, update, remove serverservice.ServerComponentSlice, err error) {
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

func serverServiceComponentsUpdated(currentObj, newObj *serverservice.ServerComponent) (*serverservice.ServerComponent, error) {
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
