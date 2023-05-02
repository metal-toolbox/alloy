package device

import (
	"context"

	"github.com/metal-toolbox/alloy/internal/model"
)

// NewMockDeviceQueryor returns a mock device.Queryor implementation.
func NewMockDeviceQueryor(_ model.AppKind) Queryor {
	return &MockDeviceQueryor{}
}

type MockDeviceQueryor struct{}

func (m *MockDeviceQueryor) Inventory(_ context.Context, _ *model.Asset) error {
	return nil
}

func (m *MockDeviceQueryor) BiosConfiguration(_ context.Context, _ *model.Asset) error {
	return nil
}
