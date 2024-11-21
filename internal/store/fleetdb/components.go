package fleetdb

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	common "github.com/metal-toolbox/bmc-common"
	fleetdbapi "github.com/metal-toolbox/fleetdb/pkg/api/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/metal-toolbox/alloy/internal/model"
)

// devel notes
// -----------
// - Components are stored in fleetdb with a composite unique constraint on the component serial, component_type and server_id.
// - When making changes to the way the serial is generated here (if one does not exist)
//   keep in mind that this will affect existing data in fleetdb, that is the components with newer serials
//   will end up being new components added.

// componentBySlugSerial returns a pointer to a component that matches the given slug, serial attributes
func componentBySlugSerial(slug, serial string, components []*fleetdbapi.ServerComponent) *fleetdbapi.ServerComponent {
	for _, c := range components {
		if strings.EqualFold(slug, c.ComponentTypeSlug) && strings.EqualFold(serial, c.Serial) {
			return c
		}
	}

	return nil
}

// componentPtrSlice returns a slice of pointers to fleetdbapi.ServerComponent.
//
// The hollow client methods require component slice objects to be passed as values
// these tend to be large objects.
//
// This helper method is to reduce the amount of copying of component objects (~240 bytes each) when passed around between methods and range loops,
// while it seems like a minor optimization, it also keeps the linter happy.
func componentPtrSlice(components fleetdbapi.ServerComponentSlice) []*fleetdbapi.ServerComponent {
	s := make([]*fleetdbapi.ServerComponent, 0, len(components))

	// nolint:gocritic // the copying has to be done somewhere
	for _, c := range components {
		c := c
		s = append(s, &c)
	}

	return s
}

// toComponentSlice converts an model.AssetDevice object to the server service component slice object
func (r *Store) toComponentSlice(serverID uuid.UUID, device *model.Asset) ([]*fleetdbapi.ServerComponent, error) {
	componentsTmp := []*fleetdbapi.ServerComponent{}
	componentsTmp = append(componentsTmp,
		r.bios(device.Vendor, device.Inventory.BIOS),
		r.bmc(device.Vendor, device.Inventory.BMC),
		r.mainboard(device.Vendor, device.Inventory.Mainboard),
	)

	componentsTmp = append(componentsTmp, r.dimms(device.Vendor, device.Inventory.Memory)...)
	componentsTmp = append(componentsTmp, r.nics(device.Vendor, device.Inventory.NICs)...)
	componentsTmp = append(componentsTmp, r.drives(device.Vendor, device.Inventory.Drives)...)
	componentsTmp = append(componentsTmp, r.psus(device.Vendor, device.Inventory.PSUs)...)
	componentsTmp = append(componentsTmp, r.cpus(device.Vendor, device.Inventory.CPUs)...)
	componentsTmp = append(componentsTmp, r.tpms(device.Vendor, device.Inventory.TPMs)...)
	componentsTmp = append(componentsTmp, r.cplds(device.Vendor, device.Inventory.CPLDs)...)
	componentsTmp = append(componentsTmp, r.gpus(device.Vendor, device.Inventory.GPUs)...)
	componentsTmp = append(componentsTmp, r.storageControllers(device.Vendor, device.Inventory.StorageControllers)...)
	componentsTmp = append(componentsTmp, r.enclosures(device.Vendor, device.Inventory.Enclosures)...)

	components := []*fleetdbapi.ServerComponent{}

	for _, component := range componentsTmp {
		if component == nil || r.requiredAttributesEmpty(component) {
			continue
		}

		component.ServerUUID = serverID
		components = append(components, component)
	}

	return components, nil
}

func (r *Store) requiredAttributesEmpty(component *fleetdbapi.ServerComponent) bool {
	return component.Serial == "0" &&
		component.Model == "" &&
		component.Vendor == "" &&
		len(component.Attributes) == 0 &&
		len(component.VersionedAttributes) == 0
}

