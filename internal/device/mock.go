package device

import (
	"context"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/alloy/internal/model"
)

// NewMockDeviceQueryor returns a mock device.Queryor implementation.
func NewMockDeviceQueryor(_ model.AppKind) Queryor {
	return &MockDeviceQueryor{}
}

type MockDeviceQueryor struct{}

func (m *MockDeviceQueryor) Inventory(_ context.Context, _ *model.LoginInfo) (*common.Device, error) {
	return nil, nil
}

func (m *MockDeviceQueryor) BiosConfiguration(_ context.Context, _ *model.LoginInfo) (map[string]string, error) {
	return nil, nil
}
