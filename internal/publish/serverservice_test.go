package publish

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

	"github.com/bmc-toolbox/common"
	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/fixtures"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

func assertComponentAttributes(t *testing.T, obj *serverservice.ServerComponent, expectedVersion string) {
	t.Helper()

	assert.NotNil(t, obj)
	assert.NotNil(t, obj.ServerUUID)
	assert.NotNil(t, obj.UUID)
	assert.NotNil(t, obj.ComponentTypeSlug)
	assert.NotEmpty(t, obj.VersionedAttributes[0].Data)
	assert.True(t, rawVersionAttributeFirmwareEquals(t, expectedVersion, obj.VersionedAttributes[0].Data))
}

// rawVersionAttributeKVEquals returns a bool value when the given key and value is equal
func rawVersionAttributeFirmwareEquals(t *testing.T, expectedVersion string, rawVA []byte) bool {
	t.Helper()

	va := &versionedAttributes{}

	err := json.Unmarshal(rawVA, va)
	if err != nil {
		t.Fatal(err)
	}

	return va.Firmware.Installed == expectedVersion
}

func Test_ServerServiceChangeList(t *testing.T) {
	components := fixtures.CopyServerServiceComponentSlice(fixtures.ServerServiceR6515Components_fc167440)

	// nolint:govet // struct alignment kept for readability
	testcases := []struct {
		name            string // test name
		current         []*serverservice.ServerComponent
		expectedUpdate  int
		expectedAdd     int
		expectedRemove  int
		slug            string // the component slug
		vaUpdates       *versionedAttributes
		aUpdates        *attributes
		addComponent    bool // adds a new component into the new slice before comparison
		removeComponent bool // removes a component from the new slice
	}{
		{
			"no changes in component lists",
			componentPtrSlice(fixtures.CopyServerServiceComponentSlice(components)),
			0,
			0,
			0,
			"",
			nil,
			nil,
			false,
			false,
		},
		{
			"updated component part of update slice",
			componentPtrSlice(fixtures.CopyServerServiceComponentSlice(components)),
			1,
			0,
			0,
			common.SlugBIOS,
			&versionedAttributes{Firmware: &common.Firmware{Installed: "2.2.6"}},
			nil,
			false,
			false,
		},
		{
			"added component part of add slice",
			componentPtrSlice(fixtures.CopyServerServiceComponentSlice(components)),
			0,
			1,
			0,
			common.SlugNIC,
			&versionedAttributes{Firmware: &common.Firmware{Installed: "1.3.3"}},
			nil,
			true,
			false,
		},
		{
			"component removed from slice",
			componentPtrSlice(fixtures.CopyServerServiceComponentSlice(components)),
			0,
			0,
			1,
			"",
			nil,
			nil,
			false,
			true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			newObjs := componentPtrSlice(fixtures.CopyServerServiceComponentSlice(fixtures.ServerServiceR6515Components_fc167440))

			switch {
			case tc.expectedAdd > 0:
				newObjs = addcomponent(newObjs, t, tc.slug, tc.vaUpdates)
			case tc.expectedUpdate > 0:
				newObjs = updateComponentVA(newObjs, t, tc.slug, tc.vaUpdates)
			case tc.expectedRemove > 0:
				newObjs = newObjs[:len(newObjs)-1]
			default:
			}

			gotAdd, gotUpdate, gotRemove, err := serverServiceChangeList(context.TODO(), tc.current, newObjs)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, tc.expectedAdd, len(gotAdd), "add list differs")
			assert.Equal(t, tc.expectedUpdate, len(gotUpdate), "update list differs")
			assert.Equal(t, tc.expectedRemove, len(gotRemove), "remove list differs")
		})
	}
}

