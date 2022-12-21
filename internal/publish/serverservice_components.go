package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/bmc-toolbox/common"
	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/codes"

	serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

// devel notes
// -----------
// - Components are stored in serverservice with a composite unique constraint on the component serial, component_type and server_id.
// - When making changes to the way the serial is generated here (if one does not exist)
//   keep in mind that this will affect existing data in serverservice, that is the components with newer serials
//   will end up being new components added.

// componentBySlugSerial returns a pointer to a component that matches the given slug, serial attributes
func componentBySlugSerial(slug, serial string, components []*serverservice.ServerComponent) *serverservice.ServerComponent {
	for _, c := range components {
		if strings.EqualFold(slug, c.ComponentTypeSlug) && strings.EqualFold(serial, c.Serial) {
			return c
		}
	}

	return nil
}

func (h *serverServicePublisher) cacheServerComponentTypes(ctx context.Context) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "cacheServerComponentTypes()")
	defer span.End()

	serverComponentTypes, _, err := h.client.ListServerComponentTypes(ctx, nil)
	if err != nil {
		// count error
		metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()

		// set span status
		span.SetStatus(codes.Error, "ListServerComponentTypes() failed")

		return err
	}

	for _, ct := range serverComponentTypes {
		h.slugs[ct.Slug] = ct
	}

	return nil
}

// componentPtrSlice returns a slice of pointers to serverservice.ServerComponent.
//
// The hollow client methods require component slice objects to be passed as values
// these tend to be large objects.
//
// This helper method is to reduce the amount of copying of component objects (~240 bytes each) when passed around between methods and range loops,
// while it seems like a minor optimization, it also keeps the linter happy.
func componentPtrSlice(components serverservice.ServerComponentSlice) []*serverservice.ServerComponent {
	s := make([]*serverservice.ServerComponent, 0, len(components))

	// nolint:gocritic // the copying has to be done somewhere
	for _, c := range components {
		c := c
		s = append(s, &c)
	}

	return s
}

// toComponentSlice converts an model.AssetDevice object to the server service component slice object
func (h *serverServicePublisher) toComponentSlice(serverID uuid.UUID, device *model.Asset) ([]*serverservice.ServerComponent, error) {
	componentsTmp := []*serverservice.ServerComponent{}
	componentsTmp = append(componentsTmp,
		h.bios(device.Inventory.BIOS),
		h.bmc(device.Inventory.BMC),
		h.mainboard(device.Inventory.Mainboard),
	)

	componentsTmp = append(componentsTmp, h.dimms(device.Inventory.Memory)...)
	componentsTmp = append(componentsTmp, h.nics(device.Inventory.NICs)...)
	componentsTmp = append(componentsTmp, h.drives(device.Inventory.Drives)...)
	componentsTmp = append(componentsTmp, h.psus(device.Inventory.PSUs)...)
	componentsTmp = append(componentsTmp, h.cpus(device.Inventory.CPUs)...)
	componentsTmp = append(componentsTmp, h.tpms(device.Inventory.TPMs)...)
	componentsTmp = append(componentsTmp, h.cplds(device.Inventory.CPLDs)...)
	componentsTmp = append(componentsTmp, h.gpus(device.Inventory.GPUs)...)
	componentsTmp = append(componentsTmp, h.storageControllers(device.Inventory.StorageControllers)...)
	componentsTmp = append(componentsTmp, h.enclosures(device.Inventory.Enclosures)...)

	components := []*serverservice.ServerComponent{}

	for _, component := range componentsTmp {
		if component == nil {
			continue
		}

		component.ServerUUID = serverID
		components = append(components, component)
	}

	return components, nil
}

func (h *serverServicePublisher) newComponent(slug, cvendor, cmodel, cserial, cproduct string) (*serverservice.ServerComponent, error) {
	// lower case slug to changeObj how its stored in server service
	slug = strings.ToLower(slug)

	// component slug lookup map is expected
	if len(h.slugs) == 0 {
		return nil, errors.Wrap(ErrSlugs, "component slugs lookup map empty")
	}

	// component slug is part of the lookup map
	_, exists := h.slugs[slug]
	if !exists {
		return nil, errors.Wrap(ErrSlugs, "unknown component slug: "+slug)
	}

	// use the product name when model number is empty
	if strings.TrimSpace(cmodel) == "" && strings.TrimSpace(cproduct) != "" {
		cmodel = cproduct
	}

	return &serverservice.ServerComponent{
		Name:              h.slugs[slug].Name,
		Vendor:            common.FormatVendorName(cvendor),
		Model:             cmodel,
		Serial:            cserial,
		ComponentTypeID:   h.slugs[slug].ID,
		ComponentTypeName: h.slugs[slug].Name,
		ComponentTypeSlug: slug,
	}, nil
}

