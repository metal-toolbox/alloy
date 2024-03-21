package types

import (
	"github.com/bmc-toolbox/common"
)

type BiosConfig map[string]string

type InventoryDevice struct {
	Inv     *common.Device `json:"inventory,omitempty"`
	BiosCfg BiosConfig     `json:"biosconfig,omitempty"`
}
