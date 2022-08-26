package publish

import (
	"testing"

	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/fixtures"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

func Test_ToComponentSlice(t *testing.T) {
	h := serverServicePublisher{
		logger: logrus.NewEntry(logrus.New()),
		slugs:  fixtures.ServerServiceSlugMap(),
	}

	testcases := []struct {
		name     string
		device   *model.Asset
		expected []*serverservice.ServerComponent
	}{
		{
			"E3C246D4INL",
			&model.Asset{Inventory: fixtures.CopyDevice(fixtures.E3C246D4INL)},
			componentPtrSlice(fixtures.ServerServiceE3C246D4INLcomponents),
		},
		{
			"R6515_A",
			&model.Asset{Inventory: fixtures.CopyDevice(fixtures.R6515_f0c8e4ac)},
			componentPtrSlice(fixtures.ServerServiceR6515Components_f0c8e4ac),
		},
		{
			"R6515_B",
			&model.Asset{Inventory: fixtures.CopyDevice(fixtures.R6515_fc167440)},
			componentPtrSlice(fixtures.ServerServiceR6515Components_fc167440),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			sc, err := h.toComponentSlice(uuid.Nil, tc.device)
			if err != nil {
				t.Fatal(err)
			}

			// zero component type IDs, serverIDs
			for idx := range sc {
				sc[idx].ComponentTypeID = ""
				sc[idx].ServerUUID = uuid.Nil
			}

			for idx := range tc.expected {
				tc.expected[idx].ComponentTypeID = ""
				tc.expected[idx].ServerUUID = uuid.Nil
			}

			//
			// left commented out here for future reference
			//
			// filterFunc := func(f reflect.StructField, v reflect.Value) bool {
			// 	switch f.Name {
			// 	case "ServerUUID", "UUID", "CreatedAt", "UpdatedAt", "LastReportedAt":
			// 		return false
			// 	default:
			// 		return true
			// 	}
			// }
			// l := litter.Options{FieldFilter: filterFunc}
			// l.Dump(sc)

			assert.Equal(t, tc.expected, sc)
		})
	}
}
