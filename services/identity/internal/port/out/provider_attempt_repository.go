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

// ProviderAttemptProof is the stored proof for an attempt whose state was
// consumed by a callback.
type ProviderAttemptProof struct {
	ID           string
	CodeVerifier string
	Nonce        string
	CallbackURL  string
}

type ProviderAttemptRepository interface {
	// CreateProviderAttempt stores a new attempt and returns its expiry.
	CreateProviderAttempt(ctx context.Context, attempt ProviderAttempt) (time.Time, error)
	// ConsumeProviderState consumes the state of a live attempt for the
	// provider once. It reports false when no such attempt exists.
	ConsumeProviderState(ctx context.Context, provider string, stateVerifier [32]byte) (ProviderAttemptProof, bool, error)
	// RecordProviderResult stores the verified identity and the handoff code
	// verifier. It reports false when the attempt expired or already has a
	// result or failure.
	RecordProviderResult(ctx context.Context, attemptID string, identity ProviderIdentity, handoffVerifier [32]byte) (bool, error)
	// FailProviderAttempt records a failed callback for an attempt without a result.
	FailProviderAttempt(ctx context.Context, attemptID string) error
	// RecordFailedProviderHandoff counts a failed session claim against a live,
	// unclaimed attempt. The fifth failure exhausts the attempt.
	RecordFailedProviderHandoff(ctx context.Context, attemptVerifier [32]byte) error
}
