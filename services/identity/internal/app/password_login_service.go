package app

import (
	"context"
	"errors"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrLoginUnavailable   = errors.New("password login unavailable")
)

const unknownAccountVerifier = "$argon2id$v=19$m=19456,t=2,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

type PasswordLoginService struct {
	repository outbound.PasswordLoginRepository
	signer     outbound.TokenSigner
	limit      func(context.Context, string, string) error
	hashSlots  chan struct{}
	verify     func(context.Context, string, string) (bool, error)
}

var _ inbound.PasswordLoginService = (*PasswordLoginService)(nil)

func NewPasswordLoginService(repository outbound.PasswordLoginRepository, signer outbound.TokenSigner,
	limit func(context.Context, string, string) error,
) *PasswordLoginService {
	return &PasswordLoginService{
		repository: repository, signer: signer, limit: limit,
		hashSlots: make(chan struct{}, 2), verify: domain.VerifyPassword,
	}
}

func (s *PasswordLoginService) CreatePasswordSession(ctx context.Context, input inbound.CreatePasswordSessionInput) (inbound.CreatePasswordSessionResult, error) {
	if s.repository == nil || s.signer == nil || s.limit == nil {
		return inbound.CreatePasswordSessionResult{}, ErrLoginUnavailable
	}
	email, err := domain.NormalizeEmail(input.Email)
	if err != nil {
		return inbound.CreatePasswordSessionResult{}, err
	}
	password, err := domain.NormalizeLoginPassword(input.Password)
	if err != nil {
		return inbound.CreatePasswordSessionResult{}, err
	}
	if err := s.limit(ctx, input.Source, email); err != nil {
		return inbound.CreatePasswordSessionResult{}, err
	}
	account, found, err := s.repository.FindPasswordAccount(ctx, email)
	if err != nil {
		return inbound.CreatePasswordSessionResult{}, ErrLoginUnavailable
	}
	hash := unknownAccountVerifier
	if found {
		hash = account.PasswordHash
	}
	matched, err := s.checkPassword(ctx, password, hash)
	if err != nil {
		return inbound.CreatePasswordSessionResult{}, err
	}
	if !found || !matched {
		return inbound.CreatePasswordSessionResult{}, ErrInvalidCredentials
	}
	return s.issue(ctx, account)
}

func (s *PasswordLoginService) checkPassword(ctx context.Context, password, hash string) (bool, error) {
	select {
	case s.hashSlots <- struct{}{}:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	matched, err := s.verify(ctx, password, hash)
	<-s.hashSlots
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, err
		}
		return false, ErrLoginUnavailable
	}
	return matched, nil
}

func (s *PasswordLoginService) issue(ctx context.Context, account outbound.PasswordAccount) (inbound.CreatePasswordSessionResult, error) {
	var result inbound.CreatePasswordSessionResult
	err := s.repository.WithinPasswordSessionTransaction(ctx, func(tx outbound.PasswordSessionTransaction) error {
		current, err := tx.LockPasswordAccount(ctx, account.Subject)
		if errors.Is(err, outbound.ErrUnauthenticated) {
			return ErrInvalidCredentials
		}
		if err != nil {
			return err
		}
		if current.Subject != account.Subject || current.PasswordHash != account.PasswordHash {
			return ErrInvalidCredentials
		}
		tokens, err := NewSessionService(tx, s.signer).Issue(ctx, account.Subject)
		if err != nil {
			return err
		}
		result = inbound.CreatePasswordSessionResult{
			Subject: account.Subject, EmailVerified: current.EmailVerified,
			AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken,
			AccessTokenExpiresAt: tokens.AccessTokenExpiresAt, RefreshTokenExpiresAt: tokens.RefreshTokenExpiresAt,
			SessionExpiresAt: tokens.SessionExpiresAt,
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return inbound.CreatePasswordSessionResult{}, ErrInvalidCredentials
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return inbound.CreatePasswordSessionResult{}, err
		}
		return inbound.CreatePasswordSessionResult{}, ErrLoginUnavailable
	}
	return result, nil
}