func (r *Store) newComponent(slug, cvendor, cmodel, cserial, cproduct string) (*fleetdbapi.ServerComponent, error) {
	// lower case slug to changeObj how its stored in server service
	slug = strings.ToLower(slug)

	// component slug lookup map is expected
	if len(r.slugs) == 0 {
		return nil, errors.Wrap(ErrSlugs, "component slugs lookup map empty")
	}

	// component slug is part of the lookup map
	_, exists := r.slugs[slug]
	if !exists {
		return nil, errors.Wrap(ErrSlugs, "unknown component slug: "+slug)
	}

	// use the product name when model number is empty
	if strings.TrimSpace(cmodel) == "" && strings.TrimSpace(cproduct) != "" {
		cmodel = cproduct
	}

	return &fleetdbapi.ServerComponent{
		Name:              r.slugs[slug].Name,
		Vendor:            common.FormatVendorName(cvendor),
		Model:             cmodel,
		Serial:            cserial,
		ComponentTypeID:   r.slugs[slug].ID,
		ComponentTypeName: r.slugs[slug].Name,
		ComponentTypeSlug: slug,
	}, nil
}

func (r *Store) gpus(deviceVendor string, gpus []*common.GPU) []*fleetdbapi.ServerComponent {
	if gpus == nil {
		return nil
	}

	components := make([]*fleetdbapi.ServerComponent, 0, len(gpus))

	for idx, c := range gpus {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := r.newComponent(common.SlugGPU, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			r.logger.Error(err)

			return nil
		}

		r.setAttributes(
			sc,
			&attributes{
				Description:  c.Description,
				ProductName:  c.ProductName,
				Metadata:     c.Metadata,
				Capabilities: c.Capabilities,
			},
		)

		r.setFirmwareVA(
			deviceVendor,
			sc,
			&firmwareVersionedAttribute{
				Firmware: c.Firmware,
			},
		)

		r.setStatusVA(
			sc,
			[]*statusVersionedAttribute{
				{
					Status: c.Status,
				},
			},
		)

		components = append(components, sc)
	}

	return components
}

func (r *Store) cplds(deviceVendor string, cplds []*common.CPLD) []*fleetdbapi.ServerComponent {
	if cplds == nil {
		return nil
	}

	components := make([]*fleetdbapi.ServerComponent, 0, len(cplds))

	for idx, c := range cplds {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := r.newComponent(common.SlugCPLD, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			r.logger.Error(err)

			return nil
		}

		r.setAttributes(
			sc,
			&attributes{
				Description:  c.Description,
				ProductName:  c.ProductName,
				Metadata:     c.Metadata,
				Capabilities: c.Capabilities,
			},
		)

		r.setFirmwareVA(
			deviceVendor,
			sc,
			&firmwareVersionedAttribute{
				Firmware: c.Firmware,
			},
		)

		r.setStatusVA(
			sc,
			[]*statusVersionedAttribute{
				{
					Status: c.Status,
				},
			},
		)

		components = append(components, sc)
	}

	return components
}

func (r *Store) tpms(deviceVendor string, tpms []*common.TPM) []*fleetdbapi.ServerComponent {
	if tpms == nil {
		return nil
	}

	components := make([]*fleetdbapi.ServerComponent, 0, len(tpms))

	for idx, c := range tpms {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := r.newComponent(common.SlugTPM, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			r.logger.Error(err)

			return nil
		}

		r.setAttributes(
			sc,
			&attributes{
				Description:   c.Description,
				ProductName:   c.ProductName,
				Metadata:      c.Metadata,
				Capabilities:  c.Capabilities,
				InterfaceType: c.InterfaceType,
			},
		)

		r.setFirmwareVA(
			deviceVendor,
			sc,
			&firmwareVersionedAttribute{
				Firmware: c.Firmware,
			},
		)

		r.setStatusVA(
			sc,
			[]*statusVersionedAttribute{
				{
					Status: c.Status,
				},
			},
		)

		components = append(components, sc)
	}

	return components
}

