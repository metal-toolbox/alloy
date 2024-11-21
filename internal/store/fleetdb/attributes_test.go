package fleetdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	common "github.com/metal-toolbox/bmc-common"
	fleetdbapi "github.com/metal-toolbox/fleetdb/pkg/api/v1"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metal-toolbox/alloy/internal/fixtures"
	"github.com/metal-toolbox/alloy/internal/model"
)

func testStoreInstance(t *testing.T, mockURL string) *Store {
	t.Helper()

	cr := retryablehttp.NewClient()
	cr.RetryMax = 1

	// comment out to enable debug logs
	cr.Logger = nil

	mockClient, err := fleetdbapi.NewClientWithToken(
		"hunter2",
		mockURL,
		cr.StandardClient(),
	)

	if err != nil {
		t.Fatal(err)
	}

	return &Store{
		logger:                       logrus.New(),
		slugs:                        fixtures.FleetDBSlugMap(),
		Client:                       mockClient,
		attributeNS:                  serverComponentAttributeNS(model.AppKindOutOfBand),
		firmwareVersionedAttributeNS: serverComponentFirmwareNS(model.AppKindOutOfBand),
		statusVersionedAttributeNS:   serverComponentStatusNS(model.AppKindOutOfBand),
	}
}

func Test_DiffVersionedAttributes(t *testing.T) {
	now := time.Now()

	// current versioned attributes fixture for data read from serverService
	fixtureCurrentVA := []fleetdbapi.VersionedAttributes{
		{
			Namespace: "server.components",
			Data:      []byte(`{"firmware":{"installed":"2.2.5","software_id":"159"}`),
			CreatedAt: now.Add(-24 * time.Hour), // 24 hours earlier
		},
		{
			Namespace: "server.components",
			Data:      []byte(`{"firmware":{"installed":"2.2.4","software_id":"159"}`),
			CreatedAt: now.Add(-48 * time.Hour), // 48 hours earlier
		},
	}

	// new versioned attributes fixture for data read from the BMC
	fixtureNewVA := []fleetdbapi.VersionedAttributes{
		{
			Namespace: "server.components",
			Data:      []byte(`{"firmware":{"installed":"2.2.6","software_id":"159"}`),
			CreatedAt: now,
		},
	}

	// current versioned attribute fixture which includes data from newer, unsorted
	fixtureCurrentWithNewerVA := []fleetdbapi.VersionedAttributes{
		fixtureCurrentVA[0],
		fixtureCurrentVA[1],
		fixtureNewVA[0],
	}

	testcases := []struct {
		name        string
		expectedErr error
		expectedObj *fleetdbapi.VersionedAttributes
		currentObjs []fleetdbapi.VersionedAttributes
		newObjs     []fleetdbapi.VersionedAttributes
	}{
		{
			"with no new versioned objects, the method returns nil",
			nil,
			nil,
			fixtureCurrentVA,
			[]fleetdbapi.VersionedAttributes{},
		},
		{
			"with no new versioned objects, and no current versioned objects the method returns nil",
			nil,
			nil,
			[]fleetdbapi.VersionedAttributes{},
			[]fleetdbapi.VersionedAttributes{},
		},
		{
			"with an empty current versioned attribute object, the method returns the newer object",
			nil,
			&fixtureNewVA[0],
			[]fleetdbapi.VersionedAttributes{},
			fixtureNewVA,
		},
		{
			"latest current versioned attribute is compared with the newer, newer is returend",
			nil,
			&fixtureNewVA[0],
			fixtureCurrentVA,
			fixtureNewVA,
		},
		{
			"latest current versioned attribute is equal to newer, nil is returned",
			nil,
			nil,
			fixtureCurrentWithNewerVA,
			fixtureNewVA,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := diffVersionedAttributes(tc.currentObjs, tc.newObjs)
			if tc.expectedErr != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.expectedErr, err)
				return
			}

			assert.Equal(t, tc.expectedObj, v)
		})
	}
}

