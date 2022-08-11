package fixtures

import (
	"context"
	"net"

	"github.com/metal-toolbox/alloy/internal/model"
)

var (
	MockAssets = map[string]*model.Asset{
		"foo": {
			ID:          "foo",
			BMCAddress:  net.ParseIP("127.0.0.1"),
			BMCUsername: "foo",
			BMCPassword: "bar",
		},
		"bar": {
			ID:          "bar",
			BMCAddress:  net.ParseIP("127.0.0.2"),
			BMCUsername: "foo",
			BMCPassword: "bar",
		},
		"borky": {
			ID:          "",
			BMCAddress:  net.ParseIP(""),
			BMCUsername: "foo",
			BMCPassword: "bar",
		},
	}
)

// MockAssetGetter mocks an asset acquirer
type MockAssetGetter struct {
	ch     chan<- *model.Asset
	assets map[string]*model.Asset
}

// NewMockAssetGetter returns a mock asset Getter that writes the given []*model.Asset to the given channel
func NewMockAssetGetter(ch chan<- *model.Asset, assets map[string]*model.Asset) *MockAssetGetter {
	return &MockAssetGetter{ch: ch, assets: assets}
}

// Listall implements the asset Getter interface, sending mock assets over the asset channel
func (m *MockAssetGetter) ListAll(ctx context.Context) {
	for _, asset := range m.assets {
		m.ch <- asset
	}

	close(m.ch)
}
