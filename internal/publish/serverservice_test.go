package publish

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/alloy/internal/fixtures"
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
