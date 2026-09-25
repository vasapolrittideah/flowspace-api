package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var ErrSignupUnavailable = errors.New("signup unavailable")

type SignupService struct {
	repository  outbound.AccountRepository
	signer      outbound.TokenSigner
	protector   outbound.DeliveryProtector
	limit       func(context.Context, string) error
	compromised func(context.Context, string) (bool, error)
	verifierKey []byte
}

var _ inbound.SignupService = (*SignupService)(nil)

func NewSignupService(repository outbound.AccountRepository, signer outbound.TokenSigner, protector outbound.DeliveryProtector,
	limit func(context.Context, string) error, compromised func(context.Context, string) (bool, error), verifierKey []byte,
) *SignupService {
	return &SignupService{
		repository: repository, signer: signer, protector: protector,
		limit: limit, compromised: compromised, verifierKey: verifierKey,
	}
}

func (s *SignupService) CreateAccount(ctx context.Context, request inbound.CreateAccountInput) (inbound.CreateAccountResult, error) {
	if s.repository == nil || s.signer == nil || s.protector == nil || s.limit == nil || len(s.verifierKey) != 32 {
		return inbound.CreateAccountResult{}, ErrSignupUnavailable
	}
	if err := s.limit(ctx, request.Source); err != nil {
		return inbound.CreateAccountResult{}, err
	}
	email, err := domain.NormalizeEmail(request.Email)
	if err != nil {
		return inbound.CreateAccountResult{}, err
	}
	hash, err := domain.HashPassword(ctx, request.Password, s.compromised)
	if err != nil {
		return inbound.CreateAccountResult{}, err
	}
	subject, err := uuid.NewRandom()
	if err != nil {
		return inbound.CreateAccountResult{}, ErrSignupUnavailable
	}
	code, verifier, _, err := domain.NewChallenge(s.verifierKey, subject.String(), email, domain.PurposeVerifyEmail, time.Now())
	if err != nil {
		return inbound.CreateAccountResult{}, ErrSignupUnavailable
	}
	var result inbound.CreateAccountResult
	err = s.repository.WithinTransaction(ctx, func(tx outbound.AccountTransaction) error {
		var recordErr error
		result, recordErr = s.createRecords(ctx, tx, subject.String(), email, hash, code, verifier)
		return recordErr
	})
	if err != nil {
		if errors.Is(err, outbound.ErrAccountExists) {
			return inbound.CreateAccountResult{}, outbound.ErrAccountExists
		}
		return inbound.CreateAccountResult{}, ErrSignupUnavailable
	}
	return result, nil
}

func (s *SignupService) createRecords(ctx context.Context, tx outbound.AccountTransaction,
	subject, email, hash, code string, verifier [32]byte,
) (inbound.CreateAccountResult, error) {
	if err := tx.CreateAccount(ctx, subject, email, hash); err != nil {
		return inbound.CreateAccountResult{}, err
	}
	tokens, err := NewSessionService(tx, s.signer).Issue(ctx, subject)
	if err != nil {
		return inbound.CreateAccountResult{}, err
	}
	challengeID, err := tx.CreateChallenge(ctx, subject, email, verifier)
	if err != nil {
		return inbound.CreateAccountResult{}, err
	}
	material, err := s.protector.Protect(challengeID, string(domain.PurposeVerifyEmail), subject, email, code)
	if err != nil {
		return inbound.CreateAccountResult{}, err
	}
	if err := tx.StoreDelivery(ctx, challengeID, material); err != nil {
		return inbound.CreateAccountResult{}, err
	}
	if err := tx.CreateOutboxEvent(ctx, challengeID); err != nil {
		return inbound.CreateAccountResult{}, err
	}
	return inbound.CreateAccountResult{
		Subject: subject, AccessToken: tokens.AccessToken,
		RefreshToken: tokens.RefreshToken, AccessTokenExpiresAt: tokens.AccessTokenExpiresAt,
	}, nil
}