// addVA
func addcomponent(sc []*fleetdbapi.ServerComponent, t *testing.T, slug string, va *firmwareVersionedAttribute) []*fleetdbapi.ServerComponent {
	t.Helper()

	data, err := json.Marshal(va)
	if err != nil {
		t.Error(err)
	}

	component := &fleetdbapi.ServerComponent{
		UUID:   uuid.New(),
		Name:   slug,
		Vendor: "",
		VersionedAttributes: []fleetdbapi.VersionedAttributes{
			{
				Data:      data,
				Namespace: "foo.bar",
			},
		},
	}

	sc = append(sc, component)

	return sc
}

func updateComponentVA(sc []*fleetdbapi.ServerComponent, t *testing.T, slug string, va *firmwareVersionedAttribute) []*fleetdbapi.ServerComponent {
	t.Helper()

	var component *fleetdbapi.ServerComponent

	for _, c := range sc {
		if strings.EqualFold(c.ComponentTypeSlug, strings.ToLower(slug)) {
			component = c
			break
		}
	}

	if component == nil {
		t.Fatal("component with slug not found:" + slug)
	}

	newVA := newFirmwareVA(t, component.VersionedAttributes[0].Data, va.Firmware.Installed)

	newVAData, err := json.Marshal(newVA)
	if err != nil {
		t.Fatal(err)
	}

	component.VersionedAttributes[0].Data = newVAData

	return sc
}

func newFirmwareVA(t *testing.T, data json.RawMessage, value string) *firmwareVersionedAttribute {
	t.Helper()

	va := &firmwareVersionedAttribute{}

	err := json.Unmarshal(data, va)
	if err != nil {
		t.Fatal(err)
	}

	va.Firmware.Installed = value

	return va
}

func Test_filterByAttributeNamespace(t *testing.T) {
	components := componentPtrSlice(
		fixtures.CopyFleetDBComponentSlice(
			fixtures.FleetDBAPIR6515Components_fc167440,
		),
	)

	// the fixture is expected to contain atleast 2 components with 1 attribute and 2 versioned attributes (status and firmware)
	assert.Equal(t, 1, len(components[0].Attributes))
	assert.Equal(t, 1, len(components[0].VersionedAttributes))
	assert.Equal(t, 1, len(components[1].Attributes))
	assert.Equal(t, 2, len(components[1].VersionedAttributes))

	// change namespace on component[1] (bios) attributes so the component is filtered
	components[1].Attributes[0].Namespace = "some.ns"

	// change namespace on component[0] (bmc) versioned attributes so the component is filtered
	components[0].VersionedAttributes[0].Namespace = "some.ns"

	// init publisher
	p := testStoreInstance(t, "foobar")

	// run method under test
	p.filterByAttributeNamespace(components)

	// expect component with set namepace to be included
	assert.Equal(t, 1, len(components[0].Attributes))
	assert.Equal(t, 2, len(components[1].VersionedAttributes))

	// components with unexpected namespaces are excluded
	assert.Equal(t, 0, len(components[1].Attributes))
	assert.Equal(t, 0, len(components[0].VersionedAttributes))
}