func (h *serverServicePublisher) gpus(gpus []*common.GPU) []*serverservice.ServerComponent {
	if gpus == nil {
		return nil
	}

	components := make([]*serverservice.ServerComponent, 0, len(gpus))

	for idx, c := range gpus {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := h.newComponent(common.SlugGPU, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			h.logger.Error(err)

			return nil
		}

		h.setAttributes(
			sc,
			&attributes{
				Description:  c.Description,
				ProductName:  c.ProductName,
				Metadata:     c.Metadata,
				Capabilities: c.Capabilities,
			},
		)

		h.setVersionedAttributes(
			sc,
			&versionedAttributes{
				Firmware: c.Firmware,
				Status:   c.Status,
			},
		)

		components = append(components, sc)
	}

	return components
}

func (h *serverServicePublisher) cplds(cplds []*common.CPLD) []*serverservice.ServerComponent {
	if cplds == nil {
		return nil
	}

	components := make([]*serverservice.ServerComponent, 0, len(cplds))

	for idx, c := range cplds {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := h.newComponent(common.SlugCPLD, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			h.logger.Error(err)

			return nil
		}

		h.setAttributes(
			sc,
			&attributes{
				Description:  c.Description,
				ProductName:  c.ProductName,
				Metadata:     c.Metadata,
				Capabilities: c.Capabilities,
			},
		)

		h.setVersionedAttributes(
			sc,
			&versionedAttributes{
				Firmware: c.Firmware,
				Status:   c.Status,
			},
		)

		components = append(components, sc)
	}

	return components
}

func (h *serverServicePublisher) tpms(tpms []*common.TPM) []*serverservice.ServerComponent {
	if tpms == nil {
		return nil
	}

	components := make([]*serverservice.ServerComponent, 0, len(tpms))

	for idx, c := range tpms {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := h.newComponent(common.SlugTPM, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			h.logger.Error(err)

			return nil
		}

		h.setAttributes(
			sc,
			&attributes{
				Description:   c.Description,
				ProductName:   c.ProductName,
				Metadata:      c.Metadata,
				Capabilities:  c.Capabilities,
				InterfaceType: c.InterfaceType,
			},
		)

		h.setVersionedAttributes(
			sc,
			&versionedAttributes{
				Firmware: c.Firmware,
				Status:   c.Status,
			},
		)

		components = append(components, sc)
	}

	return components
}

func (h *serverServicePublisher) cpus(cpus []*common.CPU) []*serverservice.ServerComponent {
	if cpus == nil {
		return nil
	}

	components := make([]*serverservice.ServerComponent, 0, len(cpus))

	for idx, c := range cpus {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := h.newComponent(common.SlugCPU, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			h.logger.Error(err)

			return nil
		}

		h.setAttributes(
			sc,
			&attributes{
				ID:           c.ID,
				Description:  c.Description,
				ProductName:  c.ProductName,
				Metadata:     c.Metadata,
				Slot:         c.Slot,
				Architecture: c.Architecture,
				ClockSpeedHz: c.ClockSpeedHz,
				Cores:        c.Cores,
				Threads:      c.Threads,
				Capabilities: c.Capabilities,
			},
		)

		h.setVersionedAttributes(
			sc,
			&versionedAttributes{
				Firmware: c.Firmware,
				Status:   c.Status,
			},
		)

		components = append(components, sc)
	}

	return components
}

func (h *serverServicePublisher) storageControllers(controllers []*common.StorageController) []*serverservice.ServerComponent {
	if controllers == nil {
		return nil
	}

	components := make([]*serverservice.ServerComponent, 0, len(controllers))

	serials := map[string]bool{}

	for idx, c := range controllers {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		// Storage controllers can show up with pci IDs are their serial number
		// set a unique serial on those components
		_, exists := serials[c.Serial]
		if exists {
			c.Serial = c.Serial + "-alloy-" + strconv.Itoa(idx)
		} else {
			serials[c.Serial] = true
		}

		sc, err := h.newComponent(common.SlugStorageController, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			h.logger.Error(err)

			return nil
		}

		h.setAttributes(
			sc,
			&attributes{
				ID:                           c.ID,
				Description:                  c.Description,
				ProductName:                  c.ProductName,
				Oem:                          c.Oem,
				SupportedControllerProtocols: c.SupportedControllerProtocols,
				SupportedDeviceProtocols:     c.SupportedDeviceProtocols,
				SupportedRAIDTypes:           c.SupportedRAIDTypes,
				PhysicalID:                   c.PhysicalID,
				BusInfo:                      c.BusInfo,
				SpeedGbps:                    c.SpeedGbps,
				Metadata:                     c.Metadata,
				Capabilities:                 c.Capabilities,
			},
		)

		h.setVersionedAttributes(
			sc,
			&versionedAttributes{
				Firmware: c.Firmware,
				Status:   c.Status,
			},
		)

		// some controller show up with model numbers in the description field.
		if sc.Model == "" && c.Description != "" {
			sc.Model = c.Description
		}

		components = append(components, sc)
	}

	return components
}

