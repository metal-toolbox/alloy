package fixtures

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

var (
	// To refresh this fixture, see docs/README.development
	// nolint:simplifycompositelit // testdata
	ServerServiceE3C246D4INLcomponents = serverservice.ServerComponentSlice{
		serverservice.ServerComponent{
			UUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			ServerUUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			Name:   "BIOS",
			Vendor: "american megatrends",
			Model:  "",
			Serial: "0",
			Attributes: []serverservice.Attributes{
				serverservice.Attributes{
					Namespace: "sh.hollow.alloy.outofband.metadata",
					Data: json.RawMessage{
						123,
						34,
						100,
						101,
						115,
						99,
						114,
						105,
						112,
						116,
						105,
						111,
						110,
						34,
						58,
						34,
						66,
						73,
						79,
						83,
						34,
						44,
						34,
						115,
						105,
						122,
						101,
						95,
						98,
						121,
						116,
						101,
						115,
						34,
						58,
						54,
						53,
						53,
						51,
						54,
						44,
						34,
						99,
						97,
						112,
						97,
						99,
						105,
						116,
						121,
						95,
						98,
						121,
						116,
						101,
						115,
						34,
						58,
						51,
						51,
						53,
						53,
						52,
						52,
						51,
						50,
						125,
					},
					CreatedAt: time.Time{},
					UpdatedAt: time.Time{},
				},
			},
			VersionedAttributes: []serverservice.VersionedAttributes{
				serverservice.VersionedAttributes{
					Namespace: "sh.hollow.alloy.outofband.status",
					Data: json.RawMessage{
						123,
						34,
						102,
						105,
						114,
						109,
						119,
						97,
						114,
						101,
						34,
						58,
						123,
						34,
						105,
						110,
						115,
						116,
						97,
						108,
						108,
						101,
						100,
						34,
						58,
						34,
						76,
						50,
						46,
						48,
						55,
						66,
						34,
						125,
						125,
					},
					Tally:          0,
					LastReportedAt: time.Time{},
					CreatedAt:      time.Time{},
				},
			},
			ComponentTypeID:   "262e1a12-25a0-4d84-8c79-b3941603d48e",
			ComponentTypeName: "BIOS",
			ComponentTypeSlug: "bios",
			CreatedAt:         time.Time{},
			UpdatedAt:         time.Time{},
		},
		serverservice.ServerComponent{
			UUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			ServerUUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			Name:   "Mainboard",
			Vendor: "asrockrack",
			Model:  "E3C246D4I-NL",
			Serial: "196231220000153",
			Attributes: []serverservice.Attributes{
				serverservice.Attributes{
					Namespace: "sh.hollow.alloy.outofband.metadata",
					Data: json.RawMessage{
						123,
						34,
						100,
						101,
						115,
						99,
						114,
						105,
						112,
						116,
						105,
						111,
						110,
						34,
						58,
						34,
						77,
						111,
						116,
						104,
						101,
						114,
						98,
						111,
						97,
						114,
						100,
						34,
						44,
						34,
						112,
						114,
						111,
						100,
						117,
						99,
						116,
						95,
						110,
						97,
						109,
						101,
						34,
						58,
						34,
						69,
						51,
						67,
						50,
						52,
						54,
						68,
						52,
						73,
						45,
						78,
						76,
						34,
						44,
						34,
						112,
						104,
						121,
						115,
						105,
						100,
						34,
						58,
						34,
						48,
						34,
						125,
					},
					CreatedAt: time.Time{},
					UpdatedAt: time.Time{},
				},
			},
			VersionedAttributes: nil,
			ComponentTypeID:     "1e0c3417-d63c-4fd5-88f7-4c525c70da12",
			ComponentTypeName:   "Mainboard",
			ComponentTypeSlug:   "mainboard",
			CreatedAt:           time.Time{},
			UpdatedAt:           time.Time{},
		},
		serverservice.ServerComponent{
			UUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			ServerUUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			Name:   "PhysicalMemory",
			Vendor: "micron",
			Model:  "18ASF2G72HZ-2G6E1",
			Serial: "F0F9053F",
			Attributes: []serverservice.Attributes{
				serverservice.Attributes{
					Namespace: "sh.hollow.alloy.outofband.metadata",
					Data: json.RawMessage{
						123,
						34,
						100,
						101,
						115,
						99,
						114,
						105,
						112,
						116,
						105,
						111,
						110,
						34,
						58,
						34,
						83,
						79,
						68,
						73,
						77,
						77,
						32,
						68,
						68,
						82,
						52,
						32,
						83,
						121,
						110,
						99,
						104,
						114,
						111,
						110,
						111,
						117,
						115,
						32,
						50,
						54,
						54,
						54,
						32,
						77,
						72,
						122,
						32,
						40,
						48,
						46,
						52,
						32,
						110,
						115,
						41,
						34,
						44,
						34,
						112,
						114,
						111,
						100,
						117,
						99,
						116,
						95,
						110,
						97,
						109,
						101,
						34,
						58,
						34,
						49,
						56,
						65,
						83,
						70,
						50,
						71,
						55,
						50,
						72,
						90,
						45,
						50,
						71,
						54,
						69,
						49,
						34,
						44,
						34,
						115,
						108,
						111,
						116,
						34,
						58,
						34,
						67,
						104,
						97,
						110,
						110,
						101,
						108,
						65,
						45,
						68,
						73,
						77,
						77,
						48,
						34,
						44,
						34,
						115,
						105,
						122,
						101,
						95,
						98,
						121,
						116,
						101,
						115,
						34,
						58,
						49,
						55,
						49,
						55,
						57,
						56,
						54,
						57,
						49,
						56,
						52,
						44,
						34,
						99,
						108,
						111,
						99,
						107,
						95,
						115,
						112,
						101,
						101,
						100,
						95,
						104,
						122,
						34,
						58,
						50,
						54,
						54,
						54,
						48,
						48,
						48,
						48,
						48,
						48,
						125,
					},
					CreatedAt: time.Time{},
					UpdatedAt: time.Time{},
				},
			},
			VersionedAttributes: nil,
			ComponentTypeID:     "5ac890cc-dd92-4609-9615-ca4b05b62a8e",
			ComponentTypeName:   "PhysicalMemory",
			ComponentTypeSlug:   "physicalmemory",
			CreatedAt:           time.Time{},
			UpdatedAt:           time.Time{},
		},
		serverservice.ServerComponent{
			UUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			ServerUUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			Name:   "PhysicalMemory",
			Vendor: "micron",
			Model:  "18ASF2G72HZ-2G6E1",
			Serial: "F0F90894",
			Attributes: []serverservice.Attributes{
				serverservice.Attributes{
					Namespace: "sh.hollow.alloy.outofband.metadata",
					Data: json.RawMessage{
						123,
						34,
						100,
						101,
						115,
						99,
						114,
						105,
						112,
						116,
						105,
						111,
						110,
						34,
						58,
						34,
						83,
						79,
						68,
						73,
						77,
						77,
						32,
						68,
						68,
						82,
						52,
						32,
						83,
						121,
						110,
						99,
						104,
						114,
						111,
						110,
						111,
						117,
						115,
						32,
						50,
						54,
						54,
						54,
						32,
						77,
						72,
						122,
						32,
						40,
						48,
						46,
						52,
						32,
						110,
						115,
						41,
						34,
						44,
						34,
						112,
						114,
						111,
						100,
						117,
						99,
						116,
						95,
						110,
						97,
						109,
						101,
						34,
						58,
						34,
						49,
						56,
						65,
						83,
						70,
						50,
						71,
						55,
						50,
						72,
						90,
						45,
						50,
						71,
						54,
						69,
						49,
						34,
						44,
						34,
						115,
						108,
						111,
						116,
						34,
						58,
						34,
						67,
						104,
						97,
						110,
						110,
						101,
						108,
						66,
						45,
						68,
						73,
						77,
						77,
						48,
						34,
						44,
						34,
						115,
						105,
						122,
						101,
						95,
						98,
						121,
						116,
						101,
						115,
						34,
						58,
						49,
						55,
						49,
						55,
						57,
						56,
						54,
						57,
						49,
						56,
						52,
						44,
						34,
						99,
						108,
						111,
						99,
						107,
						95,
						115,
						112,
						101,
						101,
						100,
						95,
						104,
						122,
						34,
						58,
						50,
						54,
						54,
						54,
						48,
						48,
						48,
						48,
						48,
						48,
						125,
					},
					CreatedAt: time.Time{},
					UpdatedAt: time.Time{},
				},
			},
			VersionedAttributes: nil,
			ComponentTypeID:     "5ac890cc-dd92-4609-9615-ca4b05b62a8e",
			ComponentTypeName:   "PhysicalMemory",
			ComponentTypeSlug:   "physicalmemory",
			CreatedAt:           time.Time{},
			UpdatedAt:           time.Time{},
		},
		serverservice.ServerComponent{
			UUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			ServerUUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			Name:   "NIC",
			Vendor: "intel",
			Model:  "Ethernet Controller X710 for 10GbE SFP+",
			Serial: "b4:96:91:70:26:c8",
			Attributes: []serverservice.Attributes{
				serverservice.Attributes{
					Namespace: "sh.hollow.alloy.outofband.metadata",
					Data: json.RawMessage{
						123,
						34,
						109,
						101,
						116,
						97,
						100,
						97,
						116,
						97,
						34,
						58,
						123,
						34,
						100,
						114,
						105,
						118,
						101,
						114,
						34,
						58,
						34,
						105,
						52,
						48,
						101,
						34,
						44,
						34,
						100,
						117,
						112,
						108,
						101,
						120,
						34,
						58,
						34,
						102,
						117,
						108,
						108,
						34,
						44,
						34,
						102,
						105,
						114,
						109,
						119,
						97,
						114,
						101,
						34,
						58,
						34,
						54,
						46,
						48,
						49,
						32,
						48,
						120,
						56,
						48,
						48,
						48,
						51,
						102,
						97,
						49,
						32,
						49,
						46,
						49,
						56,
						53,
						51,
						46,
						48,
						34,
						44,
						34,
						108,
						105,
						110,
						107,
						34,
						58,
						34,
						121,
						101,
						115,
						34,
						44,
						34,
						115,
						112,
						101,
						101,
						100,
						34,
						58,
						34,
						49,
						48,
						71,
						98,
						105,
						116,
						47,
						115,
						34,
						125,
						44,
						34,
						100,
						101,
						115,
						99,
						114,
						105,
						112,
						116,
						105,
						111,
						110,
						34,
						58,
						34,
						69,
						116,
						104,
						101,
						114,
						110,
						101,
						116,
						32,
						105,
						110,
						116,
						101,
						114,
						102,
						97,
						99,
						101,
						34,
						44,
						34,
						112,
						114,
						111,
						100,
						117,
						99,
						116,
						95,
						110,
						97,
						109,
						101,
						34,
						58,
						34,
						69,
						116,
						104,
						101,
						114,
						110,
						101,
						116,
						32,
						67,
						111,
						110,
						116,
						114,
						111,
						108,
						108,
						101,
						114,
						32,
						88,
						55,
						49,
						48,
						32,
						102,
						111,
						114,
						32,
						49,
						48,
						71,
						98,
						69,
						32,
						83,
						70,
						80,
						43,
						34,
						44,
						34,
						112,
						104,
						121,
						115,
						105,
						100,
						34,
						58,
						34,
						48,
						34,
						44,
						34,
						98,
						117,
						115,
						95,
						105,
						110,
						102,
						111,
						34,
						58,
						34,
						112,
						99,
						105,
						64,
						48,
						48,
						48,
						48,
						58,
						48,
						49,
						58,
						48,
						48,
						46,
						48,
						34,
						44,
						34,
						115,
						112,
						101,
						101,
						100,
						95,
						98,
						105,
						116,
						115,
						34,
						58,
						49,
						48,
						48,
						48,
						48,
						48,
						48,
						48,
						48,
						48,
						48,
						125,
					},
					CreatedAt: time.Time{},
					UpdatedAt: time.Time{},
				},
			},
			VersionedAttributes: []serverservice.VersionedAttributes{
				serverservice.VersionedAttributes{
					Namespace: "sh.hollow.alloy.outofband.status",
					Data: json.RawMessage{
						123,
						34,
						102,
						105,
						114,
						109,
						119,
						97,
						114,
						101,
						34,
						58,
						123,
						34,
						105,
						110,
						115,
						116,
						97,
						108,
						108,
						101,
						100,
						34,
						58,
						34,
						49,
						46,
						49,
						56,
						53,
						51,
						46,
						48,
						34,
						125,
						125,
					},
					Tally:          0,
					LastReportedAt: time.Time{},
					CreatedAt:      time.Time{},
				},
			},
			ComponentTypeID:   "5850ede2-d6d6-4df7-89d6-eab9110a9113",
			ComponentTypeName: "NIC",
			ComponentTypeSlug: "nic",
			CreatedAt:         time.Time{},
			UpdatedAt:         time.Time{},
		},
		serverservice.ServerComponent{
			UUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			ServerUUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			Name:   "Drive",
			Vendor: "intel",
			Model:  "INTEL SSDSC2KB48",
			Serial: "PHYF001300HB480BGN",
			Attributes: []serverservice.Attributes{
				serverservice.Attributes{
					Namespace: "sh.hollow.alloy.outofband.metadata",
					Data: json.RawMessage{
						123,
						34,
						100,
						101,
						115,
						99,
						114,
						105,
						112,
						116,
						105,
						111,
						110,
						34,
						58,
						34,
						65,
						84,
						65,
						32,
						68,
						105,
						115,
						107,
						34,
						44,
						34,
						112,
						114,
						111,
						100,
						117,
						99,
						116,
						95,
						110,
						97,
						109,
						101,
						34,
						58,
						34,
						73,
						78,
						84,
						69,
						76,
						32,
						83,
						83,
						68,
						83,
						67,
						50,
						75,
						66,
						52,
						56,
						34,
						44,
						34,
						98,
						117,
						115,
						95,
						105,
						110,
						102,
						111,
						34,
						58,
						34,
						115,
						99,
						115,
						105,
						64,
						52,
						58,
						48,
						46,
						48,
						46,
						48,
						34,
						44,
						34,
						99,
						97,
						112,
						97,
						99,
						105,
						116,
						121,
						95,
						98,
						121,
						116,
						101,
						115,
						34,
						58,
						52,
						56,
						48,
						49,
						48,
						51,
						57,
						56,
						49,
						48,
						53,
						54,
						125,
					},
					CreatedAt: time.Time{},
					UpdatedAt: time.Time{},
				},
			},
			VersionedAttributes: nil,
			ComponentTypeID:     "3717d747-3cc3-4800-822c-4c7a9ac2c314",
			ComponentTypeName:   "Drive",
			ComponentTypeSlug:   "drive",
			CreatedAt:           time.Time{},
			UpdatedAt:           time.Time{},
		},
		serverservice.ServerComponent{
			UUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			ServerUUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			Name:   "Drive",
			Vendor: "intel",
			Model:  "INTEL SSDSC2KB48",
			Serial: "PHYF001209KL480BGN",
			Attributes: []serverservice.Attributes{
				serverservice.Attributes{
					Namespace: "sh.hollow.alloy.outofband.metadata",
					Data: json.RawMessage{
						123,
						34,
						100,
						101,
						115,
						99,
						114,
						105,
						112,
						116,
						105,
						111,
						110,
						34,
						58,
						34,
						65,
						84,
						65,
						32,
						68,
						105,
						115,
						107,
						34,
						44,
						34,
						112,
						114,
						111,
						100,
						117,
						99,
						116,
						95,
						110,
						97,
						109,
						101,
						34,
						58,
						34,
						73,
						78,
						84,
						69,
						76,
						32,
						83,
						83,
						68,
						83,
						67,
						50,
						75,
						66,
						52,
						56,
						34,
						44,
						34,
						98,
						117,
						115,
						95,
						105,
						110,
						102,
						111,
						34,
						58,
						34,
						115,
						99,
						115,
						105,
						64,
						53,
						58,
						48,
						46,
						48,
						46,
						48,
						34,
						44,
						34,
						99,
						97,
						112,
						97,
						99,
						105,
						116,
						121,
						95,
						98,
						121,
						116,
						101,
						115,
						34,
						58,
						52,
						56,
						48,
						49,
						48,
						51,
						57,
						56,
						49,
						48,
						53,
						54,
						125,
					},
					CreatedAt: time.Time{},
					UpdatedAt: time.Time{},
				},
			},
			VersionedAttributes: nil,
			ComponentTypeID:     "3717d747-3cc3-4800-822c-4c7a9ac2c314",
			ComponentTypeName:   "Drive",
			ComponentTypeSlug:   "drive",
			CreatedAt:           time.Time{},
			UpdatedAt:           time.Time{},
		},
		serverservice.ServerComponent{
			UUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			ServerUUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			Name:   "CPU",
			Vendor: "intel",
			Model:  "Intel(R) Xeon(R) E-2278G CPU @ 3.40GHz",
			Serial: "0",
			Attributes: []serverservice.Attributes{
				serverservice.Attributes{
					Namespace: "sh.hollow.alloy.outofband.metadata",
					Data: json.RawMessage{
						123,
						34,
						100,
						101,
						115,
						99,
						114,
						105,
						112,
						116,
						105,
						111,
						110,
						34,
						58,
						34,
						67,
						80,
						85,
						34,
						44,
						34,
						112,
						114,
						111,
						100,
						117,
						99,
						116,
						95,
						110,
						97,
						109,
						101,
						34,
						58,
						34,
						73,
						110,
						116,
						101,
						108,
						40,
						82,
						41,
						32,
						88,
						101,
						111,
						110,
						40,
						82,
						41,
						32,
						69,
						45,
						50,
						50,
						55,
						56,
						71,
						32,
						67,
						80,
						85,
						32,
						64,
						32,
						51,
						46,
						52,
						48,
						71,
						72,
						122,
						34,
						44,
						34,
						115,
						108,
						111,
						116,
						34,
						58,
						34,
						67,
						80,
						85,
						49,
						34,
						44,
						34,
						99,
						108,
						111,
						99,
						107,
						95,
						115,
						112,
						101,
						101,
						100,
						95,
						104,
						122,
						34,
						58,
						49,
						48,
						48,
						48,
						48,
						48,
						48,
						48,
						48,
						44,
						34,
						99,
						111,
						114,
						101,
						115,
						34,
						58,
						56,
						44,
						34,
						116,
						104,
						114,
						101,
						97,
						100,
						115,
						34,
						58,
						49,
						54,
						125,
					},
					CreatedAt: time.Time{},
					UpdatedAt: time.Time{},
				},
			},
			VersionedAttributes: nil,
			ComponentTypeID:     "75fc736e-fe42-4495-8e62-02d46fd08528",
			ComponentTypeName:   "CPU",
			ComponentTypeSlug:   "cpu",
			CreatedAt:           time.Time{},
			UpdatedAt:           time.Time{},
		},
		serverservice.ServerComponent{
			UUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			ServerUUID: uuid.UUID{
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			},
			Name:   "StorageController",
			Vendor: "intel",
			Model:  "Cannon Lake PCH SATA AHCI Controller",
			Serial: "0",
			Attributes: []serverservice.Attributes{
				serverservice.Attributes{
					Namespace: "sh.hollow.alloy.outofband.metadata",
					Data: json.RawMessage{
						123,
						34,
						100,
						101,
						115,
						99,
						114,
						105,
						112,
						116,
						105,
						111,
						110,
						34,
						58,
						34,
						83,
						65,
						84,
						65,
						32,
						99,
						111,
						110,
						116,
						114,
						111,
						108,
						108,
						101,
						114,
						34,
						44,
						34,
						112,
						114,
						111,
						100,
						117,
						99,
						116,
						95,
						110,
						97,
						109,
						101,
						34,
						58,
						34,
						67,
						97,
						110,
						110,
						111,
						110,
						32,
						76,
						97,
						107,
						101,
						32,
						80,
						67,
						72,
						32,
						83,
						65,
						84,
						65,
						32,
						65,
						72,
						67,
						73,
						32,
						67,
						111,
						110,
						116,
						114,
						111,
						108,
						108,
						101,
						114,
						34,
						44,
						34,
						115,
						117,
						112,
						112,
						111,
						114,
						116,
						101,
						100,
						95,
						100,
						101,
						118,
						105,
						99,
						101,
						95,
						112,
						114,
						111,
						116,
						111,
						99,
						111,
						108,
						34,
						58,
						34,
						83,
						65,
						84,
						65,
						34,
						44,
						34,
						112,
						104,
						121,
						115,
						105,
						100,
						34,
						58,
						34,
						49,
						55,
						34,
						44,
						34,
						98,
						117,
						115,
						95,
						105,
						110,
						102,
						111,
						34,
						58,
						34,
						112,
						99,
						105,
						64,
						48,
						48,
						48,
						48,
						58,
						48,
						48,
						58,
						49,
						55,
						46,
						48,
						34,
						125,
					},
					CreatedAt: time.Time{},
					UpdatedAt: time.Time{},
				},
			},
			VersionedAttributes: nil,
			ComponentTypeID:     "ef563926-8011-4985-bc4a-7ed7e9933971",
			ComponentTypeName:   "StorageController",
			ComponentTypeSlug:   "storagecontroller",
			CreatedAt:           time.Time{},
			UpdatedAt:           time.Time{},
		},
	}
)
