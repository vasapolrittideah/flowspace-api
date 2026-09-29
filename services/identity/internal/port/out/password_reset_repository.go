package outbound

import "context"

type PasswordResetTransaction interface {
	GetAccountForPasswordRecovery(ctx context.Context, email string) (PasswordRecoveryAccount, bool, error)
	GetCurrentPasswordResetChallenge(ctx context.Context, subject string) (ChallengeState, bool, error)
	IncrementChallengeWrongGuess(ctx context.Context, challengeID string) error
	ConsumeChallenge(ctx context.Context, challengeID string) (bool, error)
	ReplacePasswordResetChallenge(ctx context.Context, subject string) error
	DeleteChallengeDelivery(ctx context.Context, challengeID string) error
	UpdatePasswordHash(ctx context.Context, subject, hash string) error
	RevokePasswordSessions(ctx context.Context, subject string) error
	QueuePasswordChangeNotice(ctx context.Context, subject string) error
}

type PasswordResetRepository interface {
	FindPasswordAccount(ctx context.Context, email string) (PasswordAccount, bool, error)
	WithinPasswordResetTransaction(ctx context.Context, fn func(PasswordResetTransaction) error) error
}
