package outbound

import "time"

type AccessTokenClaims struct {
	Subject   string
	SessionID string
	ID        string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type TokenSigner interface {
	Sign(claims AccessTokenClaims) (string, error)
}
