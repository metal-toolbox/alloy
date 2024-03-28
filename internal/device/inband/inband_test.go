package inband

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/metal-toolbox/alloy/internal/model"
)

func Test_Inventory(t *testing.T) {
	logger := logrus.New()
	// nolint:gocritic // comment left here for reference
	// logger.Level = logrus.TraceLevel
	inbandQueryor := NewMockIronlibClient()
	queryor := &Queryor{
		deviceManager: inbandQueryor,
		logger:        logrus.NewEntry(logger),
		mock:          true,
	}

	loginInfo := &model.LoginInfo{}

	inventory, err := queryor.Inventory(context.TODO(), loginInfo)
	if err != nil {
		t.Fatal(err)
	}

	assert.NotNil(t, inventory)
}

func Test_BiosConfiguration(t *testing.T) {
	logger := logrus.New()
	// nolint:gocritic // comment left here for reference
	// logger.Level = logrus.TraceLevel
	inbandQueryor := NewMockIronlibClient()
	queryor := &Queryor{
		deviceManager: inbandQueryor,
		logger:        logrus.NewEntry(logger),
		mock:          true,
	}

	loginInfo := &model.LoginInfo{}

	biosCfg, err := queryor.BiosConfiguration(context.TODO(), loginInfo)
	if err != nil {
		t.Fatal(err)
	}

	assert.NotNil(t, biosCfg)
}
