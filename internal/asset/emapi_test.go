package asset

import (
	"context"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/fixtures"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	// _ "net/http/pprof"
)

func Test_EMAPIListByIDs(t *testing.T) {
	// left commented out here to indicate a means of debugging
	//	go func() {
	//		log.Println(http.ListenAndServe("localhost:9091", nil))
	// }()
	// init alloy app
	alloy, err := app.New(context.TODO(), app.KindOutOfBand, "", model.LogLevelTrace)
	if err != nil {
		t.Fatal(err)
	}

	alloy.Config.AssetGetter.Emapi.AuthToken = "authtoken"
	alloy.Config.AssetGetter.Emapi.ConsumerToken = "consumertoken"
	alloy.Config.AssetGetter.Emapi.Facility = "ac12"

	got := []*model.Asset{}
	expected := []*model.Asset{
		fixtures.MockAssets["foo"],
		fixtures.MockAssets["bar"],
	}

	var collectTimedOut bool

	// background routine to receive asset objects objects sent from the asset getter
	// mocks a collector
	alloy.SyncWg.Add(1)

	go func(t *testing.T, wg *sync.WaitGroup) {
		t.Helper()

		defer wg.Done()

		timeout := time.NewTicker(time.Second * 5).C
	Loop:
		for {
			select {
			case asset, ok := <-alloy.AssetCh:
				if !ok {
					break Loop
				}
				got = append(got, asset)
			case <-timeout:
				collectTimedOut = true
				break Loop
			}
		}
	}(t, alloy.SyncWg)

	// init asset getter
	emapi, err := NewEMAPISource(context.TODO(), alloy)
	if err != nil {
		t.Fatal(err)
	}

	// set mock emapi requestor client to mock client method responses
	emapi.SetClient(fixtures.NewMockEMAPIClient())

	err = emapi.ListByIDs(context.TODO(), []string{"foo", "bar"})
	if err != nil {
		t.Fatal(err)
	}

	// wait for routines to complete
	alloy.SyncWg.Wait()

	// test inventory items match expected
	assert.ElementsMatch(t, expected, got)
	assert.False(t, collectTimedOut)
}

func Test_EMAPIListAll(t *testing.T) {
	// nolint:govet // test struct is clearer to read in this alignment
	testcases := []struct {
		batchSize   int
		totalAssets int
		name        string
	}{
		{
			1,
			1,
			"total == batch size",
		},
		{
			1,
			10,
			"total > batch size",
		},
		{
			20,
			10,
			"total < batch size",
		},
	}

	os.Setenv("TEST_ENV", "1")
	defer os.Unsetenv("TEST_ENV")

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var collectTimedOut bool
			got := []*model.Asset{}

			assetCh := make(chan *model.Asset)
			e := &emapi{
				logger:  logrus.New().WithField("component", "getter.emapi"),
				syncWg:  &sync.WaitGroup{},
				assetCh: assetCh,
				workers: workerpool.New(2),
			}
			e.logger.Level = logrus.TraceLevel

			// e.logger.Level = logrus.TraceLevel
			// set mock emapi requestor client to mock client method responses
			e.SetClient(fixtures.NewMockEMAPIClient())

			// rigs the mock AssetsByOffsetLimit to return the total count of assets
			os.Setenv("EMAPI_FIXTURE_TOTAL_ASSETS", strconv.Itoa(tc.totalAssets))
			defer os.Unsetenv("EMAPI_FIXTURE_TOTAL_ASSETS")

			// background routine to receive asset objects objects sent from the asset getter
			// mocks a collector
			e.syncWg.Add(1)
			go func(t *testing.T) {
				t.Helper()
				defer e.syncWg.Done()

				timeout := time.NewTicker(time.Second*delayBetweenRequests + 5).C
			Loop:
				for {
					select {
					case asset, ok := <-assetCh:
						if !ok {
							break Loop
						}
						got = append(got, asset)
					case <-timeout:
						collectTimedOut = true
						break Loop
					}
				}
			}(t)

			err := e.ListAll(context.TODO())
			if err != nil {
				t.Fatal(err)
			}

			// wait for routines to return
			e.syncWg.Wait()

			assert.Equal(t, tc.totalAssets, len(got))
			assert.False(t, collectTimedOut)
		})
	}
}