func (h *serverServicePublisher) psus(psus []*common.PSU) []*serverservice.ServerComponent {
	if psus == nil {
		return nil
	}

	components := make([]*serverservice.ServerComponent, 0, len(psus))

	for idx, c := range psus {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := h.newComponent(common.SlugPSU, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			h.logger.Error(err)

			return nil
		}

		h.setAttributes(
			sc,
			&attributes{
				ID:                 c.ID,
				Description:        c.Description,
				ProductName:        c.ProductName,
				PowerCapacityWatts: c.PowerCapacityWatts,
				Oem:                c.Oem,
				Metadata:           c.Metadata,
				Capabilities:       c.Capabilities,
			},
		)

		h.setVersionedAttributes(
			sc,
			&versionedAttributes{
				Firmware: c.Firmware,
				Status:   c.Status,
			},
		)

		components = append(components, sc)
	}

	return components
}

func (h *serverServicePublisher) drives(drives []*common.Drive) []*serverservice.ServerComponent {
	if drives == nil {
		return nil
	}

	components := make([]*serverservice.ServerComponent, 0, len(drives))

	for idx, c := range drives {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := h.newComponent(common.SlugDrive, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			h.logger.Error(err)

			return nil
		}

		h.setAttributes(
			sc,
			&attributes{
				Description:         c.Description,
				ProductName:         c.ProductName,
				Oem:                 c.Oem,
				Metadata:            c.Metadata,
				BusInfo:             c.BusInfo,
				OemID:               c.OemID,
				StorageController:   c.StorageController,
				Protocol:            c.Protocol,
				SmartErrors:         c.SmartErrors,
				DriveType:           c.Type,
				WWN:                 c.WWN,
				CapacityBytes:       c.CapacityBytes,
				BlockSizeBytes:      c.BlockSizeBytes,
				CapableSpeedGbps:    c.CapableSpeedGbps,
				NegotiatedSpeedGbps: c.NegotiatedSpeedGbps,
				Capabilities:        c.Capabilities,
			},
		)

		h.setVersionedAttributes(
			sc,
			&versionedAttributes{
				Firmware: c.Firmware,
				Status:   c.Status,
			},
		)

		// some drives show up with model numbers in the description field.
		if sc.Model == "" && c.Description != "" {
			sc.Model = c.Description
		}

		components = append(components, sc)
	}

	return components
}

func (h *serverServicePublisher) nics(nics []*common.NIC) []*serverservice.ServerComponent {
	if nics == nil {
		return nil
	}

	components := make([]*serverservice.ServerComponent, 0, len(nics))

	for idx, c := range nics {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := h.newComponent(common.SlugNIC, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			h.logger.Error(err)

			return nil
		}

		h.setAttributes(
			sc,
			&attributes{
				Description:  c.Description,
				ProductName:  c.ProductName,
				Oem:          c.Oem,
				Metadata:     c.Metadata,
				PhysicalID:   c.PhysicalID,
				BusInfo:      c.BusInfo,
				MacAddress:   c.MacAddress,
				SpeedBits:    c.SpeedBits,
				Capabilities: c.Capabilities,
			},
		)

		h.setVersionedAttributes(
			sc,
			&versionedAttributes{
				Firmware: c.Firmware,
				Status:   c.Status,
			},
		)

		components = append(components, sc)
	}

	return components
}

