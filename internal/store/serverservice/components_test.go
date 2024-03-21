package serverservice

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/fixtures"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/sanity-io/litter"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	serverserviceapi "go.hollow.sh/serverservice/pkg/api/v1"
)

// To refresh the fixtures used below, set dumpFixtures to true and run the test.
// copy over the slice elements from the dumped object slice into alloy/internal/fixtures/serverservice_components_$deviceModel.go
//
// The object slice elements copied over will need to be changed to be of serverservice.ServerComponent
func Test_ToComponentSlice(t *testing.T) {
	dumpFixture := false

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
	p := testStoreInstance(t, mock.URL)
	p.logger = logrus.New()
	p.slugs = fixtures.ServerServiceSlugMap()
	p.attributeNS = serverComponentAttributeNS(model.AppKindOutOfBand)
	p.firmwareVersionedAttributeNS = serverComponentFirmwareNS(model.AppKindOutOfBand)
	p.statusVersionedAttributeNS = serverComponentStatusNS(model.AppKindOutOfBand)

	testcases := []struct {
		name     string
		device   *model.Asset
		expected []*serverserviceapi.ServerComponent
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

			if dumpFixture {
				filterFunc := func(f reflect.StructField, _ reflect.Value) bool {
					switch f.Name {
					case "ServerUUID", "UUID", "CreatedAt", "UpdatedAt", "LastReportedAt":
						return false
					default:
						return true
					}
				}
				l := litter.Options{FieldFilter: filterFunc}
				l.Dump(sc)
			}

			assert.Equal(t, tc.expected, sc)
		})
	}
}
