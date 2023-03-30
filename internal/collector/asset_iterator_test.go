package collector

import (
	"context"
	"sync"
	"testing"

	"github.com/metal-toolbox/alloy/internal/store/mock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func Test_IterInBatches(t *testing.T) {
	testcases := []struct {
		name     string
		limit    int
		total    int
		expected int
	}{
		{
			"total is zero",
			10,
			0,
			0,
		},
		{
			"total is one",
			10,
			1,
			1,
		},
		{
			"limit half of total",
			10,
			20,
			20,
		},
		{
			"limit equals total",
			20,
			20,
			20,
		},
		{
			"limit higher than total",
			20,
			3,
			3,
		},
		{
			"high total returns expected",
			5,
			100,
			100,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			mockstore, _ := mock.New(tt.total)
			logger := logrus.New()
			// nolint: gocritic // comment kept for reference
			// logger.Level = logrus.TraceLevel
			pauser := NewPauser()

			assetIterator := NewAssetIterator(mockstore, logger)

			var got int

			var syncWG sync.WaitGroup

			syncWG.Add(1)
			go func() {
				defer syncWG.Done()
				for range assetIterator.Channel() {
					got++
				}

				assert.Equal(t, tt.expected, got)
			}()

			assetIterator.IterInBatches(context.TODO(), tt.limit, pauser)
			syncWG.Wait()
		})
	}
}
