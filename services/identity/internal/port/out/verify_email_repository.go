package outbound

import "context"

type VerificationTransaction interface {
	GetActiveAccountForSession(ctx context.Context, subject, sessionID string) (AccountState, error)
	GetCurrentVerificationChallenge(ctx context.Context, subject string) (ChallengeState, bool, error)
	IncrementChallengeWrongGuess(ctx context.Context, challengeID string) error
	ConsumeChallenge(ctx context.Context, challengeID string) (bool, error)
	MarkEmailVerified(ctx context.Context, subject string) (bool, error)
}

type VerifyEmailRepository interface {
	WithinVerificationTransaction(ctx context.Context, fn func(VerificationTransaction) error) error
}
