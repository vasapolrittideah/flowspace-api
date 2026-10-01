package outbound

import (
	"context"
	"errors"
	"time"
)

var (
	ErrAccountExists   = errors.New("account already exists")
	ErrUnauthenticated = errors.New("unauthenticated")
)

type AccountState struct {
	Email         string
	EmailVerified bool
}

type ChallengeState struct {
	ID           string
	Email        string
	Verifier     [32]byte
	WrongGuesses int16
	ExpiresAt    time.Time
}

type ClaimAccount struct {
	Subject       string
	EmailVerified bool
}

type DeliveryMaterial struct {
	KeyVersion int32
	Nonce      []byte
	Ciphertext []byte
}

type AccountTransaction interface {
	SessionRepository
	VerificationCodeDeliveryTransaction
	CreateAccount(ctx context.Context, subject, email, passwordHash string) error
}

type AccountRepository interface {
	WithinTransaction(ctx context.Context, fn func(AccountTransaction) error) error
}
