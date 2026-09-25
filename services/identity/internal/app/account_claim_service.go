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

var (
	ErrInvalidClaimCode = errors.New("invalid claim code")
	ErrClaimUnavailable = errors.New("account claim unavailable")
)

type AccountClaimService struct {
	repository   outbound.AccountClaimRepository
	signer       outbound.TokenSigner
	sourceLimit  func(context.Context, string) error
	accountLimit func(context.Context, string) error
	compromised  func(context.Context, string) (bool, error)
	verifierKey  []byte
}

var _ inbound.AccountClaimService = (*AccountClaimService)(nil)

func NewAccountClaimService(repository outbound.AccountClaimRepository, signer outbound.TokenSigner,
	sourceLimit, accountLimit func(context.Context, string) error,
	compromised func(context.Context, string) (bool, error), verifierKey []byte,
) *AccountClaimService {
	return &AccountClaimService{
		repository: repository, signer: signer, sourceLimit: sourceLimit, accountLimit: accountLimit,
		compromised: compromised, verifierKey: verifierKey,
	}
}

func (s *AccountClaimService) ClaimUnverifiedAccount(ctx context.Context, input inbound.ClaimAccountInput) (inbound.ClaimAccountResult, error) {
	if !s.ready() {
		return inbound.ClaimAccountResult{}, ErrClaimUnavailable
	}
	if err := s.sourceLimit(ctx, input.Source); err != nil {
		return inbound.ClaimAccountResult{}, err
	}
	email, err := domain.NormalizeEmail(input.Email)
	if err != nil {
		return inbound.ClaimAccountResult{}, err
	}
	hash, err := domain.HashPassword(ctx, input.NewPassword, s.compromised)
	if err != nil {
		return inbound.ClaimAccountResult{}, claimFailure(err)
	}
	account, found, err := s.repository.FindAccountForClaim(ctx, email)
	if err != nil {
		return inbound.ClaimAccountResult{}, claimFailure(err)
	}
	if !found || account.EmailVerified {
		return inbound.ClaimAccountResult{}, ErrInvalidClaimCode
	}
	if err := s.accountLimit(ctx, account.Subject); err != nil {
		return inbound.ClaimAccountResult{}, claimFailure(err)
	}
	subject, err := uuid.NewRandom()
	if err != nil {
		return inbound.ClaimAccountResult{}, ErrClaimUnavailable
	}
	var result inbound.ClaimAccountResult
	invalid := false
	err = s.repository.WithinAccountClaimTransaction(ctx, func(tx outbound.AccountClaimTransaction) error {
		var stepErr error
		result, invalid, stepErr = s.claim(ctx, tx, email, input.Code, hash, subject.String())
		return stepErr
	})
	if err != nil {
		return inbound.ClaimAccountResult{}, claimFailure(err)
	}
	if invalid {
		return inbound.ClaimAccountResult{}, ErrInvalidClaimCode
	}
	return result, nil
}

func (s *AccountClaimService) ready() bool {
	return s.repository != nil && s.signer != nil && s.sourceLimit != nil && s.accountLimit != nil && len(s.verifierKey) == 32
}

func (s *AccountClaimService) claim(ctx context.Context, tx outbound.AccountClaimTransaction, email, code, hash, newSubject string) (inbound.ClaimAccountResult, bool, error) {
	account, found, err := tx.GetAccountForClaim(ctx, email)
	if err != nil {
		return inbound.ClaimAccountResult{}, false, err
	}
	if !found || account.EmailVerified {
		return inbound.ClaimAccountResult{}, true, nil
	}
	challenge, found, err := tx.GetCurrentClaimChallenge(ctx, account.Subject)
	if err != nil {
		return inbound.ClaimAccountResult{}, false, err
	}
	invalid, err := s.checkClaimChallenge(ctx, tx, account.Subject, email, code, challenge, found)
	if invalid || err != nil {
		return inbound.ClaimAccountResult{}, invalid, err
	}
	consumed, err := tx.ConsumeChallenge(ctx, challenge.ID)
	if err != nil || !consumed {
		return inbound.ClaimAccountResult{}, !consumed, err
	}
	result, err := s.replaceAccount(ctx, tx, account.Subject, newSubject, email, hash)
	return result, false, err
}

func (s *AccountClaimService) checkClaimChallenge(ctx context.Context, tx outbound.AccountClaimTransaction,
	subject, email, code string, challenge outbound.ChallengeState, found bool,
) (bool, error) {
	if !validVerificationCode(code) {
		return true, nil
	}
	if !found || challenge.Email != email || challenge.WrongGuesses >= 5 ||
		!domain.VerifyChallenge(s.verifierKey, subject, email, domain.PurposeClaimAccount,
			code, challenge.Verifier, challenge.ExpiresAt, time.Now()) {
		if found && challenge.Email == email && challenge.WrongGuesses < 5 && time.Now().Before(challenge.ExpiresAt) {
			return true, tx.IncrementChallengeWrongGuess(ctx, challenge.ID)
		}
		return true, nil
	}
	return false, nil
}

func (s *AccountClaimService) replaceAccount(ctx context.Context, tx outbound.AccountClaimTransaction,
	oldSubject, newSubject, email, hash string,
) (inbound.ClaimAccountResult, error) {
	retired, err := tx.RetireAndRevokeAccount(ctx, oldSubject)
	if err != nil || !retired {
		return inbound.ClaimAccountResult{}, ErrClaimUnavailable
	}
	if err := tx.CreateAccount(ctx, newSubject, email, hash); err != nil {
		return inbound.ClaimAccountResult{}, err
	}
	verified, err := tx.MarkEmailVerified(ctx, newSubject)
	if err != nil || !verified {
		return inbound.ClaimAccountResult{}, ErrClaimUnavailable
	}
	tokens, err := NewSessionService(tx, s.signer).Issue(ctx, newSubject)
	if err != nil {
		return inbound.ClaimAccountResult{}, err
	}
	return inbound.ClaimAccountResult{
		Subject: newSubject, AccessToken: tokens.AccessToken,
		RefreshToken: tokens.RefreshToken, AccessTokenExpiresAt: tokens.AccessTokenExpiresAt,
	}, nil
}

func claimFailure(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidPassword), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), errors.Is(err, ErrRateLimited):
		return err
	default:
		return ErrClaimUnavailable
	}
}
