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
	CreateAccount(ctx context.Context, subject, email, passwordHash string) error
	GetActiveAccountForSession(ctx context.Context, subject, sessionID string) (AccountState, error)
	CanIssueCode(ctx context.Context, subject string) (bool, error)
	ReplaceVerificationChallenge(ctx context.Context, subject string) error
	CreateChallenge(ctx context.Context, subject, email string, verifier [32]byte) (string, error)
	StoreDelivery(ctx context.Context, challengeID string, material DeliveryMaterial) error
	CreateOutboxEvent(ctx context.Context, challengeID string) error
}

type VerificationTransaction interface {
	GetActiveAccountForSession(ctx context.Context, subject, sessionID string) (AccountState, error)
	GetCurrentVerificationChallenge(ctx context.Context, subject string) (ChallengeState, bool, error)
	IncrementChallengeWrongGuess(ctx context.Context, challengeID string) error
	ConsumeChallenge(ctx context.Context, challengeID string) (bool, error)
	MarkEmailVerified(ctx context.Context, subject string) (bool, error)
}

type ClaimCodeTransaction interface {
	GetAccountForClaim(ctx context.Context, email string) (ClaimAccount, bool, error)
	CanIssueCode(ctx context.Context, subject string) (bool, error)
	ReplaceClaimChallenge(ctx context.Context, subject string) error
	CreateClaimChallenge(ctx context.Context, subject, email string, verifier [32]byte) (string, error)
	StoreDelivery(ctx context.Context, challengeID string, material DeliveryMaterial) error
	CreateOutboxEvent(ctx context.Context, challengeID string) error
}

type AccountRepository interface {
	WithinTransaction(ctx context.Context, fn func(AccountTransaction) error) error
}

type VerificationCodeRepository interface {
	AccountRepository
	WithinVerificationTransaction(ctx context.Context, fn func(VerificationTransaction) error) error
}

type ClaimCodeRepository interface {
	WithinClaimCodeTransaction(ctx context.Context, fn func(ClaimCodeTransaction) error) error
}