func Test_DiffVersionedAttributes(t *testing.T) {
	now := time.Now()

	// current versioned attributes fixture for data read from serverService
	fixtureCurrentVA := []serverservice.VersionedAttributes{
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
	fixtureNewVA := []serverservice.VersionedAttributes{
		{
			Namespace: "server.components",
			Data:      []byte(`{"firmware":{"installed":"2.2.6","software_id":"159"}`),
			CreatedAt: now,
		},
	}

	// current versioned attribute fixture which includes data from newer, unsorted
	fixtureCurrentWithNewerVA := []serverservice.VersionedAttributes{
		fixtureCurrentVA[0],
		fixtureCurrentVA[1],
		fixtureNewVA[0],
	}

	testcases := []struct {
		name        string
		expectedErr error
		expectedObj *serverservice.VersionedAttributes
		currentObjs []serverservice.VersionedAttributes
		newObjs     []serverservice.VersionedAttributes
	}{
		{
			"with no new versioned objects, the method returns nil",
			nil,
			nil,
			fixtureCurrentVA,
			[]serverservice.VersionedAttributes{},
		},
		{
			"with no new versioned objects, and no current versioned objects the method returns nil",
			nil,
			nil,
			[]serverservice.VersionedAttributes{},
			[]serverservice.VersionedAttributes{},
		},
		{
			"with an empty current versioned attribute object, the method returns the newer object",
			nil,
			&fixtureNewVA[0],
			[]serverservice.VersionedAttributes{},
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
func addcomponent(sc []*serverservice.ServerComponent, t *testing.T, slug string, va *versionedAttributes) []*serverservice.ServerComponent {
	t.Helper()

	data, err := json.Marshal(va)
	if err != nil {
		t.Error(err)
	}

	component := &serverservice.ServerComponent{
		UUID:   uuid.New(),
		Name:   slug,
		Vendor: "",
		VersionedAttributes: []serverservice.VersionedAttributes{
			{
				Data:      data,
				Namespace: "foo.bar",
			},
		},
	}

	sc = append(sc, component)

	return sc
}

func updateComponentVA(sc []*serverservice.ServerComponent, t *testing.T, slug string, va *versionedAttributes) []*serverservice.ServerComponent {
	t.Helper()

	var component *serverservice.ServerComponent

	for _, c := range sc {
		if strings.EqualFold(c.ComponentTypeSlug, strings.ToLower(slug)) {
			component = c
			break
		}
	}

	if component == nil {
		t.Fatal("component with slug not found:" + slug)
	}

	newVA := newVersionAttributes(t, component.VersionedAttributes[0].Data, va.Firmware.Installed)

	newVAData, err := json.Marshal(newVA)
	if err != nil {
		t.Fatal(err)
	}

	component.VersionedAttributes[0].Data = newVAData

	return sc
}

func newVersionAttributes(t *testing.T, data json.RawMessage, value string) *versionedAttributes {
	t.Helper()

	va := &versionedAttributes{}

	err := json.Unmarshal(data, va)
	if err != nil {
		t.Fatal(err)
	}

	va.Firmware.Installed = value

	return va
}

func Test_ServerService_RegisterChanges_ObjectsEqual(t *testing.T) {
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)
	handler := http.NewServeMux()
	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/components", serverID.String()),
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				resp, err := os.ReadFile("../fixtures/serverservice_components_fc167440.json")
				if err != nil {
					t.Fatal(err)
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			default:
				t.Fatal("expected GET request, got: " + r.Method)
			}
		},
	)

	mock := httptest.NewServer(handler)
	cr := retryablehttp.NewClient()
	// nil logger to prevent debug logs
	cr.Logger = nil

	c, err := serverservice.NewClientWithToken(
		"hunter2",
		mock.URL,
		cr.StandardClient(),
	)

	if err != nil {
		t.Fatal(err)
	}

	serverService := serverServicePublisher{
		logger: app.NewLogrusEntryFromLogger(logrus.Fields{"component": "publisher"}, logrus.New()),
		slugs:  fixtures.ServerServiceSlugMap(),
		client: c,
	}

	device := &model.AssetDevice{ID: serverID.String(), Device: fixtures.CopyDevice(fixtures.R6515_fc167440)}

	err = serverService.registerChanges(context.TODO(), serverID, device)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_ServerService_RegisterChanges_ObjectsUpdated(t *testing.T) {
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
				resp, err := os.ReadFile("../fixtures/serverservice_components_fc167440.json")
				if err != nil {
					t.Fatal(err)
				}

				w.Header().Set("Content-Type", "application/json")

				_, _ = w.Write(resp)
			case http.MethodPut:
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				gotUpdate := []*serverservice.ServerComponent{}
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

	mock := httptest.NewServer(handler)
	cr := retryablehttp.NewClient()
	// nil logger to prevent debug logs
	cr.Logger = nil

	c, err := serverservice.NewClientWithToken(
		"hunter2",
		mock.URL,
		cr.StandardClient(),
	)

	if err != nil {
		t.Fatal(err)
	}

	serverService := serverServicePublisher{
		logger: app.NewLogrusEntryFromLogger(logrus.Fields{"component": "publisher"}, logrus.New()),
		slugs:  fixtures.ServerServiceSlugMap(),
		client: c,
	}

	// asset device fixture returned by the inventory collector
	device := &model.AssetDevice{
		ID:     serverID.String(),
		Device: fixtures.CopyDevice(fixtures.R6515_fc167440),
	}

	// bump version on BIOS and BMC components
	device.BIOS.Firmware.Installed = newBIOSFWVersion
	device.BMC.Firmware.Installed = newBMCFWVersion

	err = serverService.registerChanges(context.TODO(), serverID, device)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_ServerService_RegisterChanges_ObjectsAdded(t *testing.T) {
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

	fixtureNICSerial := "c00l"

	handler := http.NewServeMux()
	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/components", serverID.String()),
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				resp, err := os.ReadFile("../fixtures/serverservice_components_fc167440.json")
				if err != nil {
					t.Fatal(err)
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			case http.MethodPost:
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				gotAdded := []*serverservice.ServerComponent{}
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

	mock := httptest.NewServer(handler)
	cr := retryablehttp.NewClient()
	cr.Logger = nil

	c, err := serverservice.NewClientWithToken(
		"hunter2",
		mock.URL,
		cr.StandardClient(),
	)

	if err != nil {
		t.Fatal(err)
	}

	serverService := serverServicePublisher{
		logger: app.NewLogrusEntryFromLogger(logrus.Fields{"component": "publisher"}, logrus.New()),
		slugs:  fixtures.ServerServiceSlugMap(),
		client: c,
	}

	// asset device fixture returned by the inventory collector
	device := &model.Asset{
		ID:        serverID.String(),
		Inventory: fixtures.CopyDevice(fixtures.R6515_fc167440),
	}

	device.Inventory.NICs = append(
		device.Inventory.NICs,
		&common.NIC{
			ID:          "NEW NIC!",
			Description: "Just added!, totally incompatible",
			Common: common.Common{
				Vendor: "noname",
				Model:  "noname",
				Serial: fixtureNICSerial,
			},
		},
	)

	err = serverService.createUpdateServerComponents(context.TODO(), serverID, device)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_ServerService_CreateUpdateServerAttributes_Create(t *testing.T) {
	// test: createUpdateServerAttributes creates server attributes when its undefined in server service
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

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
				// the response here is
				resp, err := os.ReadFile("../fixtures/serverservice_server_fc167440.json")
				if err != nil {
					t.Fatal(err)
				}

				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				// unpack attributes posted by method
				attributes := &serverservice.Attributes{}
				if err = json.Unmarshal(b, attributes); err != nil {
					t.Fatal(err)
				}

				// asset NS is as expected
				assert.Equal(t, model.ServerVendorAttributeNS, attributes.Namespace)

				// unpack attributes data
				data := map[string]string{}
				if err = json.Unmarshal(attributes.Data, &data); err != nil {
					t.Fatal(err)
				}

				// asset attributes data matches device attributes
				assert.Equal(t, device.Inventory.Model, data[model.ServerModelAttributeKey])
				assert.Equal(t, device.Inventory.Serial, data[model.ServerSerialAttributeKey])
				assert.Equal(t, device.Inventory.Vendor, data[model.ServerVendorAttributeKey])

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			default:
				t.Fatal("expected POST request, got: " + r.Method)
			}
		},
	)

	mock := httptest.NewServer(handler)
	cr := retryablehttp.NewClient()
	// nil logger to prevent debug logs
	cr.Logger = nil

	c, err := serverservice.NewClientWithToken(
		"hunter2",
		mock.URL,
		cr.StandardClient(),
	)

	if err != nil {
		t.Fatal(err)
	}

	serverService := serverServicePublisher{
		logger: app.NewLogrusEntryFromLogger(logrus.Fields{"component": "publisher"}, logrus.New()),
		slugs:  fixtures.ServerServiceSlugMap(),
		client: c,
	}

	err = serverService.createUpdateServerAttributes(context.TODO(), serverID, device)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_ServerService_CreateUpdateServerAttributes_Update(t *testing.T) {
	// test: createUpdateServerAttributes updates server attributes when either of them are missing
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

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
		fmt.Sprintf("/api/v1/servers/%s/attributes/%s", serverID.String(), model.ServerVendorAttributeNS),
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				// the response here is
				resp, err := os.ReadFile("../fixtures/serverservice_server_fc167440.json")
				if err != nil {
					t.Fatal(err)
				}

				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				// unpack attributes posted by method
				attributes := &serverservice.Attributes{}
				if err = json.Unmarshal(b, attributes); err != nil {
					t.Fatal(err)
				}

				// unpack attributes data
				data := map[string]string{}
				if err = json.Unmarshal(attributes.Data, &data); err != nil {
					t.Fatal(err)
				}

				// asset attributes data matches device attributes
				assert.Equal(t, device.Inventory.Model, data[model.ServerModelAttributeKey])
				assert.Equal(t, device.Inventory.Serial, data[model.ServerSerialAttributeKey])
				assert.Equal(t, device.Inventory.Vendor, data[model.ServerVendorAttributeKey])

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			default:
				t.Fatal("expected PUT request, got: " + r.Method)
			}
		},
	)

	mock := httptest.NewServer(handler)
	cr := retryablehttp.NewClient()
	// nil logger to prevent debug logs
	cr.Logger = nil

	c, err := serverservice.NewClientWithToken(
		"hunter2",
		mock.URL,
		cr.StandardClient(),
	)

	if err != nil {
		t.Fatal(err)
	}

	serverService := serverServicePublisher{
		logger: app.NewLogrusEntryFromLogger(logrus.Fields{"component": "publisher"}, logrus.New()),
		slugs:  fixtures.ServerServiceSlugMap(),
		client: c,
	}

	err = serverService.createUpdateServerAttributes(context.TODO(), serverID, device)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_ServerService_CreateUpdateServerMetadataAttributes_Create(t *testing.T) {
	// test: createUpdateServerMetadataAttributes creats server metadata attributes when its undefined in server service.
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

	device := &model.Asset{
		Metadata: map[string]string{},
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
				// the response here is
				resp, err := os.ReadFile("../fixtures/serverservice_server_fc167440.json")
				if err != nil {
					t.Fatal(err)
				}

				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				// unpack attributes posted by method
				attributes := &serverservice.Attributes{}
				if err = json.Unmarshal(b, attributes); err != nil {
					t.Fatal(err)
				}

				// asset NS is as expected
				assert.Equal(t, model.ServerMetadataAttributeNS, attributes.Namespace)

				// unpack attributes data
				data := map[string]string{}
				if err = json.Unmarshal(attributes.Data, &data); err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, device.Inventory.Metadata, data)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			default:
				t.Fatal("expected POST request, got: " + r.Method)
			}
		},
	)

	mock := httptest.NewServer(handler)
	cr := retryablehttp.NewClient()
	// nil logger to prevent debug logs
	cr.Logger = nil

	c, err := serverservice.NewClientWithToken(
		"hunter2",
		mock.URL,
		cr.StandardClient(),
	)

	if err != nil {
		t.Fatal(err)
	}

	serverService := serverServicePublisher{
		logger: app.NewLogrusEntryFromLogger(logrus.Fields{"component": "publisher"}, logrus.New()),
		slugs:  fixtures.ServerServiceSlugMap(),
		client: c,
	}

	err = serverService.createUpdateServerMetadataAttributes(context.TODO(), serverID, device)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_ServerService_CreateUpdateServerMetadataAttributes_Update(t *testing.T) {
	// test: createUpdateServerMetadataAttributes updates server metadata attributes when it differs.
	serverID, _ := uuid.Parse(fixtures.TestserverID_Dell_fc167440)

	device := &model.Asset{
		Metadata: map[string]string{"foo": "bar"},
		Inventory: &common.Device{
			Common: common.Common{
				Metadata: map[string]string{"foo": "bar", "test": "lala"},
			},
		},
	}

	handler := http.NewServeMux()
	// get components query
	handler.HandleFunc(
		fmt.Sprintf("/api/v1/servers/%s/attributes/%s", serverID.String(), model.ServerMetadataAttributeNS),
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				// the response here is
				resp, err := os.ReadFile("../fixtures/serverservice_server_fc167440.json")
				if err != nil {
					t.Fatal(err)
				}

				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				// unpack attributes posted by method
				attributes := &serverservice.Attributes{}
				if err = json.Unmarshal(b, attributes); err != nil {
					t.Fatal(err)
				}

				// unpack attributes data
				data := map[string]string{}
				if err = json.Unmarshal(attributes.Data, &data); err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, device.Inventory.Metadata, data)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			default:
				t.Fatal("expected PUT request, got: " + r.Method)
			}
		},
	)

	mock := httptest.NewServer(handler)
	cr := retryablehttp.NewClient()
	// nil logger to prevent debug logs
	cr.Logger = nil

	c, err := serverservice.NewClientWithToken(
		"hunter2",
		mock.URL,
		cr.StandardClient(),
	)

	if err != nil {
		t.Fatal(err)
	}

	serverService := serverServicePublisher{
		logger: app.NewLogrusEntryFromLogger(logrus.Fields{"component": "publisher"}, logrus.New()),
		slugs:  fixtures.ServerServiceSlugMap(),
		client: c,
	}

	err = serverService.createUpdateServerMetadataAttributes(context.TODO(), serverID, device)
	if err != nil {
		t.Fatal(err)
	}
}
