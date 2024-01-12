package collector

import (
	"context"
	"sync"
	"testing"

	"github.com/metal-toolbox/alloy/internal/device"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/store/mock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

// XXX: Warning, this test might be flaky.
func Test_Collect_Concurrent(t *testing.T) {
	ignorefunc := "go.opencensus.io/stats/view.(*worker).start"
	defer goleak.VerifyNone(t, goleak.IgnoreTopFunction(ignorefunc))

	logger := logrus.New()
	// nolint:gocritic // comment left here for reference
	// logger.Level = logrus.TraceLevel
	mockstore, _ := mock.New(10)
	assetIterator := NewAssetIterator(mockstore, logger)
	mockDeviceQueryor := device.NewMockDeviceQueryor(model.AppKindOutOfBand)

	assetIterCollector := &AssetIterCollector{
		concurrency:   20,
		queryor:       mockDeviceQueryor,
		assetIterator: *assetIterator,
		repository:    mockstore,
		syncWG:        &sync.WaitGroup{},
		logger:        logger,
	}

	assetIterCollector.Collect(context.TODO())
	assert.Equal(t, 10, mockstore.UpdatedAssets)
}

func Test_Collect(t *testing.T) {
	ignorefunc := "go.opencensus.io/stats/view.(*worker).start"
	defer goleak.VerifyNone(t, goleak.IgnoreTopFunction(ignorefunc))

	logger := logrus.New()
	// nolint:gocritic // comment left here for reference
	// logger.Level = logrus.TraceLevel
	mockstore, _ := mock.New(3)
	assetIterator := NewAssetIterator(mockstore, logger)
	mockDeviceQueryor := device.NewMockDeviceQueryor(model.AppKindOutOfBand)

	syncWG := &sync.WaitGroup{}

	assetIterCollector := &AssetIterCollector{
		concurrency:   1,
		queryor:       mockDeviceQueryor,
		assetIterator: *assetIterator,
		repository:    mockstore,
		syncWG:        syncWG,
		logger:        logger,
	}

	syncWG.Add(1)

	go func() {
		defer syncWG.Done()
		assetIterCollector.Collect(context.TODO())
	}()

	syncWG.Wait()

	assert.Equal(t, 3, mockstore.UpdatedAssets)
}
