package outbound

import "context"

type PasswordRecoveryAccount struct {
	Subject       string
	EmailVerified bool
	HasPassword   bool
}

type PasswordResetCodeTransaction interface {
	GetAccountForPasswordRecovery(ctx context.Context, email string) (PasswordRecoveryAccount, bool, error)
	CanIssueCode(ctx context.Context, subject string) (bool, error)
	ReplacePasswordResetChallenge(ctx context.Context, subject string) error
	CreatePasswordResetChallenge(ctx context.Context, subject, email string, verifier [32]byte) (string, error)
	StoreDelivery(ctx context.Context, challengeID string, material DeliveryMaterial) error
	CreateOutboxEvent(ctx context.Context, challengeID string) error
}

type PasswordResetCodeRepository interface {
	WithinRequestPasswordResetCodeTransaction(ctx context.Context, fn func(PasswordResetCodeTransaction) error) error
}