func (r *Store) cpus(deviceVendor string, cpus []*common.CPU) []*fleetdbapi.ServerComponent {
	if cpus == nil {
		return nil
	}

	components := make([]*fleetdbapi.ServerComponent, 0, len(cpus))

	for idx, c := range cpus {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := r.newComponent(common.SlugCPU, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			r.logger.Error(err)

			return nil
		}

		r.setAttributes(
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

		r.setFirmwareVA(
			deviceVendor,
			sc,
			&firmwareVersionedAttribute{
				Firmware: c.Firmware,
			},
		)

		r.setStatusVA(
			sc,
			[]*statusVersionedAttribute{
				{
					Status: c.Status,
				},
			},
		)

		components = append(components, sc)
	}

	return components
}

func (r *Store) storageControllers(deviceVendor string, controllers []*common.StorageController) []*fleetdbapi.ServerComponent {
	if controllers == nil {
		return nil
	}

	components := make([]*fleetdbapi.ServerComponent, 0, len(controllers))

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

		sc, err := r.newComponent(common.SlugStorageController, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			r.logger.Error(err)

			return nil
		}

		r.setAttributes(
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

		r.setFirmwareVA(
			deviceVendor,
			sc,
			&firmwareVersionedAttribute{
				Firmware: c.Firmware,
			},
		)

		r.setStatusVA(
			sc,
			[]*statusVersionedAttribute{
				{
					Status: c.Status,
				},
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

func (r *Store) psus(deviceVendor string, psus []*common.PSU) []*fleetdbapi.ServerComponent {
	if psus == nil {
		return nil
	}

	components := make([]*fleetdbapi.ServerComponent, 0, len(psus))

	for idx, c := range psus {
		trimedSerial := strings.TrimSpace(c.Serial)
		if trimedSerial == "" || strings.Contains(trimedSerial, "To Be Filled By O.E.M.") {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := r.newComponent(common.SlugPSU, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			r.logger.Error(err)

			return nil
		}

		r.setAttributes(
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

		r.setFirmwareVA(
			deviceVendor,
			sc,
			&firmwareVersionedAttribute{
				Firmware: c.Firmware,
			},
		)

		r.setStatusVA(
			sc,
			[]*statusVersionedAttribute{
				{
					Status: c.Status,
				},
			},
		)

		components = append(components, sc)
	}

	return components
}

func (r *Store) drives(deviceVendor string, drives []*common.Drive) []*fleetdbapi.ServerComponent {
	if drives == nil {
		return nil
	}

	components := make([]*fleetdbapi.ServerComponent, 0, len(drives))

	for idx, c := range drives {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := r.newComponent(common.SlugDrive, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			r.logger.Error(err)

			return nil
		}

		r.setAttributes(
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
				SmartStatus:         c.SmartStatus,
				DriveType:           c.Type,
				WWN:                 c.WWN,
				CapacityBytes:       c.CapacityBytes,
				BlockSizeBytes:      c.BlockSizeBytes,
				CapableSpeedGbps:    c.CapableSpeedGbps,
				NegotiatedSpeedGbps: c.NegotiatedSpeedGbps,
				Capabilities:        c.Capabilities,
			},
		)

		r.setFirmwareVA(
			deviceVendor,
			sc,
			&firmwareVersionedAttribute{
				Firmware: c.Firmware,
			},
		)

		r.setStatusVA(
			sc,
			[]*statusVersionedAttribute{
				{
					Status: c.Status,
				},
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

func (r *Store) nics(deviceVendor string, nics []*common.NIC) []*fleetdbapi.ServerComponent {
	if nics == nil {
		return nil
	}

	components := make([]*fleetdbapi.ServerComponent, 0, len(nics))

	for idx, c := range nics {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := r.newComponent(common.SlugNIC, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			r.logger.Error(err)

			return nil
		}

		// NIC port attributes go in here
		nicPortAttrs := []*attributes{}

		// include NIC firmware attributes
		r.setFirmwareVA(
			deviceVendor,
			sc,
			&firmwareVersionedAttribute{
				Firmware: c.Firmware,
			},
		)

		// NIC port attributes are populated below.
		statusesVA := []*statusVersionedAttribute{
			{
				Status: c.Status,
			},
		}

		// TODO: fix up duplicate NIC attribute being dropped
		for _, p := range c.NICPorts {
			nicPortAttrs = append(nicPortAttrs, &attributes{
				ID:           p.ID,
				Description:  c.Description,
				ProductName:  c.ProductName,
				Oem:          c.Oem,
				Metadata:     c.Metadata,
				PhysicalID:   p.PhysicalID,
				BusInfo:      p.BusInfo,
				MacAddress:   p.MacAddress,
				SpeedBits:    p.SpeedBits,
				Capabilities: c.Capabilities,
			})

			if p.Status == nil && p.LinkStatus == "" && p.ActiveLinkTechnology == "" && !p.AutoNeg && p.MTUSize == 0 {
				continue
			}
			// Store the NIC Port status
			statusesVA = append(statusesVA, &statusVersionedAttribute{
				NicPortStatus: &nicPortStatus{
					ID:                   p.ID,
					MTUSize:              p.MTUSize,
					MacAddress:           p.MacAddress,
					Status:               p.Status,
					LinkStatus:           p.LinkStatus,
					AutoSpeedNegotiation: p.AutoNeg,
					ActiveLinkTechnology: p.ActiveLinkTechnology,
				},
			})
		}

		r.setAttributesList(sc, nicPortAttrs)
		r.setStatusVA(sc, statusesVA)

		components = append(components, sc)
	}

	return components
}

func (r *Store) dimms(deviceVendor string, dimms []*common.Memory) []*fleetdbapi.ServerComponent {
	if dimms == nil {
		return nil
	}

	components := make([]*fleetdbapi.ServerComponent, 0, len(dimms))

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

		sc, err := r.newComponent(common.SlugPhysicalMem, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			r.logger.Error(err)

			return nil
		}

		r.setAttributes(
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

		r.setFirmwareVA(
			deviceVendor,
			sc,
			&firmwareVersionedAttribute{
				Firmware: c.Firmware,
			},
		)

		r.setStatusVA(
			sc,
			[]*statusVersionedAttribute{
				{
					Status: c.Status,
				},
			},
		)

		components = append(components, sc)
	}

	return components
}

func (r *Store) mainboard(deviceVendor string, c *common.Mainboard) *fleetdbapi.ServerComponent {
	if c == nil {
		return nil
	}

	if strings.TrimSpace(c.Serial) == "" {
		c.Serial = "0"
	}

	sc, err := r.newComponent(common.SlugMainboard, c.Vendor, c.Model, c.Serial, c.ProductName)
	if err != nil {
		r.logger.Error(err)

		return nil
	}

	r.setAttributes(
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

	r.setFirmwareVA(
		deviceVendor,
		sc,
		&firmwareVersionedAttribute{
			Firmware: c.Firmware,
		},
	)

	r.setStatusVA(
		sc,
		[]*statusVersionedAttribute{
			{
				Status: c.Status,
			},
		},
	)

	return sc
}

func (r *Store) enclosures(deviceVendor string, enclosures []*common.Enclosure) []*fleetdbapi.ServerComponent {
	if enclosures == nil {
		return nil
	}

	components := make([]*fleetdbapi.ServerComponent, 0, len(enclosures))

	for idx, c := range enclosures {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := r.newComponent(common.SlugEnclosure, c.Vendor, c.Model, c.Serial, c.ProductName)
		if err != nil {
			r.logger.Error(err)

			return nil
		}

		r.setAttributes(
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

		r.setFirmwareVA(
			deviceVendor,
			sc,
			&firmwareVersionedAttribute{
				Firmware: c.Firmware,
			},
		)

		r.setStatusVA(
			sc,
			[]*statusVersionedAttribute{
				{
					Status: c.Status,
				},
			},
		)

		components = append(components, sc)
	}

	return components
}

func (r *Store) bmc(deviceVendor string, c *common.BMC) *fleetdbapi.ServerComponent {
	if c == nil {
		return nil
	}

	if strings.TrimSpace(c.Serial) == "" {
		c.Serial = "0"
	}

	sc, err := r.newComponent(common.SlugBMC, c.Vendor, c.Model, c.Serial, c.ProductName)
	if err != nil {
		r.logger.Error(err)

		return nil
	}

	r.setAttributes(
		sc,
		&attributes{
			Description:  c.Description,
			ProductName:  c.ProductName,
			Oem:          c.Oem,
			Metadata:     c.Metadata,
			Capabilities: c.Capabilities,
		},
	)

	r.setFirmwareVA(
		deviceVendor,
		sc,
		&firmwareVersionedAttribute{
			Firmware: c.Firmware,
		},
	)

	r.setStatusVA(
		sc,
		[]*statusVersionedAttribute{
			{
				Status: c.Status,
			},
		},
	)

	return sc
}

func (r *Store) bios(deviceVendor string, c *common.BIOS) *fleetdbapi.ServerComponent {
	if c == nil {
		return nil
	}

	if strings.TrimSpace(c.Serial) == "" {
		c.Serial = "0"
	}

	sc, err := r.newComponent(common.SlugBIOS, c.Vendor, c.Model, c.Serial, c.ProductName)
	if err != nil {
		r.logger.Error(err)

		return nil
	}

	r.setAttributes(
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

	r.setFirmwareVA(
		deviceVendor,
		sc,
		&firmwareVersionedAttribute{
			Firmware: c.Firmware,
		},
	)

	r.setStatusVA(
		sc,
		[]*statusVersionedAttribute{
			{
				Status: c.Status,
			},
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
	SmartStatus                  string               `json:"smart_status,omitempty"`
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

// nicPortStatus holds the NIC port status which includes the health status and link status information.
type nicPortStatus struct {
	*common.Status
	ID                   string `json:"id,omitempty"`
	MacAddress           string `json:"macaddress,omitempty"`
	ActiveLinkTechnology string `json:"active_link_technology,omitempty"`
	LinkStatus           string `json:"link_status,omitempty"`
	MTUSize              int    `json:"mtu_size,omitempty"`
	AutoSpeedNegotiation bool   `json:"autospeednegotiation,omitempty"`
}

// firmwareVersionedAttribute holds component firmware information.
type firmwareVersionedAttribute struct {
	Firmware *common.Firmware `json:"firmware,omitempty"`
	UUID     *uuid.UUID       `json:"uuid,omitempty"` // UUID references firmware UUID identified in fleetdb based on component/device attributes.
	Vendor   string           `json:"vendor,omitempty"`
}

// statusVersionedAttribute holds component status information.
type statusVersionedAttribute struct {
	Status        *common.Status `json:"status,omitempty"`
	NicPortStatus *nicPortStatus `json:"nic_port_status,omitempty"`
	SmartStatus   string         `json:"smart_status,omitempty"`
}

// setAttributesList updates the given component with the given list of attributes.
//
// attributes per component in fleetdb have a unique constraint on the component ID, namespace values.
//
// so if this method is called twice for the same component, namespace, that attribute will be ignored,
func (r *Store) setAttributesList(component *fleetdbapi.ServerComponent, attrs []*attributes) {
	if len(attrs) == 0 {
		return
	}

	// convert attributes to raw json
	data, err := json.Marshal(attrs)
	if err != nil {
		r.logger.WithFields(
			logrus.Fields{
				"slug": component.ComponentTypeSlug,
				"kind": fmt.Sprintf("%T", data),
				"err":  err,
			}).Warn("error in conversion of versioned attributes to raw data")
	}

	if component.Attributes == nil {
		component.Attributes = []fleetdbapi.Attributes{}
	} else {
		for _, existingA := range component.Attributes {
			if existingA.Namespace != r.attributeNS {
				continue
			}

			r.logger.WithFields(
				logrus.Fields{
					"slug":      component.ComponentTypeSlug,
					"kind":      fmt.Sprintf("%T", data),
					"namespace": r.attributeNS,
				}).Warn("duplicate attribute list on component dropped - this is unexpected.")

			return
		}
	}

	component.Attributes = append(
		component.Attributes,
		fleetdbapi.Attributes{
			Namespace: r.attributeNS,
			Data:      data,
		},
	)
}

// setAttributes updates the fleetdb API component object with the given attributes
func (r *Store) setAttributes(component *fleetdbapi.ServerComponent, attr *attributes) {
	// convert attributes to raw json
	data, err := json.Marshal(attr)
	if err != nil {
		r.logger.WithFields(
			logrus.Fields{
				"slug": component.ComponentTypeSlug,
				"kind": fmt.Sprintf("%T", data),
				"err":  err,
			}).Warn("error in conversion of versioned attributes to raw data")
	}

	// skip min sized json data containing just the braces `{}`
	smallest := 2
	if len(data) == smallest {
		return
	}

	if component.Attributes == nil {
		component.Attributes = []fleetdbapi.Attributes{}
	} else {
		for _, existingA := range component.Attributes {
			if existingA.Namespace != r.attributeNS {
				continue
			}

			r.logger.WithFields(
				logrus.Fields{
					"slug":      component.ComponentTypeSlug,
					"kind":      fmt.Sprintf("%T", data),
					"namespace": r.attributeNS,
				}).Warn("duplicate attribute on component dropped - use setAttributesList() instead.")

			return
		}
	}

	component.Attributes = append(
		component.Attributes,
		fleetdbapi.Attributes{
			Namespace: r.attributeNS,
			Data:      data,
		},
	)
}

// setStatusVA updates the given component with given list of statusVersionedAttributes.
//
// versioned attributes per component in fleetdb have a unique constraint on
// the component ID, namespace values.
//
// so if this method is called twice for the same component, namespace, that statusVersionedAttribute will be ignored.
func (r *Store) setStatusVA(component *fleetdbapi.ServerComponent, statusesVA []*statusVersionedAttribute) {
	if len(statusesVA) == 0 {
		return
	}

	// convert versioned attributes to raw json
	data, err := json.Marshal(statusesVA)
	if err != nil {
		r.logger.WithFields(
			logrus.Fields{
				"slug": component.ComponentTypeSlug,
				"kind": fmt.Sprintf("%T", data),
				"err":  err,
			}).Warn("error in conversion of versioned attributes to raw data")
	}

	// skip empty json data containing just the braces `[{}]`
	smallest := 4
	if len(data) == smallest {
		return
	}

	if component.VersionedAttributes == nil {
		component.VersionedAttributes = []fleetdbapi.VersionedAttributes{}
	} else {
		for _, existingVA := range component.VersionedAttributes {
			if existingVA.Namespace != r.statusVersionedAttributeNS {
				continue
			}

			r.logger.WithFields(
				logrus.Fields{
					"slug":      component.ComponentTypeSlug,
					"kind":      fmt.Sprintf("%T", data),
					"namespace": r.statusVersionedAttributeNS,
				}).Warn("duplicate versioned attribute on component dropped.")

			return
		}
	}

	component.VersionedAttributes = append(
		component.VersionedAttributes,
		fleetdbapi.VersionedAttributes{
			Namespace: r.statusVersionedAttributeNS,
			Data:      data,
		},
	)
}

// setFirmwareVA updates a component with single firmwareVersionedAttribute.
//
// versioned attributes per component in fleetdb have a unique constraint on
// the component ID, namespace values.
//
// so if this method is called twice for the same component, namespace, that versioned attribute will be ignored.
func (r *Store) setFirmwareVA(deviceVendor string, component *fleetdbapi.ServerComponent, fwVA *firmwareVersionedAttribute) {
	// add FirmwareData
	if fwVA.Firmware != nil {
		r.enrichFirmwareData(deviceVendor, component.Vendor, fwVA)
	}

	// convert versioned attributes to raw json
	data, err := json.Marshal(fwVA)
	if err != nil {
		r.logger.WithFields(
			logrus.Fields{
				"slug": component.ComponentTypeSlug,
				"kind": fmt.Sprintf("%T", data),
				"err":  err,
			}).Warn("error in conversion of versioned attributes to raw data")
	}

	// skip empty json data containing just the braces `{}`
	smallest := 2
	if len(data) == smallest {
		return
	}

	if component.VersionedAttributes == nil {
		component.VersionedAttributes = []fleetdbapi.VersionedAttributes{}
	} else {
		for _, existingVA := range component.VersionedAttributes {
			if existingVA.Namespace != r.firmwareVersionedAttributeNS {
				continue
			}

			r.logger.WithFields(
				logrus.Fields{
					"slug":      component.ComponentTypeSlug,
					"kind":      fmt.Sprintf("%T", data),
					"namespace": r.firmwareVersionedAttributeNS,
				}).Warn("duplicate versioned attribute on component dropped.")

			return
		}
	}

	component.VersionedAttributes = append(
		component.VersionedAttributes,
		fleetdbapi.VersionedAttributes{
			Namespace: r.firmwareVersionedAttributeNS,
			Data:      data,
		},
	)
}

// enrichFirmwareData queries FleetDB for the firmware version and try to find a match.
//
// the given versionedAttribute object is updated to include the firmware vendor and the fleetdb firmware UUID.
func (r *Store) enrichFirmwareData(deviceVendor, componentVendor string, vattr *firmwareVersionedAttribute) {
	// Check in the cache if we have a match by vendor + version
	for _, fw := range r.firmwares[componentVendor] {
		if strings.EqualFold(fw.Version, vattr.Firmware.Installed) {
			fwUUID := fw.UUID

			vattr.Vendor = fw.Vendor
			vattr.UUID = &fwUUID

			return
		}
	}

	for _, fw := range r.firmwares[deviceVendor] {
		if strings.EqualFold(fw.Version, vattr.Firmware.Installed) {
			fwUUID := fw.UUID

			vattr.Vendor = fw.Vendor
			vattr.UUID = &fwUUID

			return
		}
	}
}