func Test_FleetDB_CreateUpdateServerComponents_ObjectsEqual(t *testing.T) {
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)
	handler := http.NewServeMux()

	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/components", serverID.String()),
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(fixtures.FleetDBAPIR6515Components_fc167440_JSON())
			default:
				t.Fatal("expected GET request, got: " + r.Method)
			}
		},
	)

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

	device := &model.Asset{ID: serverID.String(), Vendor: "dell", Inventory: fixtures.CopyDevice(fixtures.R6515_fc167440)}

	err := p.createUpdateServerComponents(context.TODO(), serverID, device)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_FleetDB_CreateUpdateServerComponents_ObjectsUpdated(t *testing.T) {
	// comment left here for future reference
	//
	// os.Setenv(model.EnvVarDumpDiffers, "true")
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)
	newBIOSFWVersion := "2.6.7"

	newBMCFWVersion := "5.12.00.00"

	handler := http.NewServeMux()

	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/components", serverID.String()),
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")

				_, _ = w.Write(fixtures.FleetDBAPIR6515Components_fc167440_JSON())
			case http.MethodPut:
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				gotUpdate := []*fleetdbapi.ServerComponent{}
				if err := json.Unmarshal(b, &gotUpdate); err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, 2, len(gotUpdate))

				gotObj := componentBySlugSerial(common.SlugBIOS, "0", gotUpdate)
				assertComponentAttributes(t, gotObj, newBIOSFWVersion)

				gotObj = componentBySlugSerial(common.SlugBMC, "0", gotUpdate)
				assertComponentAttributes(t, gotObj, newBMCFWVersion)

				_, _ = w.Write([]byte(`{}`))
			default:
				t.Fatal("expected GET request, got: " + r.Method)
			}
		},
	)

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

	// asset device fixture returned by the inventory collector
	device := &model.Asset{
		ID:        serverID.String(),
		Vendor:    "dell",
		Inventory: fixtures.CopyDevice(fixtures.R6515_fc167440),
	}

	// bump version on BIOS and BMC components
	device.Inventory.BIOS.Firmware.Installed = newBIOSFWVersion
	device.Inventory.BMC.Firmware.Installed = newBMCFWVersion

	err := p.createUpdateServerComponents(context.TODO(), serverID, device)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_FleetDB_CreateUpdateServerComponents_ObjectsAdded(t *testing.T) {
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

	fixtureNICSerial := "c00l"

	handler := http.NewServeMux()
	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/components", serverID.String()),
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(fixtures.FleetDBAPIR6515Components_fc167440_JSON())
			case http.MethodPost:
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				gotAdded := []*fleetdbapi.ServerComponent{}
				if err := json.Unmarshal(b, &gotAdded); err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, 1, len(gotAdded))

				gotObj := componentBySlugSerial(common.SlugNIC, fixtureNICSerial, gotAdded)
				assert.NotNil(t, gotObj)

				_, _ = w.Write([]byte(`{}`))
			default:
				t.Fatal("expected GET request, got: " + r.Method)
			}
		},
	)

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

	// asset device fixture returned by the inventory collector
	device := &model.Asset{
		ID:        serverID.String(),
		Vendor:    "dell",
		Inventory: fixtures.CopyDevice(fixtures.R6515_fc167440),
	}

	device.Inventory.NICs = append(
		device.Inventory.NICs,
		&common.NIC{
			ID: "NEW NIC!",
			Common: common.Common{
				Vendor:      "noname",
				Model:       "noname",
				Serial:      fixtureNICSerial,
				Description: "Just added!, totally incompatible",
			},
		},
	)

	mock := httptest.NewServer(handler)
	p := testStoreInstance(t, mock.URL)

	err := p.createUpdateServerComponents(context.TODO(), serverID, device)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_FleetDB_CreateUpdateServerAttributes_Create(t *testing.T) {
	// test: createUpdateServerAttributes creates server attributes when its undefined in server service
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

	server := &fleetdbapi.Server{UUID: serverID}
	// the device with model, vendor, serial as unknown in server service
	// with inventory from the device with the actual model, vendor, serial attributes
	device := &model.Asset{
		Model:  "unknown",
		Vendor: "unknown",
		Serial: "unknown",
		ID:     serverID.String(),
		Inventory: &common.Device{
			Common: common.Common{
				Model:  "foobar",
				Vendor: "test",
				Serial: "lala",
			},
		},
	}

	handler := http.NewServeMux()
	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/attributes", serverID.String()),
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				// unpack attributes posted by method
				attributes := &fleetdbapi.Attributes{}
				if err = json.Unmarshal(b, attributes); err != nil {
					t.Fatal(err)
				}

				// asset NS is as expected
				assert.Equal(t, serverVendorAttributeNS, attributes.Namespace)

				// unpack attributes data
				data := map[string]string{}
				if err = json.Unmarshal(attributes.Data, &data); err != nil {
					t.Fatal(err)
				}

				// asset attributes data matches device attributes
				assert.Equal(t, device.Inventory.Model, data[serverModelAttributeKey])
				assert.Equal(t, device.Inventory.Serial, data[serverSerialAttributeKey])
				assert.Equal(t, device.Inventory.Vendor, data[serverVendorAttributeKey])

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(fixtures.FleetDBAPIR6515Components_fc167440_JSON())
			default:
				t.Fatal("expected POST request, got: " + r.Method)
			}
		},
	)

	mock := httptest.NewServer(handler)
	p := testStoreInstance(t, mock.URL)

	err := p.createUpdateServerAttributes(context.TODO(), server, device)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_FleetDB_CreateUpdateServerAttributes_Update(t *testing.T) {
	// test: createUpdateServerAttributes updates server attributes when either of them are missing
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

	// vendor attribute data
	m := map[string]string{
		serverSerialAttributeKey: "unknown",
		serverVendorAttributeKey: "unknown",
		serverModelAttributeKey:  "unknown",
	}

	d, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	server := &fleetdbapi.Server{
		UUID: serverID,
		Attributes: []fleetdbapi.Attributes{
			{
				Namespace: serverVendorAttributeNS,
				Data:      d,
			},
		},
	}

	// the device with model, vendor, serial as unknown in server service
	// with inventory from the device with the actual model, vendor, serial attributes
	device := &model.Asset{
		Model:  "unknown",
		Vendor: "test",
		Serial: "unknown",
		ID:     serverID.String(),
		Inventory: &common.Device{
			Common: common.Common{
				Model:  "foobar",
				Vendor: "test",
				Serial: "lala",
			},
		},
	}

	handler := http.NewServeMux()
	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/attributes/%s", serverID.String(), serverVendorAttributeNS),
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				var b []byte
				b, err = io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				// unpack attributes posted by method
				attributes := &fleetdbapi.Attributes{}
				if err = json.Unmarshal(b, attributes); err != nil {
					t.Fatal(err)
				}

				// unpack attributes data
				data := map[string]string{}
				if err = json.Unmarshal(attributes.Data, &data); err != nil {
					t.Fatal(err)
				}

				// asset attributes data matches device attributes
				assert.Equal(t, device.Inventory.Model, data[serverModelAttributeKey])
				assert.Equal(t, device.Inventory.Serial, data[serverSerialAttributeKey])
				assert.Equal(t, device.Inventory.Vendor, data[serverVendorAttributeKey])

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(fixtures.FleetDBAPIR6515Components_fc167440_JSON())
			default:
				t.Fatal("expected PUT request, got: " + r.Method)
			}
		},
	)

	mock := httptest.NewServer(handler)
	p := testStoreInstance(t, mock.URL)

	if err = p.createUpdateServerAttributes(context.TODO(), server, device); err != nil {
		t.Fatal(err)
	}
}

