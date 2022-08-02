package fixtures

import (
	"context"
	"os"

	bmclibv2 "github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/common"
)

const (
	EnvMockBMCOpen  = "MOCK_BMC_OPEN"
	EnvMockBMCClose = "MOCK_BMC_CLOSE"
)

type MockBmclib struct {
	// embed bmclib client to provide methods
	bmclibv2.Client
	device *common.Device
}

func NewBmclibClient() *MockBmclib {
	return &MockBmclib{}
}

func (m *MockBmclib) Open(ctx context.Context) error {
	os.Setenv(EnvMockBMCOpen, "true")
	return nil
}

func (m *MockBmclib) Close(ctx context.Context) error {
	os.Setenv(EnvMockBMCClose, "true")
	return nil
}

func (m *MockBmclib) Inventory(ctx context.Context) (*common.Device, error) {
	return CopyDevice(E3C246D4INL), nil
}

func NewMockBmclib() *MockBmclib {
	return &MockBmclib{}
}

func (m *MockBmclib) SetMockDevice(d *common.Device) {
	m.device = d
}
