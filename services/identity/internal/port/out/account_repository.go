package outbound

import (
	"context"
	"errors"
)

var (
	ErrAccountExists   = errors.New("account already exists")
	ErrUnauthenticated = errors.New("unauthenticated")
)

type AccountState struct {
	Email         string
	EmailVerified bool
}

type DeliveryMaterial struct {
	KeyVersion int32
	Nonce      []byte
	Ciphertext []byte
}

type AccountTransaction interface {
	SessionRepository
	CreateAccount(ctx context.Context, subject, email, passwordHash string) error
	GetActiveAccountForSession(ctx context.Context, subject, sessionID string) (AccountState, error)
	CanIssueCode(ctx context.Context, subject string) (bool, error)
	ReplaceVerificationChallenge(ctx context.Context, subject string) error
	CreateChallenge(ctx context.Context, subject, email string, verifier [32]byte) (string, error)
	StoreDelivery(ctx context.Context, challengeID string, material DeliveryMaterial) error
	CreateOutboxEvent(ctx context.Context, challengeID string) error
}

type AccountRepository interface {
	WithinTransaction(ctx context.Context, fn func(AccountTransaction) error) error
}
