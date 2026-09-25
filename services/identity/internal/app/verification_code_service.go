package app

import (
	"context"
	"errors"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var (
	ErrEmailAlreadyVerified        = errors.New("email already verified")
	ErrVerificationCodeUnavailable = errors.New("verification code unavailable")
)

type VerificationCodeService struct {
	repository  outbound.AccountRepository
	protector   outbound.DeliveryProtector
	sourceLimit func(context.Context, string) error
	verifierKey []byte
}

var _ inbound.VerificationCodeService = (*VerificationCodeService)(nil)

func NewVerificationCodeService(repository outbound.AccountRepository, protector outbound.DeliveryProtector,
	sourceLimit func(context.Context, string) error, verifierKey []byte,
) *VerificationCodeService {
	return &VerificationCodeService{repository: repository, protector: protector, sourceLimit: sourceLimit, verifierKey: verifierKey}
}

func (s *VerificationCodeService) RequestEmailVerificationCode(ctx context.Context, input inbound.RequestEmailVerificationCodeInput) error {
	if input.Subject == "" || input.SessionID == "" {
		return outbound.ErrUnauthenticated
	}
	if s.repository == nil || s.protector == nil || s.sourceLimit == nil || len(s.verifierKey) != 32 {
		return ErrVerificationCodeUnavailable
	}
	if err := s.sourceLimit(ctx, input.Source); err != nil {
		return err
	}
	err := s.repository.WithinTransaction(ctx, func(tx outbound.AccountTransaction) error {
		return s.issue(ctx, tx, input)
	})
	switch {
	case err == nil, errors.Is(err, outbound.ErrUnauthenticated), errors.Is(err, ErrEmailAlreadyVerified), errors.Is(err, ErrRateLimited),
		errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return ErrVerificationCodeUnavailable
	}
}

func (s *VerificationCodeService) issue(ctx context.Context, tx outbound.AccountTransaction, input inbound.RequestEmailVerificationCodeInput) error {
	account, err := tx.GetActiveAccountForSession(ctx, input.Subject, input.SessionID)
	if err != nil {
		return err
	}
	if account.EmailVerified {
		return ErrEmailAlreadyVerified
	}
	allowed, err := tx.CanIssueCode(ctx, input.Subject)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrRateLimited
	}
	code, verifier, _, err := domain.NewChallenge(s.verifierKey, input.Subject, account.Email, domain.PurposeVerifyEmail, time.Now())
	if err != nil {
		return err
	}
	if err := tx.ReplaceVerificationChallenge(ctx, input.Subject); err != nil {
		return err
	}
	challengeID, err := tx.CreateChallenge(ctx, input.Subject, account.Email, verifier)
	if err != nil {
		return err
	}
	material, err := s.protector.Protect(challengeID, string(domain.PurposeVerifyEmail), input.Subject, account.Email, code)
	if err != nil {
		return err
	}
	if err := tx.StoreDelivery(ctx, challengeID, material); err != nil {
		return err
	}
	return tx.CreateOutboxEvent(ctx, challengeID)
}
