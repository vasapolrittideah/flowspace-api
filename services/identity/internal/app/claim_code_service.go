package app

import (
	"context"
	"errors"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var ErrClaimCodeUnavailable = errors.New("claim code unavailable")

// ponytail: This fixed floor masks paths under 150 ms; measure and raise it if eligible requests exceed that time.
const claimResponseFloor = 150 * time.Millisecond

type ClaimCodeService struct {
	repository  outbound.ClaimCodeRepository
	protector   outbound.DeliveryProtector
	sourceLimit func(context.Context, string) error
	verifierKey []byte
}

var _ inbound.ClaimCodeService = (*ClaimCodeService)(nil)

func NewClaimCodeService(repository outbound.ClaimCodeRepository, protector outbound.DeliveryProtector,
	sourceLimit func(context.Context, string) error, verifierKey []byte,
) *ClaimCodeService {
	return &ClaimCodeService{repository: repository, protector: protector, sourceLimit: sourceLimit, verifierKey: verifierKey}
}

func (s *ClaimCodeService) RequestUnverifiedAccountClaimCode(ctx context.Context, input inbound.RequestClaimCodeInput) error {
	if s.repository == nil || s.protector == nil || s.sourceLimit == nil || len(s.verifierKey) != 32 {
		return ErrClaimCodeUnavailable
	}
	started := time.Now()
	if err := s.sourceLimit(ctx, input.Source); err != nil {
		return err
	}
	email, err := domain.NormalizeEmail(input.Email)
	if err != nil {
		return err
	}
	if err := s.repository.WithinClaimCodeTransaction(ctx, func(tx outbound.ClaimCodeTransaction) error {
		return s.issue(ctx, tx, email)
	}); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return ErrClaimCodeUnavailable
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

func (s *ClaimCodeService) issue(ctx context.Context, tx outbound.ClaimCodeTransaction, email string) error {
	account, found, err := tx.GetAccountForClaim(ctx, email)
	if err != nil {
		return err
	}
	if !found || account.EmailVerified {
		return nil
	}
	allowed, err := tx.CanIssueCode(ctx, account.Subject)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	code, verifier, _, err := domain.NewChallenge(s.verifierKey, account.Subject, email, domain.PurposeClaimAccount, time.Now())
	if err != nil {
		return err
	}
	if err := tx.ReplaceClaimChallenge(ctx, account.Subject); err != nil {
		return err
	}
	challengeID, err := tx.CreateClaimChallenge(ctx, account.Subject, email, verifier)
	if err != nil {
		return err
	}
	material, err := s.protector.Protect(challengeID, string(domain.PurposeClaimAccount), account.Subject, email, code)
	if err != nil {
		return err
	}
	if err := tx.StoreDelivery(ctx, challengeID, material); err != nil {
		return err
	}
	return tx.CreateOutboxEvent(ctx, challengeID)
}
