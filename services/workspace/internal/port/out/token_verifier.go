package outbound

import (
	"context"
	"errors"
)

var (
	ErrEmailUnverified     = errors.New("email unverified")
	ErrIdentityUnavailable = errors.New("identity unavailable")
	ErrUnauthenticated     = errors.New("unauthenticated")
)

type TokenVerifier interface {
	VerifyToken(ctx context.Context, rawToken string) (string, error)
}
