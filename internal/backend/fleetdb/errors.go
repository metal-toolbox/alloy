package fleetdb

import "github.com/pkg/errors"

var (
	ErrSlugs                 = errors.New("slugs error")
	ErrAssetObject           = errors.New("asset object error")
	ErrAssetObjectConversion = errors.New("error converting asset object")
	ErrFleetDBObject         = errors.New("fleetdb object error")
	ErrChangeList            = errors.New("error building change list")
)
