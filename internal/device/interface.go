package device

import (
	"context"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/alloy/internal/device/inband"
	"github.com/metal-toolbox/alloy/internal/device/outofband"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	ErrQueryor = errors.New("device queryor error")
)

// Queryor interface defines methods to query a device for information.
type Queryor interface {
	// Inventory retrieves device component and firmware information
	Inventory(ctx context.Context, loginInfo *model.LoginInfo) (*common.Device, error)

	// BiosConfiguration retrieves the device component and firmware information
	// and updates the given asset object with the bios configuration.
	BiosConfiguration(ctx context.Context, loginInfo *model.LoginInfo) (map[string]string, error)
}

func NewQueryor(kind model.AppKind, logger *logrus.Logger) (Queryor, error) {
	switch kind {
	case model.AppKindInband:
		return inband.NewQueryor(logger), nil
	case model.AppKindOutOfBand:
		return outofband.NewQueryor(logger), nil
	default:
		return nil, errors.Wrap(ErrQueryor, "unsupported device queryor: "+string(kind))
	}
}
