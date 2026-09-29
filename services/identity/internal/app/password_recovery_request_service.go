package app

import (
	"context"
	"errors"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var ErrPasswordRecoveryRequestUnavailable = errors.New("password recovery request unavailable")

type PasswordRecoveryRequestService struct {
	repository  outbound.PasswordRecoveryRequestRepository
	protector   outbound.DeliveryProtector
	sourceLimit func(context.Context, string) error
	emailLimit  func(context.Context, string) error
	verifierKey []byte
}

var _ inbound.PasswordRecoveryRequestService = (*PasswordRecoveryRequestService)(nil)

func NewPasswordRecoveryRequestService(repository outbound.PasswordRecoveryRequestRepository, protector outbound.DeliveryProtector,
	sourceLimit, emailLimit func(context.Context, string) error, verifierKey []byte,
) *PasswordRecoveryRequestService {
	return &PasswordRecoveryRequestService{repository: repository, protector: protector, sourceLimit: sourceLimit, emailLimit: emailLimit, verifierKey: verifierKey}
}

func (s *PasswordRecoveryRequestService) RequestPasswordResetCode(ctx context.Context, input inbound.PasswordRecoveryRequestInput) error {
	if s.repository == nil || s.protector == nil || s.sourceLimit == nil || s.emailLimit == nil || len(s.verifierKey) != 32 {
		return ErrPasswordRecoveryRequestUnavailable
	}
	email, err := domain.NormalizeEmail(input.Email)
	if err != nil {
		return err
	}
	started := time.Now()
	if err := s.sourceLimit(ctx, input.Source); err != nil {
		return err
	}
	if err := s.emailLimit(ctx, email); err != nil {
		return err
	}
	if err := s.repository.WithinPasswordRecoveryRequestTransaction(ctx, func(tx outbound.PasswordRecoveryRequestTransaction) error {
		return s.issue(ctx, tx, email)
	}); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return ErrPasswordRecoveryRequestUnavailable
	}
	if remaining := claimResponseFloor - time.Since(started); remaining > 0 {
		timer := time.NewTimer(remaining)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	return ctx.Err()
}

func (s *PasswordRecoveryRequestService) issue(ctx context.Context, tx outbound.PasswordRecoveryRequestTransaction, email string) error {
	account, found, err := tx.GetAccountForPasswordRecovery(ctx, email)
	if err != nil {
		return err
	}
	if !found || !account.EmailVerified || !account.HasPassword {
		return nil
	}
	allowed, err := tx.CanIssueCode(ctx, account.Subject)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	code, verifier, _, err := domain.NewChallenge(s.verifierKey, account.Subject, email, domain.PurposePasswordReset, time.Now())
	if err != nil {
		return err
	}
	if err := tx.ReplacePasswordResetChallenge(ctx, account.Subject); err != nil {
		return err
	}
	challengeID, err := tx.CreatePasswordResetChallenge(ctx, account.Subject, email, verifier)
	if err != nil {
		return err
	}
	material, err := s.protector.Protect(challengeID, string(domain.PurposePasswordReset), account.Subject, email, code)
	if err != nil {
		return err
	}
	if err := tx.StoreDelivery(ctx, challengeID, material); err != nil {
		return err
	}
	return tx.CreateOutboxEvent(ctx, challengeID)
}
