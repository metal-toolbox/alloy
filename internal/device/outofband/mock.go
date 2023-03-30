package outofband

import (
	"context"

	bmclibv2 "github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/common"
)

// nolint:govet // fieldalignment, pointless in tests
type MockBmclib struct {
	// embed bmclib client to provide methods
	bmclibv2.Client
	device     *common.Device
	connOpened bool
	connClosed bool
}

func NewMockBmclibClient() *MockBmclib {
	return &MockBmclib{}
}

func (m *MockBmclib) Open(ctx context.Context) error {
	m.connOpened = true
	return nil
}

func (m *MockBmclib) Close(ctx context.Context) error {
	m.connClosed = true
	return nil
}

func (m *MockBmclib) Inventory(ctx context.Context) (*common.Device, error) {
	return &common.Device{Common: common.Common{Vendor: "foo", Model: "bar"}}, nil
}

func (m *MockBmclib) GetBiosConfiguration(ctx context.Context) (biosConfig map[string]string, err error) {
	biosConfig = make(map[string]string)
	biosConfig["foo"] = "bar"

	return biosConfig, nil
}

func NewMockBmclib() *MockBmclib {
	return &MockBmclib{}
}

func (m *MockBmclib) SetMockDevice(d *common.Device) {
	m.device = d
}
