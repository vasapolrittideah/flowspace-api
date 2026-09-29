package outbound

import "context"

type AccountClaimTransaction interface {
	SessionRepository
	GetAccountForClaim(ctx context.Context, email string) (ClaimAccount, bool, error)
	GetCurrentClaimChallenge(ctx context.Context, subject string) (ChallengeState, bool, error)
	IncrementChallengeWrongGuess(ctx context.Context, challengeID string) error
	ConsumeChallenge(ctx context.Context, challengeID string) (bool, error)
	RetireAndRevokeAccount(ctx context.Context, subject string) (bool, error)
	CreateAccount(ctx context.Context, subject, email, passwordHash string) error
	MarkEmailVerified(ctx context.Context, subject string) (bool, error)
}

type AccountClaimRepository interface {
	FindAccountForClaim(ctx context.Context, email string) (ClaimAccount, bool, error)
	WithinAccountClaimTransaction(ctx context.Context, fn func(AccountClaimTransaction) error) error
}
