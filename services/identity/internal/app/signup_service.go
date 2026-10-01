package app

import (
	"context"
	"errors"

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
	var result inbound.CreateAccountResult
	err = s.repository.WithinTransaction(ctx, func(tx outbound.AccountTransaction) error {
		var recordErr error
		result, recordErr = s.createRecords(ctx, tx, subject.String(), email, hash)
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

func (s *SignupService) createRecords(ctx context.Context, tx outbound.AccountTransaction, subject, email, hash string) (inbound.CreateAccountResult, error) {
	if err := tx.CreateAccount(ctx, subject, email, hash); err != nil {
		return inbound.CreateAccountResult{}, err
	}
	tokens, err := NewSessionService(tx, s.signer).Issue(ctx, subject)
	if err != nil {
		return inbound.CreateAccountResult{}, err
	}
	if err := queueVerificationCode(ctx, tx, s.protector, s.verifierKey, subject, email); err != nil {
		return inbound.CreateAccountResult{}, err
	}
	return inbound.CreateAccountResult{
		Subject: subject, AccessToken: tokens.AccessToken,
		RefreshToken: tokens.RefreshToken, AccessTokenExpiresAt: tokens.AccessTokenExpiresAt,
	}, nil
}