func Test_FleetDB_CreateUpdateServerMetadataAttributes_Create(t *testing.T) {
	// test: createUpdateServerMetadataAttributes creats server metadata attributes when its undefined in server service.
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

	device := &model.Asset{
		Metadata: map[string]string{},
		Vendor:   "foobar",
		Inventory: &common.Device{
			Common: common.Common{
				Metadata: map[string]string{"foo": "bar"},
			},
		},
	}

	handler := http.NewServeMux()
	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/attributes", serverID.String()),
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				// unpack attributes posted by method
				attributes := &fleetdbapi.Attributes{}
				if err = json.Unmarshal(b, attributes); err != nil {
					t.Fatal(err)
				}

				// asset NS is as expected
				assert.Equal(t, serverMetadataAttributeNS, attributes.Namespace)

				// unpack attributes data
				data := map[string]string{}
				if err = json.Unmarshal(attributes.Data, &data); err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, device.Inventory.Metadata, data)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(fixtures.FleetDBAPIR6515Components_fc167440_JSON())
			default:
				t.Fatal("expected POST request, got: " + r.Method)
			}
		},
	)

	mock := httptest.NewServer(handler)
	p := testStoreInstance(t, mock.URL)

	err := p.createUpdateServerMetadataAttributes(context.TODO(), serverID, device)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_FleetDB_CreateUpdateServerMetadataAttributes_Update(t *testing.T) {
	// test: createUpdateServerMetadataAttributes updates server metadata attributes when it differs.
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

	device := &model.Asset{
		Metadata: map[string]string{
			"foo":        "bar",
			"__ss_found": "true",
		},
		Vendor: "foobar",
		Inventory: &common.Device{
			Common: common.Common{
				Metadata: map[string]string{
					"foo":  "bar",
					"test": "lala",
				},
			},
		},
	}

	handler := http.NewServeMux()
	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/attributes/%s", serverID.String(), serverMetadataAttributeNS),
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				b, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				// unpack attributes posted by method
				attributes := &fleetdbapi.Attributes{}
				err = json.Unmarshal(b, attributes)
				require.NoError(t, err)

				// unpack attributes data
				data := map[string]string{}
				err = json.Unmarshal(attributes.Data, &data)
				require.NoError(t, err)

				require.Equal(t, device.Inventory.Metadata, data)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(fixtures.FleetDBAPIR6515Components_fc167440_JSON())
			default:
				t.Fatal("expected PUT request, got: " + r.Method)
			}
		},
	)

	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Logf("unhandled URL: %s", r.URL)
		w.WriteHeader(404)
	})

	mock := httptest.NewServer(handler)
	t.Logf("mock URL: %s", mock.URL)
	p := testStoreInstance(t, mock.URL)

	err := p.createUpdateServerMetadataAttributes(context.TODO(), serverID, device)
	require.NoError(t, err)
}

