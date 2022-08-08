package publish

import (
	"context"
	"os"
	"reflect"
	"sort"
	"sync"

	"github.com/gammazero/workerpool"
	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/helpers"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	r3diff "github.com/r3labs/diff/v3"
	serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

const (
	KindServerService = "serverService"
	namespace         = "server.components"
)

var (
	// concurrent requests
	concurrency = 5

	ErrSlugs                 = errors.New("slugs error")
	ErrServerServiceQuery    = errors.New("error in server service query")
	ErrRegisterChanges       = errors.New("error in server service register changes")
	ErrAssetObjectConversion = errors.New("error converting asset object")
	ErrChangeList            = errors.New("error building change list")
	// The serverservice publisher tracer
	tracer trace.Tracer
)

func init() {
	tracer = otel.Tracer("publisher-serverservice")
}

// serverServicePublisher publishes asset inventory to serverService
type serverServicePublisher struct {
	logger      *logrus.Entry
	config      *model.Config
	syncWg      *sync.WaitGroup
	collectorCh <-chan *model.AssetDevice
	termCh      <-chan os.Signal
	workers     *workerpool.WorkerPool
	client      *serverservice.Client
	slugs       map[string]*serverservice.ServerComponentType
}

// NewServerServicePublisher returns a serverService publisher to submit inventory data.
func NewServerServicePublisher(ctx context.Context, alloy *app.App) (Publisher, error) {
	logger := app.NewLogrusEntryFromLogger(logrus.Fields{"component": "publisher.serverService"}, alloy.Logger)

	client, err := helpers.NewServerServiceClient(alloy.Config, logger)
	if err != nil {
		return nil, err
	}

	p := &serverServicePublisher{
		logger:      logger,
		config:      alloy.Config,
		syncWg:      alloy.SyncWg,
		collectorCh: alloy.CollectorCh,
		termCh:      alloy.TermCh,
		workers:     workerpool.New(concurrency),
		client:      client,
		slugs:       make(map[string]*serverservice.ServerComponentType),
	}

	return p, nil
}

// Run implements the Publisher interface to publish asset inventory
//
// Listens on the collector channel for asset inventory send by the collector.
func (h *serverServicePublisher) Run(ctx context.Context) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "Run()")
	defer span.End()

	// cache server component types for lookups
	err := h.cacheServerComponentTypes(ctx)
	if err != nil {
		return err
	}

	for device := range h.collectorCh {
		// context canceled
		if ctx.Err() != nil {
			break
		}

		if device == nil {
			continue
		}

		// count asset received on the collector channel
		metrics.AssetsReceived.With(stageLabel).Inc()

		// count dispatched worker task
		metrics.TasksDispatched.With(stageLabel).Inc()

		h.publish(ctx, device)
	}

	return nil
}

// publish device information with hollow server service
func (h *serverServicePublisher) publish(ctx context.Context, device *model.AssetDevice) {
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

	err = h.registerChanges(ctx, server.UUID, device)
	if err != nil {
		h.logger.WithFields(
			logrus.Fields{
				"id":  server.UUID.String(),
				"err": err,
			}).Warn("error converting device object")
	}
}

// registerChanges compares the current object in serverService with the device data and registers changes.
//
// nolint:gocyclo // the method caries out all steps to have device data compared and registered, for now its accepted as cyclomatic.
func (h *serverServicePublisher) registerChanges(ctx context.Context, serverID uuid.UUID, device *model.AssetDevice) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "registerChanges()")
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

	// identify changes to be applied
	add, update, remove, err := serverServiceChangeList(ctx, componentPtrSlice(currentInventory), newInventory)
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

			return errors.Wrap(ErrRegisterChanges, err.Error())
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
		).Add(float64(len(add)))

		_, err = h.client.UpdateComponents(ctx, serverID, update)
		if err != nil {
			// count error
			metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()

			// set span status
			span.SetStatus(codes.Error, "UpdateComponents() failed")

			return errors.Wrap(ErrRegisterChanges, err.Error())
		}
	}

	if len(remove) > 0 {
		// count components removed
		metricServerServiceDataChanges.With(
			metrics.AddLabels(
				stageLabel,
				prometheus.Labels{
					"change_kind": "components-removed",
				},
			),
		).Add(float64(len(add)))

		return errors.Wrap(ErrRegisterChanges, "component deletion not implemented")
	}

	h.logger.WithFields(
		logrus.Fields{
			"serverID": serverID,
			"added":    len(add),
			"updated":  len(update),
			"removed":  len(remove),
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

func diffComponentObjectsAttributes(currentObj, changeObj *serverservice.ServerComponent) ([]serverservice.Attributes, []serverservice.VersionedAttributes, error) {
	var attributes []serverservice.Attributes

	var versionedAttributes []serverservice.VersionedAttributes

	differ, err := r3diff.NewDiffer(r3diff.Filter(diffFilter))
	if err != nil {
		return attributes, versionedAttributes, err
	}

	// compare attribute changes
	attributeObjChanges, err := differ.Diff(currentObj.Attributes, changeObj.Attributes)
	if err != nil {
		return attributes, versionedAttributes, err
	}

	if len(attributeObjChanges) > 0 {
		attributes = changeObj.Attributes
	}

	// compare versioned attributes
	//
	// the returned versioned attribute is to be included in the change object.
	vAttributeObjChange, err := diffVersionedAttributes(currentObj.VersionedAttributes, changeObj.VersionedAttributes)
	if err != nil {
		return attributes, versionedAttributes, err
	}

	if vAttributeObjChange != nil {
		versionedAttributes = append(versionedAttributes, *vAttributeObjChange)
	}

	return attributes, versionedAttributes, nil
}

// diffVersionedAttributes compares the current latest (created_at) versioned attribute
// with the newer versioned attribute (from the inventory collection)
// returning the versioned attribute to be registered with serverService.
//
// In the case that no changes are to be registered, a nil object is returned.
func diffVersionedAttributes(currentObjs, newObjs []serverservice.VersionedAttributes) (*serverservice.VersionedAttributes, error) {
	// no newObjects
	if len(newObjs) == 0 {
		return nil, nil
	}

	// no versioned attributes in current
	if len(newObjs) > 0 && len(currentObjs) == 0 {
		return &newObjs[0], nil
	}

	// identify current latest versioned attribute (sorted by created_at)
	var currentObj serverservice.VersionedAttributes

	sort.Slice(currentObjs, func(i, j int) bool {
		return currentObjs[i].CreatedAt.After(
			currentObjs[j].CreatedAt,
		)
	})

	currentObj = currentObjs[0]

	// differ currentObj with newObj
	differ, err := r3diff.NewDiffer(r3diff.Filter(diffFilter))
	if err != nil {
		return nil, err
	}

	changes, err := differ.Diff(currentObj, newObjs[0])
	if err != nil {
		return nil, err
	}

	if len(changes) > 0 {
		return &newObjs[0], err
	}

	return nil, nil
}
