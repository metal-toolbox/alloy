package device

import (
	"context"

	"github.com/metal-toolbox/alloy/internal/model"
)

// NewMockDeviceQueryor returns a mock device.Queryor implementation.
func NewMockDeviceQueryor(kind model.AppKind) Queryor {
	return &MockDeviceQueryor{}
}

type MockDeviceQueryor struct{}

func (m *MockDeviceQueryor) Inventory(ctx context.Context, asset *model.Asset) error {
	return nil
}

func (m *MockDeviceQueryor) BiosConfiguration(ctx context.Context, asset *model.Asset) error {
	return nil
}
