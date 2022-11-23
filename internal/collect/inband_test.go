package collect

import (
	"context"
	"testing"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/fixtures"
	"github.com/stretchr/testify/assert"

	"github.com/metal-toolbox/alloy/internal/model"
)

func Test_InbandInventory(t *testing.T) {
	// init mock ironlib with test fixture
	mockIronlib := fixtures.NewMockIronlib()
	mockIronlib.SetMockDevice(fixtures.CopyDevice(fixtures.E3C246D4INL))

	// init alloy app
	alloy, err := app.New(context.TODO(), model.AppKindInband, "", model.LogLevelTrace)
	if err != nil {
		t.Fatal(err)
	}

	// init mock inband inventory collector
	collector := NewInbandCollector(alloy)
	collector.SetMockGetter(mockIronlib)

	var got *model.Asset

	// collect inventory
	got, err = collector.InventoryLocal(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, &model.Asset{Inventory: fixtures.E3C246D4INL, Vendor: "unknown", Model: "unknown", Serial: "unknown"}, got)
}
