package fixtures

import (
	"context"

	"github.com/bmc-toolbox/common"

	"github.com/metal-toolbox/ironlib/actions"
)

// MockIronlib mocks ironlib methods, responses
type MockIronlib struct {
	// embed DeviceManager interface so we don't have to implement all interface methods
	actions.DeviceManager

	// device is the device object returned by this mock instance
	device *common.Device
}

func NewMockIronlib() *MockIronlib {
	return &MockIronlib{}
}

func (m *MockIronlib) SetMockDevice(d *common.Device) {
	m.device = d
}

func (m *MockIronlib) GetInventory(ctx context.Context, options ...actions.Option) (*common.Device, error) {
	return m.device, nil
}

func (m *MockIronlib) GetBIOSConfiguration(ctx context.Context) (biosConfig map[string]string, err error) {
	return biosConfig, nil
}
