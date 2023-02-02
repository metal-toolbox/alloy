package publish

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/fixtures"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

func Test_ToComponentSlice(t *testing.T) {
	handler := http.NewServeMux()

	// get firmwares query
	handler.HandleFunc(
		"/api/v1/server-component-firmwares",
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")

				_, _ = w.Write([]byte(`{}`))
			default:
				t.Fatal("expected GET request, got: " + r.Method)
			}
		},
	)

	mock := httptest.NewServer(handler)
	p := testPublisherInstance(t, mock.URL)
	p.logger = logrus.NewEntry(logrus.New())
	p.slugs = fixtures.ServerServiceSlugMap()
	p.attributeNS = model.ServerComponentAttributeNS(model.AppKindOutOfBand)
	p.versionedAttributeNS = model.ServerComponentVersionedAttributeNS(model.AppKindOutOfBand)

	testcases := []struct {
		name     string
		device   *model.Asset
		expected []*serverservice.ServerComponent
	}{
		{
			"E3C246D4INL",
			&model.Asset{Vendor: "asrockrack", Inventory: fixtures.CopyDevice(fixtures.E3C246D4INL)},
			componentPtrSlice(fixtures.ServerServiceE3C246D4INLcomponents),
		},
		{
			"R6515_A",
			&model.Asset{Vendor: "dell", Inventory: fixtures.CopyDevice(fixtures.R6515_f0c8e4ac)},
			componentPtrSlice(fixtures.ServerServiceR6515Components_f0c8e4ac),
		},
		{
			"R6515_B",
			&model.Asset{Vendor: "dell", Inventory: fixtures.CopyDevice(fixtures.R6515_fc167440)},
			componentPtrSlice(fixtures.ServerServiceR6515Components_fc167440),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			sc, err := p.toComponentSlice(uuid.Nil, tc.device)
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
