package outbound

import "context"

type VerificationCodeIssueTransaction interface {
	GetActiveAccountForSession(ctx context.Context, subject, sessionID string) (AccountState, error)
	CanIssueCode(ctx context.Context, subject string) (bool, error)
	ReplaceVerificationChallenge(ctx context.Context, subject string) error
	CreateChallenge(ctx context.Context, subject, email string, verifier [32]byte) (string, error)
	StoreDelivery(ctx context.Context, challengeID string, material DeliveryMaterial) error
	CreateOutboxEvent(ctx context.Context, challengeID string) error
}

type RequestEmailVerificationCodeRepository interface {
	WithinVerificationCodeIssueTransaction(ctx context.Context, fn func(VerificationCodeIssueTransaction) error) error
}
