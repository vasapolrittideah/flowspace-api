package outbound

import (
	"context"
)

type VerificationCodeIssueTransaction interface {
	GetActiveAccountForSession(ctx context.Context, subject, sessionID string) (AccountState, error)
	CanIssueCode(ctx context.Context, subject string) (bool, error)
	ReplaceVerificationChallenge(ctx context.Context, subject string) error
	CreateChallenge(ctx context.Context, subject, email string, verifier [32]byte) (string, error)
	StoreDelivery(ctx context.Context, challengeID string, material DeliveryMaterial) error
	CreateOutboxEvent(ctx context.Context, challengeID string) error
}

type EmailVerificationCodeRepository interface {
	WithinVerificationCodeIssueTransaction(ctx context.Context, fn func(VerificationCodeIssueTransaction) error) error
}

type VerificationTransaction interface {
	GetActiveAccountForSession(ctx context.Context, subject, sessionID string) (AccountState, error)
	GetCurrentVerificationChallenge(ctx context.Context, subject string) (ChallengeState, bool, error)
	IncrementChallengeWrongGuess(ctx context.Context, challengeID string) error
	ConsumeChallenge(ctx context.Context, challengeID string) (bool, error)
	MarkEmailVerified(ctx context.Context, subject string) (bool, error)
}

type EmailVerificationRepository interface {
	WithinVerificationTransaction(ctx context.Context, fn func(VerificationTransaction) error) error
}
