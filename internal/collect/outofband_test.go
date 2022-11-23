package collect

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/fixtures"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/stretchr/testify/assert"
)

func Test_OutOfBandInventoryRemote(t *testing.T) {
	// init alloy app
	alloy, err := app.New(context.TODO(), model.AppKindOutOfBand, "", model.LogLevelInfo)
	if err != nil {
		t.Fatal(err)
	}

	// env variables set by mock bmclib fixture
	defer func() {
		os.Unsetenv(fixtures.EnvMockBMCOpen)
		os.Unsetenv(fixtures.EnvMockBMCClose)
	}()

	// init mock collector which mocks OOB inventory
	collector := NewOutOfBandCollector(alloy)
	collector.SetMockGetter(fixtures.NewMockBmclib())

	// mock assets the collector will collect OOB inventory for
	mockAssets := fixtures.MockAssets
	got := []*model.Asset{}

	// background routine to get assets from the store for which inventory is collected
	// mocks an AssetGetter
	alloy.SyncWg.Add(1)

	go func(t *testing.T, wg *sync.WaitGroup) {
		t.Helper()

		defer wg.Done()

		assetGetter := fixtures.NewMockAssetGetter(alloy.AssetCh, mockAssets)
		assetGetter.ListAll(context.TODO())
	}(t, alloy.SyncWg)

	// background routine to collect device inventory objects sent from the collector
	// mocks a Publisher
	alloy.SyncWg.Add(1)

	go func(t *testing.T, wg *sync.WaitGroup) {
		t.Helper()

		defer wg.Done()

		timeout := time.NewTicker(time.Second * 5).C
	Loop:
		for {
			select {
			case device, ok := <-alloy.CollectorCh:
				if !ok {
					break Loop
				}
				got = append(got, device)
			case <-timeout:
				fmt.Println("hit timeout...")
				break Loop
			}
		}
	}(t, alloy.SyncWg)

	// run the inventory collector
	err = collector.InventoryRemote(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	// wait for routines to complete
	alloy.SyncWg.Wait()

	// test inventory items match expected
	assert.Equal(t, len(mockAssets), len(got))

	// test bmc connection was opened
	assert.Equal(t, os.Getenv(fixtures.EnvMockBMCOpen), "true")

	// test bmc connection was closed
	assert.Equal(t, os.Getenv(fixtures.EnvMockBMCClose), "true")
}
