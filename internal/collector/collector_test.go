package collector

//func newMockServerServiceGetter(t *testing.T, alloy *app.App) *serverServiceStore {
//	t.Helper()
//
//	return &serverServiceStore{
//		logger: alloy.Logger.WithField("component", "test"),
//		config: alloy.Config,
//		client: fixtures.NewMockServerServiceClient(),
//	}
//}

//func Test_ServerServiceListByIDs(t *testing.T) {
//	// left commented out here to indicate a means of debugging
//	//	go func() {
//	//		log.Println(http.ListenAndServe("localhost:9091", nil))
//	// }()
//	// init alloy app
//	alloy, err := app.New(context.TODO(), model.AppKindOutOfBand, "", model.LogLevelTrace)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	got := []*model.Asset{}
//	expected := []*model.Asset{
//		fixtures.MockAssets["foo"],
//		fixtures.MockAssets["bar"],
//	}
//
//	var collectTimedOut bool
//
//	// background routine to receive asset objects objects sent from the asset getter
//	// mocks a collector
//	alloy.SyncWg.Add(1)
//
//	go func(t *testing.T, wg *sync.WaitGroup) {
//		t.Helper()
//
//		defer wg.Done()
//
//		timeout := time.NewTicker(time.Second * 5).C
//	Loop:
//		for {
//			select {
//			case asset, ok := <-alloy.AssetCh:
//				if !ok {
//					break Loop
//				}
//				got = append(got, asset)
//			case <-timeout:
//				collectTimedOut = true
//				break Loop
//			}
//		}
//	}(t, alloy.SyncWg)
//
//	// init asset getter
//	getter := newMockServerServiceGetter(t, alloy)
//
//	err = getter.ListByIDs(context.TODO(), []string{"foo", "bar"})
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	// wait for routines to complete
//	alloy.SyncWg.Wait()
//
//	// test inventory items match expected
//	assert.ElementsMatch(t, expected, got)
//	assert.False(t, collectTimedOut)
//}
//
//func Test_ServerServiceListAll(t *testing.T) {
//	// nolint:govet // test struct is clearer to read in this alignment
//	testcases := []struct {
//		batchSize   int
//		totalAssets int
//		name        string
//	}{
//		{
//			1,
//			1,
//			"total == batch size",
//		},
//		{
//			1,
//			11,
//			"total > batch size",
//		},
//		{
//			20,
//			11,
//			"total < batch size",
//		},
//	}
//
//	os.Setenv("TEST_ENV", "1")
//	defer os.Unsetenv("TEST_ENV")
//
//	for _, tc := range testcases {
//		t.Run(tc.name, func(t *testing.T) {
//			var collectTimedOut bool
//			got := []*model.Asset{}
//
//			assetCh := make(chan *model.Asset)
//
//			alloy, err := app.New(context.Background(), model.AppKindOutOfBand, "", model.LogLevelInfo)
//			if err != nil {
//				t.Fatal(err)
//			}
//
//			// alloy.Config.ServerserviceOptions.Concurrency = model.ConcurrencyDefault
//
//			getter := newMockServerServiceGetter(t, alloy)
//			getter.assetCh = assetCh
//
//			// rigs the mock AssetsByOffsetLimit to return the total count of assets
//			os.Setenv("FIXTURE_TOTAL_ASSETS", strconv.Itoa(tc.totalAssets))
//			defer os.Unsetenv("FIXTURE_TOTAL_ASSETS")
//
//			// background routine to receive asset objects objects sent from the asset getter
//			// mocks a collector
//			getter.syncWg.Add(1)
//			go func(t *testing.T) {
//				t.Helper()
//				defer getter.syncWg.Done()
//
//				timeout := time.NewTicker(time.Second*delayBetweenRequests + 5).C
//			Loop:
//				for {
//					select {
//					case asset, ok := <-assetCh:
//						if !ok {
//							break Loop
//						}
//						got = append(got, asset)
//					case <-timeout:
//						collectTimedOut = true
//						break Loop
//					}
//				}
//			}(t)
//
//			err = getter.ListAll(context.TODO())
//			if err != nil {
//				t.Fatal(err)
//			}
//
//			// wait for routines to return
//			getter.syncWg.Wait()
//
//			assert.Equal(t, tc.totalAssets, len(got))
//			assert.False(t, collectTimedOut)
//		})
//	}
//}