func Test_FleetDB_CreateUpdateServerBMCErrorAttributes_NoErrorsNoChanges(t *testing.T) {
	// tests - no errors were reported by the collection, nor are there any currently registered in server service
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

	handler := http.NewServeMux()

	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/attributes/%s", serverID.String(), serverBMCErrorsAttributeNS),
		func(_ http.ResponseWriter, r *http.Request) {
			t.Fatal("expected no request, got: " + r.Method)
		},
	)

	mock := httptest.NewServer(handler)
	p := testStoreInstance(t, mock.URL)

	err := p.createUpdateServerBMCErrorAttributes(context.TODO(), serverID, nil, &model.Asset{})
	if err != nil {
		t.Fatal(err)
	}
}

func Test_FleetDB_CreateUpdateServerBMCErrorAttributes_HasErrorsNoChanges(t *testing.T) {
	// tests - errors were reported by the collection, the same errors are currently registered.
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

	handler := http.NewServeMux()

	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/attributes/%s", serverID.String(), serverBMCErrorsAttributeNS),
		func(_ http.ResponseWriter, r *http.Request) {
			t.Fatal("expected no request, got: " + r.Method)
		},
	)

	mock := httptest.NewServer(handler)
	p := testStoreInstance(t, mock.URL)

	errs := []byte(`{"login_error": "bmc gave up"}`)
	errAttribs := &fleetdbapi.Attributes{Data: errs}

	asset := &model.Asset{Errors: map[string]string{"login_error": "bmc gave up"}}

	err := p.createUpdateServerBMCErrorAttributes(context.TODO(), serverID, errAttribs, asset)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_FleetDB_CreateUpdateServerBMCErrorAttributes_RegisteredErrorsPurged(t *testing.T) {
	// tests - no errors were reported by the collection, although there are error registered in server service for the server,
	// the registered error is then purged.
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

	handler := http.NewServeMux()

	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/attributes/%s", serverID.String(), serverBMCErrorsAttributeNS),
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				// unpack attributes posted by method
				attributes := &fleetdbapi.Attributes{}
				if err = json.Unmarshal(b, attributes); err != nil {
					t.Fatal(err)
				}

				// unpack attributes data
				data := map[string]string{}
				if err = json.Unmarshal(attributes.Data, &data); err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, 0, len(data))

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(fixtures.FleetDBAPIR6515Components_fc167440_JSON())
			default:
				t.Fatal("expected PUT request, got: " + r.Method)
			}
		},
	)

	mock := httptest.NewServer(handler)
	p := testStoreInstance(t, mock.URL)

	errAttribs := &fleetdbapi.Attributes{Data: []byte(`{"login_error": "bmc gave up"}`)}

	err := p.createUpdateServerBMCErrorAttributes(context.TODO(), serverID, errAttribs, &model.Asset{})
	if err != nil {
		t.Fatal(err)
	}
}

