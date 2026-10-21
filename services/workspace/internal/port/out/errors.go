package outbound

import "errors"

var (
	ErrEmailUnverified     = errors.New("email unverified")
	ErrIdentityUnavailable = errors.New("identity unavailable")
	ErrUnauthenticated     = errors.New("unauthenticated")
)
