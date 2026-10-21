package app

import (
	"context"
	"errors"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

// ProviderReauthenticationService reuses the provider clients, attempts, and
// callback of provider login for a session that is already signed in.
type ProviderReauthenticationService struct {
	login        *ProviderLoginService
	repository   outbound.ProviderReauthenticationRepository
	failureLimit func(context.Context, string) error
}

var _ inbound.ProviderReauthenticationService = (*ProviderReauthenticationService)(nil)

// NewProviderReauthenticationService counts each rejected handoff from a
// source with failureLimit, as provider login does.
func NewProviderReauthenticationService(login *ProviderLoginService, repository outbound.ProviderReauthenticationRepository,
	failureLimit func(context.Context, string) error,
) *ProviderReauthenticationService {
	return &ProviderReauthenticationService{login: login, repository: repository, failureLimit: failureLimit}
}

func (s *ProviderReauthenticationService) StartProviderReauthentication(ctx context.Context, input inbound.StartProviderReauthenticationInput,
) (inbound.StartProviderLoginResult, error) {
	if input.Subject == "" || input.SessionID == "" {
		return inbound.StartProviderLoginResult{}, outbound.ErrUnauthenticated
	}
	if s.login == nil || s.repository == nil {
		return inbound.StartProviderLoginResult{}, ErrReauthenticationUnavailable
	}
	var result inbound.StartProviderLoginResult
	_, err := s.repository.WithinProviderReauthenticationTransaction(ctx, func(tx outbound.ProviderReauthenticationTransaction) error {
		if _, _, err := tx.LockSessionForReauthentication(ctx, input.Subject, input.SessionID); err != nil {
			return err
		}
		var err error
		result, err = s.login.start(ctx, inbound.StartProviderLoginInput{Provider: input.Provider, Source: input.Source}, tx,
			outbound.ProviderAttempt{Subject: input.Subject, SessionID: input.SessionID})
		return err
	})
	if err != nil {
		return inbound.StartProviderLoginResult{}, providerReauthenticationError(err)
	}
	return result, nil
}

func (s *ProviderReauthenticationService) CompleteProviderReauthentication(ctx context.Context, input inbound.CompleteProviderReauthenticationInput,
) (inbound.CompleteProviderReauthenticationResult, error) {
	if input.Subject == "" || input.SessionID == "" {
		return inbound.CompleteProviderReauthenticationResult{}, outbound.ErrUnauthenticated
	}
	if !domain.ValidProviderSecret(input.AttemptToken) {
		return inbound.CompleteProviderReauthenticationResult{}, domain.ErrInvalidAttemptToken
	}
	if !domain.ValidProviderSecret(input.HandoffCode) {
		return inbound.CompleteProviderReauthenticationResult{}, domain.ErrInvalidHandoffCode
	}
	if s.login == nil || s.login.repository == nil || s.repository == nil || s.failureLimit == nil || len(s.login.verifierKey) != 32 {
		return inbound.CompleteProviderReauthenticationResult{}, ErrReauthenticationUnavailable
	}
	attempt := domain.ProviderSecretVerifier(s.login.verifierKey, domain.ProviderSecretAttemptToken, input.AttemptToken)
	handoff := domain.ProviderSecretVerifier(s.login.verifierKey, domain.ProviderSecretHandoffCode, input.HandoffCode)
	linked := false
	window, err := s.repository.WithinProviderReauthenticationTransaction(ctx, func(tx outbound.ProviderReauthenticationTransaction) error {
		var err error
		linked, err = s.claim(ctx, tx, input, attempt, handoff)
		return err
	})
	switch {
	case errors.Is(err, ErrProviderHandoffRejected):
		return inbound.CompleteProviderReauthenticationResult{}, providerReauthenticationError(s.login.rejectHandoff(ctx, attempt, input.Source, s.failureLimit))
	case err != nil:
		return inbound.CompleteProviderReauthenticationResult{}, providerReauthenticationError(err)
	case !linked:
		return inbound.CompleteProviderReauthenticationResult{}, ErrProviderAccountUnavailable
	}
	return inbound.CompleteProviderReauthenticationResult{ExpiresAt: window.ExpiresAt()}, nil
}

// claim claims the provider result of the session and opens its window when
// the provider identity is linked to the subject. An unlinked result stays
// claimed, so the caller cannot retry it.
func (s *ProviderReauthenticationService) claim(ctx context.Context, tx outbound.ProviderReauthenticationTransaction,
	input inbound.CompleteProviderReauthenticationInput, attempt, handoff [32]byte,
) (bool, error) {
	if _, _, err := tx.LockSessionForReauthentication(ctx, input.Subject, input.SessionID); err != nil {
		return false, err
	}
	result, claimed, err := tx.ClaimProviderReauthenticationResult(ctx, input.Subject, input.SessionID, attempt, handoff)
	if err != nil {
		return false, err
	}
	if !claimed {
		return false, ErrProviderHandoffRejected
	}
	linked, err := tx.HasProviderLink(ctx, input.Subject, result.Provider, result.Subject)
	if err != nil || !linked {
		return false, err
	}
	_, err = openWindow(ctx, tx, input.Subject, input.SessionID)
	return err == nil, err
}

func providerReauthenticationError(err error) error {
	switch {
	case errors.Is(err, outbound.ErrReauthenticationRequired):
		return outbound.ErrUnauthenticated
	case errors.Is(err, ErrProviderHandoffRejected), errors.Is(err, domain.ErrInvalidProvider), errors.Is(err, outbound.ErrUnauthenticated),
		errors.Is(err, ErrRateLimited), errors.Is(err, ErrLimitUnavailable), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return ErrReauthenticationUnavailable
	}
}
