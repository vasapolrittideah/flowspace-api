package outbound

import "context"

type TokenVerifier interface {
	VerifyToken(context.Context, string) (string, error)
}
