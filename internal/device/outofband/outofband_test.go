package outofband

import (
	"context"
	"testing"

	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func Test_Inventory(t *testing.T) {
	logger := logrus.New()
	// nolint:gocritic // comment left here for reference
	// logger.Level = logrus.TraceLevel
	bmcQueryor := NewMockBmclibClient()
	queryor := &Queryor{
		mockClient: bmcQueryor,
		logger:     logrus.NewEntry(logger),
	}

	loginInfo := &model.LoginInfo{}

	inventory, err := queryor.Inventory(context.TODO(), loginInfo)
	if err != nil {
		t.Fatal(err)
	}

	assert.NotNil(t, inventory)
	assert.True(t, bmcQueryor.connOpened)
	assert.True(t, bmcQueryor.connClosed)
}

func Test_BiosConfiguration(t *testing.T) {
	logger := logrus.New()
	// nolint:gocritic // comment left here for reference
	// logger.Level = logrus.TraceLevel
	bmcQueryor := NewMockBmclibClient()
	queryor := &Queryor{
		mockClient: bmcQueryor,
		logger:     logrus.NewEntry(logger),
	}

	loginInfo := &model.LoginInfo{}

	biosCfg, err := queryor.BiosConfiguration(context.TODO(), loginInfo)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 1, len(biosCfg))
	assert.True(t, bmcQueryor.connOpened)
	assert.True(t, bmcQueryor.connClosed)
}