func Test_FleetDB_CreateUpdateServerBMCErrorAttributes_Create(t *testing.T) {
	// test: createUpdateServerMetadataAttributes updates server metadata attributes when it differs.
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

	handler := http.NewServeMux()

	device := &model.Asset{Errors: map[string]string{"login_error": "password was not hunter2"}}

	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/attributes", serverID.String()),
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				// the response here is
				resp, err := os.ReadFile("../../fixtures/fleetdb_server_fc167440.json")
				if err != nil {
					t.Fatal(err)
				}

				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				// unpack attributes posted by method
				attributes := &fleetdbapi.Attributes{}
				if err = json.Unmarshal(b, attributes); err != nil {
					t.Fatal(err)
				}

				// unpack attributes data
				data := map[string]string{}
				if err = json.Unmarshal(attributes.Data, &data); err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, device.Errors, data)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			default:
				t.Fatal("expected POST request, got: " + r.Method)
			}
		},
	)

	mock := httptest.NewServer(handler)
	p := testStoreInstance(t, mock.URL)

	err := p.createUpdateServerBMCErrorAttributes(context.TODO(), serverID, nil, device)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_FleetDB_CreateUpdateServerBMCErrorAttributes_Updated(t *testing.T) {
	// tests - errors were reported by the collection, there are error registered in server service for the server, update
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

	handler := http.NewServeMux()

	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/attributes/%s", serverID.String(), serverBMCErrorsAttributeNS),
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				// unpack attributes posted by method
				attributes := &fleetdbapi.Attributes{}
				if err = json.Unmarshal(b, attributes); err != nil {
					t.Fatal(err)
				}

				// unpack attributes data
				data := map[string]string{}
				if err = json.Unmarshal(attributes.Data, &data); err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, map[string]string{"login_error": "bmc on vacation"}, data)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(fixtures.FleetDBAPIR6515Components_fc167440_JSON())
			default:
				t.Fatal("expected PUT request, got: " + r.Method)
			}
		},
	)

	mock := httptest.NewServer(handler)
	p := testStoreInstance(t, mock.URL)

	errs := []byte(`{"login_error": "bmc gave up"}`)
	errAttribs := &fleetdbapi.Attributes{Data: errs}

	asset := &model.Asset{Errors: map[string]string{"login_error": "bmc on vacation"}}

	err := p.createUpdateServerBMCErrorAttributes(context.TODO(), serverID, errAttribs, asset)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMetadataFilter(t *testing.T) {
	t.Parallel()
	clean := map[string]string{
		"foo": "bar",
		"baz": "quux",
	}

	dirty := map[string]string{
		"foo":            "bar",
		"baz":            "quux",
		"uefi-variables": "uhoh-nogood",
	}

	exp, err := json.Marshal(clean)
	require.NoError(t, err, "prerequisite setup")

	got := mustFilterAssetMetadata(clean)
	require.Equal(t, json.RawMessage(exp), got, "clean doesn't serialize properly")

	got = mustFilterAssetMetadata(dirty)
	require.Equal(t, json.RawMessage(exp), got, "dirty doesn't serialize properly")
}

