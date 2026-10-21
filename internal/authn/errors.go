package authn

import "errors"

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrUnavailable     = errors.New("identity unavailable")
)
