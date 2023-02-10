package publish

import (
	"context"
	"encoding/json"
	"os"
	"sort"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/helpers"
	"github.com/metal-toolbox/alloy/internal/model"
	r3diff "github.com/r3labs/diff/v3"
	serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

// createUpdateServerAttributes creates/updates the server serial, vendor, model attributes
func (h *serverServicePublisher) createUpdateServerAttributes(ctx context.Context, server *serverservice.Server, asset *model.Asset) error {
	// device vendor data
	deviceVendorData := h.deviceVendorData(asset)

	// marshal data from device
	deviceVendorDataBytes, err := json.Marshal(deviceVendorData)
	if err != nil {
		return err
	}

	deviceVendorAttributes := &serverservice.Attributes{
		Namespace: model.ServerVendorAttributeNS,
		Data:      deviceVendorDataBytes,
	}

	// identify current vendor data in the inventory
	inventoryAttrs := attributeByNamespace(model.ServerVendorAttributeNS, server.Attributes)
	if inventoryAttrs == nil {
		// create if none exists
		_, err = h.client.CreateAttributes(ctx, server.UUID, *deviceVendorAttributes)
		return err
	}

	// unpack vendor data from inventory
	inventoryVendorData := map[string]string{}
	if err := json.Unmarshal(inventoryAttrs.Data, &inventoryVendorData); err != nil {
		// update vendor data since it seems to be invalid
		h.logger.Warn("server vendor attributes data invalid, updating..")

		_, err = h.client.UpdateAttributes(ctx, server.UUID, model.ServerVendorAttributeNS, deviceVendorDataBytes)

		return err
	}

	update := vendorDataUpdate(deviceVendorData, inventoryVendorData)
	if len(update) > 0 {
		updateBytes, err := json.Marshal(update)
		if err != nil {
			return err
		}

		_, err = h.client.UpdateAttributes(ctx, server.UUID, model.ServerVendorAttributeNS, updateBytes)

		return err
	}

	return nil
}

// initializes a map with the device vendor data attributes
func (h *serverServicePublisher) deviceVendorData(asset *model.Asset) map[string]string {
	// initialize map
	m := map[string]string{
		model.ServerSerialAttributeKey: "unknown",
		model.ServerVendorAttributeKey: "unknown",
		model.ServerModelAttributeKey:  "unknown",
	}

	if asset.Inventory.Serial != "" {
		m[model.ServerSerialAttributeKey] = asset.Inventory.Serial
	}

	if asset.Inventory.Model != "" {
		m[model.ServerModelAttributeKey] = asset.Inventory.Model
	}

	if asset.Inventory.Vendor != "" {
		m[model.ServerVendorAttributeKey] = asset.Inventory.Vendor
	}

	return m
}

// returns a map with device vendor attributes when an update is required
func vendorDataUpdate(newData, currentData map[string]string) map[string]string {
	if currentData == nil {
		return newData
	}

	var changes bool

	setValue := func(key string, newData, currentData map[string]string) {
		const unknown = "unknown"

		if currentData[key] == "" || currentData[key] == unknown {
			if newData[key] != unknown {
				changes = true
				currentData[key] = newData[key]
			}
		}
	}

	for k := range newData {
		setValue(k, newData, currentData)
	}

	if !changes {
		return nil
	}

	return currentData
}

// createUpdateServerMetadataAttributes creates/updates metadata attributes of a server
func (h *serverServicePublisher) createUpdateServerMetadataAttributes(ctx context.Context, serverID uuid.UUID, asset *model.Asset) error {
	// no metadata reported in inventory from device
	if len(asset.Inventory.Metadata) == 0 {
		return nil
	}

	// marshal metadata from device
	metadata, err := json.Marshal(asset.Inventory.Metadata)
	if err != nil {
		return err
	}

	attribute := serverservice.Attributes{
		Namespace: model.ServerMetadataAttributeNS,
		Data:      metadata,
	}

	// current asset metadata has no attributes set, create
	if len(asset.Metadata) == 0 {
		_, err = h.client.CreateAttributes(ctx, serverID, attribute)
		return err
	}

	// update when metadata differs
	if helpers.MapsAreEqual(asset.Metadata, asset.Inventory.Metadata) {
		return nil
	}

	// update vendor, model attributes
	_, err = h.client.UpdateAttributes(ctx, serverID, model.ServerMetadataAttributeNS, metadata)

	return err
}

func (h *serverServicePublisher) createUpdateServerBIOSConfiguration(ctx context.Context, serverID uuid.UUID, biosConfig map[string]string) error {
	// marshal metadata from device
	bc, err := json.Marshal(biosConfig)
	if err != nil {
		return err
	}

	va := serverservice.VersionedAttributes{
		Namespace: model.ServerBIOSConfigNS(h.config.AppKind),
		Data:      bc,
	}

	_, err = h.client.CreateVersionedAttributes(ctx, serverID, va)
	if err != nil {
		return err
	}

	return nil
}

// createUpdateServerMetadataAttributes creates/updates metadata attributes of a server
// nolint:gocyclo // (joel) theres a bunch of validation going on here, I'll split the method out if theres more to come.
func (h *serverServicePublisher) createUpdateServerBMCErrorAttributes(ctx context.Context, serverID uuid.UUID, current *serverservice.Attributes, asset *model.Asset) error {
	// 1. no errors reported, none currently present
	if len(asset.Errors) == 0 {
		// server has no bmc errors registered
		if current == nil || len(current.Data) == 0 {
			return nil
		}

		// server has bmc errors registered, update the attributes to purge existing errors
		_, err := h.client.UpdateAttributes(ctx, serverID, model.ServerBMCErrorsAttributeNS, []byte(`{}`))
		if err != nil {
			return err
		}

		// no errors, nothing to update
		return nil
	}

	// marshal new data
	newData, err := json.Marshal(asset.Errors)
	if err != nil {
		return err
	}

	attribute := serverservice.Attributes{
		Namespace: model.ServerBMCErrorsAttributeNS,
		Data:      newData,
	}

	// 2. current data has no BMC error attributes object, create
	if current == nil || len(current.Data) == 0 {
		_, err = h.client.CreateAttributes(ctx, serverID, attribute)
		if err != nil {
			return err
		}

		return nil
	}

	// 3. current asset has some error attributes set, compare and update
	currentData := map[string]string{}

	err = json.Unmarshal(current.Data, &currentData)
	if err != nil {
		return err
	}

	// data is equal
	if helpers.MapsAreEqual(currentData, asset.Errors) {
		return nil
	}

	// update vendor, model attributes
	_, err = h.client.UpdateAttributes(ctx, serverID, model.ServerBMCErrorsAttributeNS, newData)
	if err != nil {
		return err
	}

	return nil
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

	// For debugging dump differ data
	if os.Getenv(model.EnvVarDumpDiffers) == "true" {
		objChangesf := currentObj.ServerUUID.String() + ".attributes.diff"

		// write cmp diff for readability
		helpers.WriteDebugFile(objChangesf, cmp.Diff(currentObj.Attributes, changeObj.Attributes))
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
		// For debugging dump differ data
		if os.Getenv(model.EnvVarDumpDiffers) == "true" {
			objChangesf := currentObj.Namespace + ".versioned_attributes.diff"

			// write cmp diff for readability
			helpers.WriteDebugFile(objChangesf, cmp.Diff(currentObj, newObjs[0]))
		}

		return &newObjs[0], err
	}

	return nil, nil
}

// filterByAttributeNamespace removes any components attributes, versioned that is
// not related to this instance (inband/out-of-band) of Alloy.
//
// This is to ensure that this instance of Alloy is only working with the data that
// is part of the defined attributes, versioned attributes namespaces
func (h *serverServicePublisher) filterByAttributeNamespace(components []*serverservice.ServerComponent) {
	for cIdx, component := range components {
		attributes := []serverservice.Attributes{}
		versionedAttributes := []serverservice.VersionedAttributes{}

		for idx, attribute := range component.Attributes {
			if attribute.Namespace == h.attributeNS {
				attributes = append(attributes, component.Attributes[idx])
			}
		}

		components[cIdx].Attributes = attributes

		for idx, versionedAttribute := range component.VersionedAttributes {
			if versionedAttribute.Namespace == h.versionedAttributeNS {
				versionedAttributes = append(versionedAttributes, component.VersionedAttributes[idx])
			}
		}

		components[cIdx].VersionedAttributes = versionedAttributes
	}
}

// attributeByNamespace returns the attribute in the slice that matches the namespace
func attributeByNamespace(ns string, attributes []serverservice.Attributes) *serverservice.Attributes {
	for _, attribute := range attributes {
		if attribute.Namespace == ns {
			return &attribute
		}
	}

	return nil
}
