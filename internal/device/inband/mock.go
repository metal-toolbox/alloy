package inband

import (
	"context"

	common "github.com/metal-toolbox/bmc-common"
	"github.com/metal-toolbox/ironlib/actions"
)

// MockIronlib mocks ironlib methods, responses
type MockIronlib struct {
	// embed DeviceManager interface so we don't have to implement all interface methods
	actions.DeviceManager
}

func NewMockIronlibClient() *MockIronlib {
	return &MockIronlib{}
}

func (m *MockIronlib) GetInventory(_ context.Context, _ ...actions.Option) (*common.Device, error) {
	return &common.Device{Common: common.Common{Vendor: "foo", Model: "bar"}}, nil
}

func (m *MockIronlib) GetBIOSConfiguration(_ context.Context) (map[string]string, error) {
	return map[string]string{"foo": "bar"}, nil
}