func TestDeviceVendorData(t *testing.T) {
	testcases := []struct {
		name     string
		asset    *model.Asset
		expected map[string]string
	}{
		{
			"default values",
			&model.Asset{
				ID: uuid.NewString(),
				Inventory: &common.Device{
					Common: common.Common{},
				},
			},
			map[string]string{
				serverSerialAttributeKey: "unknown",
				serverVendorAttributeKey: "unknown",
				serverModelAttributeKey:  "unknown",
			},
		},
		{
			"expected values set",
			&model.Asset{
				ID: uuid.NewString(),
				Inventory: &common.Device{
					Common: common.Common{
						Vendor: "foobar",
						Model:  "baz",
						Serial: "00123",
					},
				},
			},
			map[string]string{
				serverVendorAttributeKey: "foobar",
				serverModelAttributeKey:  "baz",
				serverSerialAttributeKey: "00123",
			},
		},
		{
			"model attribute formatted",
			&model.Asset{
				ID: uuid.NewString(),
				Inventory: &common.Device{
					Common: common.Common{
						Vendor: "foobar",
						Model:  "PIO-519C-MR-PH004",
						Serial: "00123",
					},
				},
			},
			map[string]string{
				serverVendorAttributeKey: "foobar",
				serverModelAttributeKey:  "x11sch-f",
				serverSerialAttributeKey: "00123",
			},
		},
	}

	s := &Store{}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got := s.deviceVendorData(tc.asset)
			assert.Equal(t, tc.expected, got, tc.name)
		})
	}
}

func TestVendorDataUpdate(t *testing.T) {
	tests := []struct {
		name        string
		newData     map[string]string
		currentData map[string]string
		expected    map[string]string
	}{
		{
			name:        "updates when currentData is nil",
			newData:     map[string]string{"vendor": "foo", "model": "bar"},
			currentData: nil,
			expected:    map[string]string{"vendor": "foo", "model": "bar"},
		},
		{
			name:        "updates model when current model is unknown",
			newData:     map[string]string{"model": "foo"},
			currentData: map[string]string{"model": "unknown"},
			expected:    map[string]string{"model": "foo"},
		},
		{
			name:        "updates serial when current serial is unknown",
			newData:     map[string]string{"model": "foo", "serial": "123"},
			currentData: map[string]string{"model": "foo", "serial": "unknown"},
			expected:    map[string]string{"model": "foo", "serial": "123"},
		},
		{
			name:        "updates serial when current serial is empty",
			newData:     map[string]string{"model": "foo", "serial": "123"},
			currentData: map[string]string{"model": "foo", "serial": ""},
			expected:    map[string]string{"model": "foo", "serial": "123"},
		},
		{
			name:        "no update when new model matches current",
			newData:     map[string]string{"model": "r6515"},
			currentData: map[string]string{"model": "r6515"},
			expected:    nil,
		},
		{
			name:        "no update when new vendor matches current",
			newData:     map[string]string{"vendor": "r6515"},
			currentData: map[string]string{"vendor": "baz"},
			expected:    nil,
		},
		{
			name:        "no update when serial matches current",
			newData:     map[string]string{"model": "foo", "serial": "123"},
			currentData: map[string]string{"model": "foo", "serial": "123"},
			expected:    nil,
		},
		{
			name:        "update model when current does not match new",
			newData:     map[string]string{"vendor": "bar", "model": "newfoo"},
			currentData: map[string]string{"vendor": "baz", "model": "oldfoo"},
			expected:    map[string]string{"vendor": "baz", "model": "newfoo"},
		},
		{
			name:        "no update when new model is formatted and matches current",
			newData:     map[string]string{"vendor": "bar", "model": "PIO-519C-MR-PH004"},
			currentData: map[string]string{"vendor": "baz", "model": "x11sch-f"},
			expected:    nil,
		},
		{
			name:        "update when new model is formatted and does not match current",
			newData:     map[string]string{"vendor": "bar", "model": "PIO-519C-MR-PH004"},
			currentData: map[string]string{"vendor": "baz", "model": "PIO-519C-MR-PH004"},
			expected:    map[string]string{"vendor": "baz", "model": "x11sch-f"},
		},
		{
			name:        "ignore 'unknown' in new values",
			newData:     map[string]string{"model": "unknown", "vendor": "unknown"},
			currentData: map[string]string{"model": "foo", "vendor": "bar"},
			expected:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := vendorDataUpdate(tc.newData, tc.currentData)
			if tc.expected != nil {
				assert.Equal(t, tc.expected, got, tc.name)
			} else {
				assert.Nil(t, got, tc.name)
			}
		})
	}
}
