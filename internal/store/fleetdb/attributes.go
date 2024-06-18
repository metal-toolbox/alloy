package fleetdb

import (
	"context"
	"encoding/json"
	"os"
	"sort"

	"github.com/bmc-toolbox/common"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/helpers"
	"github.com/metal-toolbox/alloy/internal/model"
	fleetdbapi "github.com/metal-toolbox/fleetdb/pkg/api/v1"
	"github.com/pkg/errors"
	r3diff "github.com/r3labs/diff/v3"
	"golang.org/x/exp/slices"
)

const (
	uefiVariablesKey         = "uefi-variables"
	ssMetadataAttributeFound = "__ss_found"
)

// createUpdateServerAttributes creates/updates the server serial, vendor, model attributes
func (r *Store) createUpdateServerAttributes(ctx context.Context, server *fleetdbapi.Server, asset *model.Asset) error {
	// device vendor data
	deviceVendorData := r.deviceVendorData(asset)

	// marshal data from device
	deviceVendorDataBytes, err := json.Marshal(deviceVendorData)
	if err != nil {
		return err
	}

	deviceVendorAttributes := &fleetdbapi.Attributes{
		Namespace: serverVendorAttributeNS,
		Data:      deviceVendorDataBytes,
	}

	// identify current vendor data in the inventory
	inventoryAttrs := attributeByNamespace(serverVendorAttributeNS, server.Attributes)
	if inventoryAttrs == nil {
		// create if none exists
		_, err = r.CreateAttributes(ctx, server.UUID, *deviceVendorAttributes)
		return err
	}

	// unpack vendor data from inventory
	inventoryVendorData := map[string]string{}
	if err := json.Unmarshal(inventoryAttrs.Data, &inventoryVendorData); err != nil {
		// update vendor data since it seems to be invalid
		r.logger.Warn("server vendor attributes data invalid, updating..")

		_, err = r.UpdateAttributes(ctx, server.UUID, serverVendorAttributeNS, deviceVendorDataBytes)

		return err
	}

	update := vendorDataUpdate(deviceVendorData, inventoryVendorData)
	if len(update) > 0 {
		updateBytes, err := json.Marshal(update)
		if err != nil {
			return err
		}

		_, err = r.UpdateAttributes(ctx, server.UUID, serverVendorAttributeNS, updateBytes)

		return err
	}

	return r.publishUEFIVars(ctx, server.UUID, asset)
}

func (r *Store) publishUEFIVars(ctx context.Context, serverID uuid.UUID, asset *model.Asset) error {
	if asset.Inventory == nil || asset.Inventory.Metadata == nil {
		return nil
	}

	vars, exists := asset.Inventory.Metadata[uefiVariablesKey]
	if !exists {
		return nil
	}

	va := fleetdbapi.VersionedAttributes{
		// waiting on https://github.com/metal-toolbox/rivets/pull/28
		// Namespace: rs.UEFIVarsNS,
		Namespace: "sh.hollow.alloy.uefi_vars",
		Data:      []byte(vars),
	}

	_, err := r.CreateVersionedAttributes(ctx, serverID, va)

	return err
}

// initializes a map with the device vendor data attributes
func (r *Store) deviceVendorData(asset *model.Asset) map[string]string {
	// initialize map
	m := map[string]string{
		serverSerialAttributeKey: "unknown",
		serverVendorAttributeKey: "unknown",
		serverModelAttributeKey:  "unknown",
	}

	if asset.Inventory != nil {
		if asset.Inventory.Serial != "" {
			m[serverSerialAttributeKey] = asset.Inventory.Serial
		}

		invModelFormatted := common.FormatProductName(asset.Inventory.Model)
		if invModelFormatted != "" {
			m[serverModelAttributeKey] = invModelFormatted
		}

		if asset.Inventory.Vendor != "" {
			m[serverVendorAttributeKey] = asset.Inventory.Vendor
		}
	}

	return m
}

