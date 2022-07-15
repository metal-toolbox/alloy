package fixtures

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/metal-toolbox/alloy/internal/model"
)

// MockEMAPIClient mocks an EMAPI asset requestor
type MockEMAPIClient struct{}

// NewMockEMAPIClient returns a MockEMAPIClient that implements the emapiRequestor interface
func NewMockEMAPIClient() *MockEMAPIClient {
	return &MockEMAPIClient{}
}

func (c *MockEMAPIClient) AssetByID(ctx context.Context, id string) (*model.Asset, error) {
	if id == "borky" {
		return nil, errors.New("asset is missing an ID attribute")
	}

	return MockAssets[id], nil
}

func (c *MockEMAPIClient) AssetsByOffsetLimit(ctx context.Context, offset, limit int) ([]*model.Asset, int, error) {
	var total int

	totalEnv := os.Getenv("EMAPI_FIXTURE_TOTAL_ASSETS")
	if totalEnv != "" {
		total, _ = strconv.Atoi(totalEnv)
	} else {
		return nil, 0, errors.New("test fixture error, expected env var EMAPI_FIXTURE_TOTAL_ASSETS")
	}

	assets := []*model.Asset{}

	if offset == limit {
		return []*model.Asset{
			{
				ID:          fmt.Sprintf("bar-%d", offset),
				BMCAddress:  net.ParseIP(fmt.Sprintf("127.0.0.%d", offset)),
				BMCUsername: "foo",
				BMCPassword: "bar",
			},
		}, total, nil
	}

	i := offset
	for {
		if offset+limit == total {
			if i >= offset+limit+1 {
				break
			}
		} else {
			if i >= offset+limit {
				break
			}
		}

		assets = append(
			assets,
			&model.Asset{
				ID:          "bar-" + strconv.Itoa(i),
				BMCAddress:  net.ParseIP(fmt.Sprintf("127.0.0.%d", i)),
				BMCUsername: "foo",
				BMCPassword: "bar",
			})

		i++

	}

	return assets, total, nil
}
