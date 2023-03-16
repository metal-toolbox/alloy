package serverservice

import (
	"context"
	"net"

	// _ "net/http/pprof"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/fixtures"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/stretchr/testify/assert"
	serverserviceapi "go.hollow.sh/serverservice/pkg/api/v1"
)

func newMockServerServiceGetter(t *testing.T, alloy *app.App) *serverServiceStore {
	t.Helper()

	return &serverServiceStore{
		logger: alloy.Logger.WithField("component", "test"),
		config: alloy.Config,
		client: fixtures.NewMockServerServiceClient(),
	}
}

func Test_ServerServiceListByIDs(t *testing.T) {
	// left commented out here to indicate a means of debugging
	//	go func() {
	//		log.Println(http.ListenAndServe("localhost:9091", nil))
	// }()
	// init alloy app
	alloy, err := app.New(context.TODO(), model.AppKindOutOfBand, "", model.LogLevelTrace)
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

			alloy, err := app.New(context.Background(), model.AppKindOutOfBand, "", model.LogLevelInfo)
			if err != nil {
				t.Fatal(err)
			}

			// alloy.Config.ServerserviceOptions.Concurrency = model.ConcurrencyDefault

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
	// nolint:govet // ignore struct alignment in test
	cases := []struct {
		name              string
		server            *serverserviceapi.Server
		secret            *serverserviceapi.ServerCredential
		expectCredentials bool
		expectedErr       string
	}{
		{
			"server object nil",
			nil,
			nil,
			true,
			"server object nil",
		},
		{
			"server credential object nil",
			&serverserviceapi.Server{},
			nil,
			true,
			"server credential object nil",
		},
		{
			"server attributes slice empty",
			&serverserviceapi.Server{},
			&serverserviceapi.ServerCredential{},
			true,
			"server attributes slice empty",
		},
		{
			"BMC password field empty",
			&serverserviceapi.Server{Attributes: []serverserviceapi.Attributes{{Namespace: bmcAttributeNamespace}}},
			&serverserviceapi.ServerCredential{Username: "foo", Password: ""},
			true,
			"BMC password field empty",
		},
		{
			"BMC username field empty",
			&serverserviceapi.Server{Attributes: []serverserviceapi.Attributes{{Namespace: bmcAttributeNamespace}}},
			&serverserviceapi.ServerCredential{Username: "", Password: "123"},
			true,
			"BMC username field empty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRequiredAttributes(tc.server, tc.secret, tc.expectCredentials)
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
		server        *serverserviceapi.Server
		secret        *serverserviceapi.ServerCredential
		expectedAsset *model.Asset
		expectedErr   string
	}{
		{
			"Expected attributes empty raises error",
			&serverserviceapi.Server{
				Attributes: []serverserviceapi.Attributes{
					{
						Namespace: "invalid",
					},
				},
			},
			&serverserviceapi.ServerCredential{Username: "foo", Password: "bar"},
			nil,
			"expected server attributes with BMC address, got none",
		},
		{
			"Attributes missing BMC IP Address raises error",
			&serverserviceapi.Server{
				Attributes: []serverserviceapi.Attributes{
					{
						Namespace: bmcAttributeNamespace,
						Data:      []byte(`{"namespace":"foo"}`),
					},
				},
			},
			&serverserviceapi.ServerCredential{Username: "user", Password: "hunter2"},
			nil,
			"expected BMC address attribute empty",
		},
		{
			"Valid server, secret objects returns *model.Asset object",
			&serverserviceapi.Server{
				Attributes: []serverserviceapi.Attributes{
					{
						Namespace: bmcAttributeNamespace,
						Data:      []byte(`{"address":"127.0.0.1"}`),
					},
				},
			},
			&serverserviceapi.ServerCredential{Username: "user", Password: "hunter2"},
			&model.Asset{
				ID:          "00000000-0000-0000-0000-000000000000",
				Vendor:      "unknown",
				Model:       "unknown",
				Serial:      "unknown",
				Facility:    "",
				BMCUsername: "user",
				BMCPassword: "hunter2",
				BMCAddress:  net.ParseIP("127.0.0.1"),
				Metadata:    map[string]string{},
			},
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			asset, err := toAsset(tc.server, tc.secret, true)
			if tc.expectedErr != "" {
				assert.Contains(t, err.Error(), tc.expectedErr)
				return
			}

			assert.Nil(t, err)
			assert.Equal(t, tc.expectedAsset, asset)
		})
	}
}
