package fleetdb

import "github.com/pkg/errors"

var (
	ErrSlugs                  = errors.New("slugs error")
	ErrFleetDBRegisterChanges = errors.New("error in FleetDB API register changes")
	ErrAssetObject            = errors.New("asset object error")
	ErrAssetObjectConversion  = errors.New("error converting asset object")
	ErrFleetDBAPIObject       = errors.New("serverService object error")
	ErrChangeList             = errors.New("error building change list")
	ErrFleetDBAttrObject      = errors.New("error in FleetDB API attribute object")
)
