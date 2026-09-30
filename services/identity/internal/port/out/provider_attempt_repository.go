package outbound

import (
	"context"
	"time"
)

// ProviderAttempt binds one provider login attempt to its proofs. The attempt
// token and state are stored only as keyed verifiers.
type ProviderAttempt struct {
	Provider             string
	AttemptTokenVerifier [32]byte
	StateVerifier        [32]byte
	CodeVerifier         string
	Nonce                string
	CallbackURL          string
}

type ProviderAttemptRepository interface {
	// CreateProviderAttempt stores a new attempt and returns its expiry.
	CreateProviderAttempt(ctx context.Context, attempt ProviderAttempt) (time.Time, error)
}