// returns a map with device vendor attributes when an update is required
//
// nolint:gocyclo // data verify is cyclomatic
func vendorDataUpdate(newData, currentData map[string]string) map[string]string {
	const unknown = "unknown"
	var changes bool

	if currentData == nil {
		return newData
	}

	newModelNumber := common.FormatProductName(newData[serverModelAttributeKey])
	currentModelNumber := currentData[serverModelAttributeKey]

	changeRequired := func(key string) bool {
		newModelDefined := newModelNumber != unknown && newModelNumber != ""
		currentModelUndefined := currentModelNumber == unknown || currentModelNumber == ""

		// model number to be updated
		if key == serverModelAttributeKey {
			if currentModelUndefined && newModelDefined {
				return true
			}

			if newModelDefined && currentModelNumber != newModelNumber {
				newData[serverModelAttributeKey] = newModelNumber
				return true
			}

			return false
		}

		// rest of the keys are updated when the new data is defined and current is not.
		newKeyDefined := newData[key] != unknown && newData[key] != ""
		currentKeyUndefined := currentData[key] == "" || currentData[key] == unknown
		return newKeyDefined && currentKeyUndefined
	}

	setValue := func(key string, newData, currentData map[string]string) {
		// value is empty or explicit check for model number
		if changeRequired(key) {
			changes = true
			currentData[key] = newData[key]
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

// mustFilterAssetMetadata processes the asset inventory metadata to filter out fields we'll turn into versioned attributes (e.g. UEFIVariables)
func mustFilterAssetMetadata(inventory map[string]string) json.RawMessage {
	excludedKeys := map[string]struct{}{
		uefiVariablesKey: {},
	}

	filtered := make(map[string]string)

	for k, v := range inventory {
		if _, ok := excludedKeys[k]; ok {
			continue
		}
		filtered[k] = v
	}

	byt, err := json.Marshal(filtered)
	if err != nil {
		panic("serializing metadata string map")
	}

	return byt
}

// createUpdateServerMetadataAttributes creates/updates metadata attributes of a server
func (r *Store) createUpdateServerMetadataAttributes(ctx context.Context, serverID uuid.UUID, asset *model.Asset) error {
	// no metadata reported in inventory from device
	if asset.Inventory == nil || len(asset.Inventory.Metadata) == 0 {
		// XXX: should delete the metadata on the server-service record!
		return nil
	}

	// marshal metadata from device
	metadata := mustFilterAssetMetadata(asset.Inventory.Metadata)

	attribute := fleetdbapi.Attributes{
		Namespace: serverMetadataAttributeNS,
		Data:      metadata,
	}

	// XXX: This would be much easier if fleetdb/fleetdb supported upsert
	// current asset metadata has no attributes set and no metadata attribute, create one
	if _, ok := asset.Metadata[ssMetadataAttributeFound]; !ok {
		r.logger.WithField("server.id", serverID.String()).Debug("creating metadata attributes")
		_, err := r.CreateAttributes(ctx, serverID, attribute)
		return err
	}

	r.logger.WithField("server.id", serverID.String()).Debug("updating metadata attributes")
	// update vendor, model attributes
	_, err := r.UpdateAttributes(ctx, serverID, serverMetadataAttributeNS, metadata)

	return err
}

func (r *Store) createUpdateServerBIOSConfiguration(ctx context.Context, serverID uuid.UUID, biosConfig map[string]string) error {
	// marshal metadata from device
	bc, err := json.Marshal(biosConfig)
	if err != nil {
		return err
	}

	va := fleetdbapi.VersionedAttributes{
		Namespace: serverBIOSConfigNS(r.appKind),
		Data:      bc,
	}

	_, err = r.CreateVersionedAttributes(ctx, serverID, va)

	return err
}

// nolint:gocyclo // (joel) theres a bunch of validation going on here, I'll split the method out if theres more to come.
func (r *Store) createUpdateServerBMCErrorAttributes(ctx context.Context, serverID uuid.UUID, current *fleetdbapi.Attributes, asset *model.Asset) error {
	// 1. no errors reported, none currently present
	if len(asset.Errors) == 0 {
		// server has no bmc errors registered
		if current == nil || len(current.Data) == 0 {
			return nil
		}

		// server has bmc errors registered, update the attributes to purge existing errors
		_, err := r.UpdateAttributes(ctx, serverID, serverBMCErrorsAttributeNS, []byte(`{}`))

		return err
	}

	// marshal new data
	newData, err := json.Marshal(asset.Errors)
	if err != nil {
		return err
	}

	attribute := fleetdbapi.Attributes{
		Namespace: serverBMCErrorsAttributeNS,
		Data:      newData,
	}

	// 2. current data has no BMC error attributes object, create
	if current == nil || len(current.Data) == 0 {
		_, err = r.CreateAttributes(ctx, serverID, attribute)
		return err
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
	_, err = r.UpdateAttributes(ctx, serverID, serverBMCErrorsAttributeNS, newData)

	return err
}

func diffComponentObjectsAttributes(currentObj, changeObj *fleetdbapi.ServerComponent) ([]fleetdbapi.Attributes, []fleetdbapi.VersionedAttributes, error) {
	var attributes []fleetdbapi.Attributes

	var versionedAttributes []fleetdbapi.VersionedAttributes

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
// returning the versioned attribute to be registered with fleetdbapi.
//
// In the case that no changes are to be registered, a nil object is returned.
func diffVersionedAttributes(currentObjs, newObjs []fleetdbapi.VersionedAttributes) (*fleetdbapi.VersionedAttributes, error) {
	// no newObjects
	if len(newObjs) == 0 {
		return nil, nil
	}

	// no versioned attributes in current
	if len(newObjs) > 0 && len(currentObjs) == 0 {
		return &newObjs[0], nil
	}

	// identify current latest versioned attribute (sorted by created_at)
	var currentObj fleetdbapi.VersionedAttributes

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
func (r *Store) filterByAttributeNamespace(components []*fleetdbapi.ServerComponent) {
	for cIdx, component := range components {
		attributes := []fleetdbapi.Attributes{}
		versionedAttributes := []fleetdbapi.VersionedAttributes{}

		for idx, attribute := range component.Attributes {
			if attribute.Namespace == r.attributeNS {
				attributes = append(attributes, component.Attributes[idx])
			}
		}

		components[cIdx].Attributes = attributes

		for idx, versionedAttribute := range component.VersionedAttributes {
			if slices.Contains([]string{r.firmwareVersionedAttributeNS, r.statusVersionedAttributeNS}, versionedAttribute.Namespace) {
				versionedAttributes = append(versionedAttributes, component.VersionedAttributes[idx])
			}
		}

		components[cIdx].VersionedAttributes = versionedAttributes
	}
}

// attributeByNamespace returns the attribute in the slice that matches the namespace
func attributeByNamespace(ns string, attributes []fleetdbapi.Attributes) *fleetdbapi.Attributes {
	for _, attribute := range attributes {
		if attribute.Namespace == ns {
			return &attribute
		}
	}

	return nil
}

// serverAttributes parses the server service attribute data
// and returns a map containing the bmc address, server serial, vendor, model attributes
// and optionally the BMC address and attributes.
func serverAttributes(attributes []fleetdbapi.Attributes, wantBmcCredentials bool) (map[string]string, error) {
	// returned server attributes map
	sAttributes := map[string]string{}

	// bmc IP Address attribute data is unpacked into this map
	bmcData := map[string]string{}

	// server vendor, model attribute data is unpacked into this map
	serverVendorData := map[string]string{}

	for _, attribute := range attributes {
		// bmc address attribute
		if wantBmcCredentials && (attribute.Namespace == bmcAttributeNamespace) {
			if err := json.Unmarshal(attribute.Data, &bmcData); err != nil {
				return nil, errors.Wrap(ErrFleetDBAPIObject, "bmc address attribute: "+err.Error())
			}
		}

		// server vendor, model attributes
		if attribute.Namespace == serverVendorAttributeNS {
			if err := json.Unmarshal(attribute.Data, &serverVendorData); err != nil {
				return nil, errors.Wrap(ErrFleetDBAPIObject, "server vendor attribute: "+err.Error())
			}
		}
	}

	if wantBmcCredentials {
		if len(bmcData) == 0 {
			return nil, errors.New("expected server attributes with BMC address, got none")
		}

		// set bmc address attribute
		sAttributes[bmcIPAddressAttributeKey] = bmcData[bmcIPAddressAttributeKey]
		if sAttributes[bmcIPAddressAttributeKey] == "" {
			return nil, errors.New("expected BMC address attribute empty")
		}
	}

	// set server vendor, model attributes in the returned map
	serverAttributes := []string{
		serverSerialAttributeKey,
		serverModelAttributeKey,
		serverVendorAttributeKey,
	}

	for _, key := range serverAttributes {
		sAttributes[key] = serverVendorData[key]
		if sAttributes[key] == "" {
			sAttributes[key] = "unknown"
		}
	}

	return sAttributes, nil
}

func validateRequiredAttributes(server *fleetdbapi.Server, credential *fleetdbapi.ServerCredential, expectCredentials bool) error {
	if server == nil {
		return errors.New("server object nil")
	}

	if expectCredentials && credential == nil {
		return errors.New("server credential object nil")
	}

	if len(server.Attributes) == 0 {
		return errors.New("server attributes slice empty")
	}

	if expectCredentials && credential.Username == "" {
		return errors.New("BMC username field empty")
	}

	if expectCredentials && credential.Password == "" {
		return errors.New("BMC password field empty")
	}

	return nil
}

// serverMetadataAttributes parses the server service server metdata attribute data
// and returns a map containing the server metadata
func serverMetadataAttributes(attributes []fleetdbapi.Attributes) (map[string]string, error) {
	metadata := map[string]string{}

	for _, attribute := range attributes {
		// bmc address attribute
		if attribute.Namespace == serverMetadataAttributeNS {
			if err := json.Unmarshal(attribute.Data, &metadata); err != nil {
				return nil, errors.Wrap(ErrFleetDBAPIObject, "server metadata attribute: "+err.Error())
			}
			// XXX: it is possible for there to be a metadata attribute with an empty Data field
			// Add an entry here so that when we test for doing a create vs. an update we make the
			// right decision.
			metadata[ssMetadataAttributeFound] = "true"
		}
	}

	return metadata, nil
}
