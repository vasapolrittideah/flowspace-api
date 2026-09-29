package outbound

import "context"

type ClaimCodeTransaction interface {
	GetAccountForClaim(ctx context.Context, email string) (ClaimAccount, bool, error)
	CanIssueCode(ctx context.Context, subject string) (bool, error)
	ReplaceClaimChallenge(ctx context.Context, subject string) error
	CreateClaimChallenge(ctx context.Context, subject, email string, verifier [32]byte) (string, error)
	StoreDelivery(ctx context.Context, challengeID string, material DeliveryMaterial) error
	CreateOutboxEvent(ctx context.Context, challengeID string) error
}

type ClaimCodeRepository interface {
	WithinClaimCodeTransaction(ctx context.Context, fn func(ClaimCodeTransaction) error) error
}