func (h *serverServicePublisher) dimms(dimms []*common.Memory) []*serverservice.ServerComponent {
	if dimms == nil {
		return nil
	}

	components := make([]*serverservice.ServerComponent, 0, len(dimms))

	for idx, c := range dimms {
		// skip empty dimm slots
		if c.Vendor == "" && c.ProductName == "" && c.SizeBytes == 0 && c.ClockSpeedHz == 0 {
			continue
		}

		// set incrementing serial when one isn't found
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		// trim redundant prefix
		c.Slot = strings.TrimPrefix(c.Slot, "DIMM.Socket.")

		sc, err := h.newComponent(common.SlugPhysicalMem, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			h.logger.Error(err)

			return nil
		}

		h.setAttributes(
			sc,
			&attributes{
				Description:  c.Description,
				ProductName:  c.ProductName,
				Oem:          c.Oem,
				Slot:         c.Slot,
				ClockSpeedHz: c.ClockSpeedHz,
				FormFactor:   c.FormFactor,
				PartNumber:   c.PartNumber,
				Metadata:     c.Metadata,
				SizeBytes:    c.SizeBytes,
				Capabilities: c.Capabilities,
			},
		)

		h.setVersionedAttributes(
			sc,
			&versionedAttributes{
				Firmware: c.Firmware,
				Status:   c.Status,
			},
		)

		components = append(components, sc)
	}

	return components
}

func (h *serverServicePublisher) mainboard(c *common.Mainboard) *serverservice.ServerComponent {
	if c == nil {
		return nil
	}

	if strings.TrimSpace(c.Serial) == "" {
		c.Serial = "0"
	}

	sc, err := h.newComponent(common.SlugMainboard, c.Vendor, c.Model, c.Serial, c.ProductName)
	if err != nil {
		h.logger.Error(err)

		return nil
	}

	h.setAttributes(
		sc,
		&attributes{
			Description:  c.Description,
			ProductName:  c.ProductName,
			Oem:          c.Oem,
			PhysicalID:   c.PhysicalID,
			Metadata:     c.Metadata,
			Capabilities: c.Capabilities,
		},
	)

	h.setVersionedAttributes(
		sc,
		&versionedAttributes{
			Firmware: c.Firmware,
			Status:   c.Status,
		},
	)

	return sc
}

func (h *serverServicePublisher) enclosures(enclosures []*common.Enclosure) []*serverservice.ServerComponent {
	if enclosures == nil {
		return nil
	}

	components := make([]*serverservice.ServerComponent, 0, len(enclosures))

	for idx, c := range enclosures {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := h.newComponent(common.SlugEnclosure, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			h.logger.Error(err)

			return nil
		}

		h.setAttributes(
			sc,
			&attributes{
				ID:           c.ID,
				Description:  c.Description,
				ProductName:  c.ProductName,
				Oem:          c.Oem,
				Metadata:     c.Metadata,
				ChassisType:  c.ChassisType,
				Capabilities: c.Capabilities,
			},
		)

		h.setVersionedAttributes(
			sc,
			&versionedAttributes{
				Firmware: c.Firmware,
				Status:   c.Status,
			},
		)

		components = append(components, sc)
	}

	return components
}

func (h *serverServicePublisher) bmc(c *common.BMC) *serverservice.ServerComponent {
	if c == nil {
		return nil
	}

	if strings.TrimSpace(c.Serial) == "" {
		c.Serial = "0"
	}

	sc, err := h.newComponent(common.SlugBMC, c.Vendor, c.Model, c.Serial, c.ProductName)
	if err != nil {
		h.logger.Error(err)

		return nil
	}

	h.setAttributes(
		sc,
		&attributes{
			Description:  c.Description,
			ProductName:  c.ProductName,
			Oem:          c.Oem,
			Metadata:     c.Metadata,
			Capabilities: c.Capabilities,
		},
	)

	h.setVersionedAttributes(
		sc,
		&versionedAttributes{
			Firmware: c.Firmware,
			Status:   c.Status,
		},
	)

	return sc
}

func (h *serverServicePublisher) bios(c *common.BIOS) *serverservice.ServerComponent {
	if c == nil {
		return nil
	}

	if strings.TrimSpace(c.Serial) == "" {
		c.Serial = "0"
	}

	sc, err := h.newComponent(common.SlugBIOS, c.Vendor, c.Model, c.Serial, c.ProductName)
	if err != nil {
		h.logger.Error(err)

		return nil
	}

	h.setAttributes(
		sc,
		&attributes{
			Description:   c.Description,
			ProductName:   c.ProductName,
			SizeBytes:     c.SizeBytes,
			CapacityBytes: c.CapacityBytes,
			Oem:           c.Oem,
			Metadata:      c.Metadata,
			Capabilities:  c.Capabilities,
		},
	)

	h.setVersionedAttributes(
		sc,
		&versionedAttributes{
			Firmware: c.Firmware,
			Status:   c.Status,
		},
	)

	return sc
}

