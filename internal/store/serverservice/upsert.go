package serverservice

import (
	"context"
	"os"
	"reflect"

	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	r3diff "github.com/r3labs/diff/v3"
	"github.com/sanity-io/litter"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/codes"

	serverserviceapi "go.hollow.sh/serverservice/pkg/api/v1"
)

// upsert inserts/updates the asset data in the serverservice store
func (r *serverServiceClient) upsert(ctx context.Context, device *model.Asset) {
	// attach child span
	ctx, span := tracer.Start(ctx, "publish()")
	defer span.End()

	if device == nil {
		r.logger.Warn("nil device ignored")

		return
	}

	id, err := uuid.Parse(device.ID)
	if err != nil {
		r.logger.WithField("err", err).Warn("invalid device ID")

		return
	}

	// 1. retrieve server object, no action if the server doesn't exist
	server, hr, err := r.apiclient.Get(ctx, id)
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

		return
	}

	if server == nil {
		r.logger.WithFields(
			logrus.Fields{
				"id": id,
				"hr": hr,
			}).Warn("server service server query returned nil object")

		return
	}

	// create/update server bmc error attributes - for out of band data collection
	if r.config.AppKind == model.AppKindOutOfBand && len(device.Errors) > 0 {
		err = r.createUpdateServerBMCErrorAttributes(
			ctx,
			server.UUID,
			attributeByNamespace(model.ServerBMCErrorsAttributeNS, server.Attributes),
			device,
		)

		if err != nil {
			r.logger.WithFields(
				logrus.Fields{
					"id":  id,
					"err": err,
				}).Warn("error in server bmc error attributes update")
		}

		// count devices with errors
		metricInventorized.With(prometheus.Labels{"status": "failed"}).Add(1)

		return
	}

	// count devices with no errors
	metricInventorized.With(prometheus.Labels{"status": "success"}).Add(1)

	// create/update server serial, vendor, model attributes
	err = r.createUpdateServerAttributes(ctx, server, device)
	if err != nil {
		r.logger.WithFields(
			logrus.Fields{
				"id":  server.UUID.String(),
				"err": err,
			}).Warn("error in server attributes update")
	}

	// create update server metadata attributes
	err = r.createUpdateServerMetadataAttributes(ctx, server.UUID, device)
	if err != nil {
		r.logger.WithFields(
			logrus.Fields{
				"id":  server.UUID.String(),
				"err": err,
			}).Warn("error in server metadata attributes update")
	}

	// create update server component
	err = r.createUpdateServerComponents(ctx, server.UUID, device)
	if err != nil {
		r.logger.WithFields(
			logrus.Fields{
				"id":  server.UUID.String(),
				"err": err,
			}).Warn("error converting device object")
	}

	// Don't publish bios config if there's no data
	if len(device.BiosConfig) != 0 {
		err = r.createUpdateServerBIOSConfiguration(ctx, server.UUID, device.BiosConfig)
		if err != nil {
			r.logger.WithFields(
				logrus.Fields{
					"id":  server.UUID.String(),
					"err": err,
				}).Warn("error in server bios configuration versioned attribute update")
		}
	}
}

// createUpdateServerComponents compares the current object in serverService with the device data and creates/updates server component data.
//
// nolint:gocyclo // the method caries out all steps to have device data compared and registered, for now its accepted as cyclomatic.
func (r *serverServiceClient) createUpdateServerComponents(ctx context.Context, serverID uuid.UUID, device *model.Asset) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "createUpdateServerComponents()")
	defer span.End()

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
	currentInventory, _, err := s.apiclient.GetComponents(ctx, serverID, &serverserviceapi.PaginationParams{})
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
		return errors.Wrap(ErrRegisterChanges, err.Error())
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

		_, err = r.apiclient.CreateComponents(ctx, serverID, add)
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

		_, err = r.client.UpdateComponents(ctx, serverID, update)
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
