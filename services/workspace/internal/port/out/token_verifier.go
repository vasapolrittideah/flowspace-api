package outbound

import "context"

type TokenVerifier interface {
	VerifyToken(ctx context.Context, rawToken string) (string, error)
}
