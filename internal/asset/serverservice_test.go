package asset

import (
	"context"
	"net"

	// _ "net/http/pprof"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/fixtures"
	"github.com/metal-toolbox/alloy/internal/helpers"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/stretchr/testify/assert"
	serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

func newMockServerServiceGetter(t *testing.T, alloy *app.App) *serverServiceGetter {
	t.Helper()

	return &serverServiceGetter{
		logger:  alloy.Logger.WithField("component", "test"),
		syncWg:  alloy.SyncWg,
		config:  alloy.Config,
		assetCh: alloy.AssetCh,
		workers: workerpool.New(1),
		pauser:  helpers.NewPauser(),
		client:  fixtures.NewMockServerServiceClient(),
	}
}

func Test_ServerServiceListByIDs(t *testing.T) {
	// left commented out here to indicate a means of debugging
	//	go func() {
	//		log.Println(http.ListenAndServe("localhost:9091", nil))
	// }()
	// init alloy app
	alloy, err := app.New(context.TODO(), app.KindOutOfBand, "", model.LogLevelTrace)
	if err != nil {
		t.Fatal(err)
	}

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
	getter := newMockServerServiceGetter(t, alloy)

	err = getter.ListByIDs(context.TODO(), []string{"foo", "bar"})
	if err != nil {
		t.Fatal(err)
	}

	// wait for routines to complete
	alloy.SyncWg.Wait()

	// test inventory items match expected
	assert.ElementsMatch(t, expected, got)
	assert.False(t, collectTimedOut)
}

func Test_ServerServiceListAll(t *testing.T) {
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
			11,
			"total > batch size",
		},
		{
			20,
			11,
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

			alloy, err := app.New(context.Background(), app.KindOutOfBand, "", model.LogLevelInfo)
			if err != nil {
				t.Fatal(err)
			}

			alloy.Config.ServerService.Concurrency = model.ConcurrencyDefault

			getter := newMockServerServiceGetter(t, alloy)
			getter.assetCh = assetCh

			// rigs the mock AssetsByOffsetLimit to return the total count of assets
			os.Setenv("FIXTURE_TOTAL_ASSETS", strconv.Itoa(tc.totalAssets))
			defer os.Unsetenv("FIXTURE_TOTAL_ASSETS")

			// background routine to receive asset objects objects sent from the asset getter
			// mocks a collector
			getter.syncWg.Add(1)
			go func(t *testing.T) {
				t.Helper()
				defer getter.syncWg.Done()

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

			err = getter.ListAll(context.TODO())
			if err != nil {
				t.Fatal(err)
			}

			// wait for routines to return
			getter.syncWg.Wait()

			assert.Equal(t, tc.totalAssets, len(got))
			assert.False(t, collectTimedOut)
		})
	}
}

func Test_validateRequiredAttribtues(t *testing.T) {
	cases := []struct {
		name        string
		server      *serverservice.Server
		secret      *serverservice.ServerCredential
		expectedErr string
	}{
		{
			"server object nil",
			nil,
			nil,
			"server object nil",
		},
		{
			"server credential object nil",
			&serverservice.Server{},
			nil,
			"server credential object nil",
		},
		{
			"server attributes slice empty",
			&serverservice.Server{},
			&serverservice.ServerCredential{},
			"server attributes slice empty",
		},
		{
			"BMC password field empty",
			&serverservice.Server{Attributes: []serverservice.Attributes{{Namespace: bmcAttributeNamespace}}},
			&serverservice.ServerCredential{Username: "foo", Password: ""},
			"BMC password field empty",
		},
		{
			"BMC username field empty",
			&serverservice.Server{Attributes: []serverservice.Attributes{{Namespace: bmcAttributeNamespace}}},
			&serverservice.ServerCredential{Username: "", Password: "123"},
			"BMC username field empty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRequiredAttributes(tc.server, tc.secret)
			if tc.expectedErr != "" {
				assert.Contains(t, err.Error(), tc.expectedErr)
				return
			}

			assert.Nil(t, err)
		})
	}
}

func Test_toAsset(t *testing.T) {
	cases := []struct {
		name          string
		server        *serverservice.Server
		secret        *serverservice.ServerCredential
		expectedAsset *model.Asset
		expectedErr   string
	}{
		{
			"Expected attributes empty raises error",
			&serverservice.Server{
				Attributes: []serverservice.Attributes{
					{
						Namespace: "invalid",
					},
				},
			},
			&serverservice.ServerCredential{Username: "foo", Password: "bar"},
			nil,
			"expected server attributes with BMC address, got none",
		},
		{
			"Attributes missing BMC IP Address raises error",
			&serverservice.Server{
				Attributes: []serverservice.Attributes{
					{
						Namespace: bmcAttributeNamespace,
						Data:      []byte(`{"namespace":"foo"}`),
					},
				},
			},
			&serverservice.ServerCredential{Username: "user", Password: "hunter2"},
			nil,
			"expected BMC address attribute empty",
		},
		{
			"Valid server, secret objects returns *model.Asset object",
			&serverservice.Server{
				Attributes: []serverservice.Attributes{
					{
						Namespace: bmcAttributeNamespace,
						Data:      []byte(`{"address":"127.0.0.1"}`),
					},
				},
			},
			&serverservice.ServerCredential{Username: "user", Password: "hunter2"},
			&model.Asset{
				ID:          "00000000-0000-0000-0000-000000000000",
				Vendor:      "",
				Model:       "",
				Facility:    "",
				BMCUsername: "user",
				BMCPassword: "hunter2",
				BMCAddress:  net.ParseIP("127.0.0.1"),
			},
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			asset, err := toAsset(tc.server, tc.secret)
			if tc.expectedErr != "" {
				assert.Contains(t, err.Error(), tc.expectedErr)
				return
			}

			assert.Nil(t, err)
			assert.Equal(t, tc.expectedAsset, asset)
		})
	}
}
