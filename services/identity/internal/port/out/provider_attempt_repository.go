package outbound

import (
	"context"
	"time"
)

// ProviderAttemptProof is the stored proof for an attempt after a callback
// consumes its state. Reauthentication is true for an attempt with the
// re-authentication purpose.
type ProviderAttemptProof struct {
	ID               string
	Reauthentication bool
	CodeVerifier     string
	Nonce            string
	CallbackURL      string
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