// attributes are generic component attributes
type attributes struct {
	Capabilities                 []*common.Capability `json:"capabilities,omitempty"`
	Metadata                     map[string]string    `json:"metadata,omitempty"`
	ID                           string               `json:"id,omitempty"`
	ChassisType                  string               `json:"chassis_type,omitempty"`
	Description                  string               `json:"description,omitempty"`
	ProductName                  string               `json:"product_name,omitempty"`
	InterfaceType                string               `json:"interface_type,omitempty"`
	Slot                         string               `json:"slot,omitempty"`
	Architecture                 string               `json:"architecture,omitempty"`
	MacAddress                   string               `json:"macaddress,omitempty"`
	SupportedControllerProtocols string               `json:"supported_controller_protocol,omitempty"`
	SupportedDeviceProtocols     string               `json:"supported_device_protocol,omitempty"`
	SupportedRAIDTypes           string               `json:"supported_raid_types,omitempty"`
	PhysicalID                   string               `json:"physid,omitempty"`
	FormFactor                   string               `json:"form_factor,omitempty"`
	PartNumber                   string               `json:"part_number,omitempty"`
	OemID                        string               `json:"oem_id,omitempty"`
	DriveType                    string               `json:"drive_type,omitempty"`
	StorageController            string               `json:"storage_controller,omitempty"`
	BusInfo                      string               `json:"bus_info,omitempty"`
	WWN                          string               `json:"wwn,omitempty"`
	Protocol                     string               `json:"protocol,omitempty"`
	SmartErrors                  []string             `json:"smart_errors,omitempty"`
	PowerCapacityWatts           int64                `json:"power_capacity_watts,omitempty"`
	SizeBytes                    int64                `json:"size_bytes,omitempty"`
	CapacityBytes                int64                `json:"capacity_bytes,omitempty" diff:"immutable"`
	ClockSpeedHz                 int64                `json:"clock_speed_hz,omitempty"`
	Cores                        int                  `json:"cores,omitempty"`
	Threads                      int                  `json:"threads,omitempty"`
	SpeedBits                    int64                `json:"speed_bits,omitempty"`
	SpeedGbps                    int64                `json:"speed_gbps,omitempty"`
	BlockSizeBytes               int64                `json:"block_size_bytes,omitempty"`
	CapableSpeedGbps             int64                `json:"capable_speed_gbps,omitempty"`
	NegotiatedSpeedGbps          int64                `json:"negotiated_speed_gbps,omitempty"`
	Oem                          bool                 `json:"oem,omitempty"`
}

// versionedAttributes are component attributes to be versioned in server service
type versionedAttributes struct {
	Firmware    *common.Firmware `json:"firmware,omitempty"`
	Status      *common.Status   `json:"status,omitempty"`
	SmartStatus string           `json:"smart_status,omitempty"`
}

func (h *serverServicePublisher) setAttributes(component *serverservice.ServerComponent, attr *attributes) {
	// convert attributes to raw json
	data, err := json.Marshal(attr)
	if err != nil {
		h.logger.WithFields(
			logrus.Fields{
				"slug": component.ComponentTypeSlug,
				"kind": fmt.Sprintf("%T", data),
				"err":  err,
			}).Warn("error in conversion of versioned attributes to raw data")
	}

	// skip min sized json data containing just the braces `{}`
	min := 2
	if len(data) == min {
		return
	}

	if component.Attributes == nil {
		component.Attributes = []serverservice.Attributes{}
	}

	component.Attributes = append(
		component.Attributes,
		serverservice.Attributes{
			Namespace: h.attributeNS,
			Data:      data,
		},
	)
}

func (h *serverServicePublisher) setVersionedAttributes(component *serverservice.ServerComponent, vattr *versionedAttributes) {
	// convert versioned attributes to raw json
	data, err := json.Marshal(vattr)
	if err != nil {
		h.logger.WithFields(
			logrus.Fields{
				"slug": component.ComponentTypeSlug,
				"kind": fmt.Sprintf("%T", data),
				"err":  err,
			}).Warn("error in conversion of versioned attributes to raw data")
	}

	// skip empty json data containing just the braces `{}`
	min := 2
	if len(data) == min {
		return
	}

	if component.VersionedAttributes == nil {
		component.VersionedAttributes = []serverservice.VersionedAttributes{}
	}

	component.VersionedAttributes = append(
		component.VersionedAttributes,
		serverservice.VersionedAttributes{
			Namespace: h.versionedAttributeNS,
			Data:      data,
		},
	)
}
