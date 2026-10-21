package outbound

import (
	"context"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
)

type ReauthenticationCodeIssueTransaction interface {
	ReauthenticationTransaction
	GetActiveAccountForSession(ctx context.Context, subject, sessionID string) (AccountState, error)
	GetLatestReauthenticationCodeIssue(ctx context.Context, subject string) (time.Time, error)
	CanIssueCode(ctx context.Context, subject string) (bool, error)
	ReplaceReauthenticationChallenge(ctx context.Context, subject string) error
	CreateReauthenticationChallenge(ctx context.Context, subject, email string, verifier [32]byte) (string, error)
	StoreDelivery(ctx context.Context, challengeID string, material DeliveryMaterial) error
	CreateOutboxEvent(ctx context.Context, challengeID string) error
}

type CodeReauthenticationTransaction interface {
	ReauthenticationTransaction
	GetActiveAccountForSession(ctx context.Context, subject, sessionID string) (AccountState, error)
	GetCurrentReauthenticationChallenge(ctx context.Context, subject string) (domain.Challenge, bool, error)
	IncrementChallengeWrongGuess(ctx context.Context, challengeID string) error
	ConsumeChallenge(ctx context.Context, challengeID string) (bool, error)
	ReauthenticateSession(ctx context.Context, subject, sessionID string) error
}
type CodeReauthenticationRepository interface {
	WithinReauthenticationCodeIssueTransaction(ctx context.Context, fn func(ReauthenticationCodeIssueTransaction) error) error
	WithinCodeReauthenticationTransaction(ctx context.Context, fn func(CodeReauthenticationTransaction) error) (domain.ReauthenticationWindow, error)
}
