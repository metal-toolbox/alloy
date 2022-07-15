package collect

import (
	"context"
	"testing"
	"time"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/fixtures"
	"gotest.tools/assert"

	"github.com/metal-toolbox/alloy/internal/model"
)

func Test_InbandInventory(t *testing.T) {
	// init mock ironlib with test fixture
	mockIronlib := fixtures.NewMockIronlib()
	mockIronlib.SetMockDevice(fixtures.CopyDevice(fixtures.E3C246D4INL))

	// init alloy app
	alloy, err := app.New(context.TODO(), app.KindInband, "", model.LogLevelTrace)
	if err != nil {
		t.Fatal(err)
	}

	// init mock inband inventory collector
	collector := NewInbandCollector(alloy)
	collector.SetMockGetter(mockIronlib)

	var got *model.AssetDevice

	// background routine to collect device inventory objects sent from the collector
	go func(t *testing.T) {
		t.Helper()

		timeout := time.NewTicker(time.Second * 2).C

		var ok bool

	Loop:
		for {
			select {
			case got, ok = <-alloy.CollectorCh:
				if !ok {
					continue
				}
				break Loop
			case <-timeout:
				break Loop
			}
		}
	}(t)

	// collect inventory
	err = collector.Inventory(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	assert.DeepEqual(t, &model.AssetDevice{Device: fixtures.E3C246D4INL}, got)
}
