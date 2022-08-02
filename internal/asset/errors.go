package asset

import "errors"

var (
	ErrGetter = errors.New("asset getter error")
	ErrConfig = errors.New("required config not set")
)
