package fixtures

import (
	fleetdbapi "github.com/metal-toolbox/fleetdb/pkg/api/v1"
)

var (
	ServerServiceComponentTypes = fleetdbapi.ServerComponentTypeSlice{
		&fleetdbapi.ServerComponentType{
			ID:   "02dc2503-b64c-439b-9f25-8e130705f14a",
			Name: "Backplane-Expander",
			Slug: "backplane-expander",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "1e0c3417-d63c-4fd5-88f7-4c525c70da12",
			Name: "Mainboard",
			Slug: "mainboard",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "262e1a12-25a0-4d84-8c79-b3941603d48e",
			Name: "BIOS",
			Slug: "bios",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "322b8728-dcc9-44e3-a139-81220c75a339",
			Name: "NVMe-PCIe-SSD",
			Slug: "nvme-pcie-ssd",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "352631d2-b1ed-4d8e-85f7-9c92ddb76679",
			Name: "Sata-SSD",
			Slug: "sata-ssd",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "3717d747-3cc3-4800-822c-4c7a9ac2c314",
			Name: "Drive",
			Slug: "drive",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "3fc448ce-ea68-4f7c-beb1-c376f594db80",
			Name: "Chassis",
			Slug: "chassis",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "4588a8fb-2e0f-4fa1-9634-9819a319b70b",
			Name: "GPU",
			Slug: "gpu",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "5850ede2-d6d6-4df7-89d6-eab9110a9113",
			Name: "NIC",
			Slug: "nic",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "5ac890cc-dd92-4609-9615-ca4b05b62a8e",
			Name: "PhysicalMemory",
			Slug: "physicalmemory",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "75fc736e-fe42-4495-8e62-02d46fd08528",
			Name: "CPU",
			Slug: "cpu",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "79ad53a2-0c05-4912-a156-8311bd54017d",
			Name: "TPM",
			Slug: "tpm",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "9f5f39a4-82ed-4870-ab32-268bec45c8c8",
			Name: "Enclosure",
			Slug: "enclosure",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "cbfbbe99-8d79-49e0-8f5d-c5296932bbd1",
			Name: "Sata-HDD",
			Slug: "sata-hdd",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "ce396912-210e-4f6e-902d-9f07a8efe092",
			Name: "CPLD",
			Slug: "cpld",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "d51b438b-a767-459e-8eda-fd0700a46686",
			Name: "Power-Supply",
			Slug: "power-supply",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "e96c8557-4a71-4887-a3bb-28b6f90e5489",
			Name: "BMC",
			Slug: "bmc",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "eb82dbe3-df77-4409-833b-c44241885410",
			Name: "unknown",
			Slug: "unknown",
		},
		&fleetdbapi.ServerComponentType{
			ID:   "ef563926-8011-4985-bc4a-7ed7e9933971",
			Name: "StorageController",
			Slug: "storagecontroller",
		},
	}
)

func ServerServiceSlugMap() map[string]*fleetdbapi.ServerComponentType {
	m := make(map[string]*fleetdbapi.ServerComponentType)

	for _, ct := range ServerServiceComponentTypes {
		m[ct.Slug] = ct
	}

	return m
}
