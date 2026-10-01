package app

import (
	"context"
	"errors"
	"net/url"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var (
	ErrProviderLoginUnavailable = errors.New("provider login unavailable")
	ErrInvalidProviderCallback  = errors.New("invalid provider callback")
	// ErrProviderHandoffRejected covers an unknown, pending, expired,
	// exhausted, failed, or claimed attempt alike.
	ErrProviderHandoffRejected    = errors.New("provider handoff rejected")
	ErrProviderAccountUnavailable = errors.New("provider account unavailable")
)

// ProviderClient is the registered OAuth client for one provider. Identity
// verifies callbacks; a provider without it can start but not complete login.
type ProviderClient struct {
	ClientID    string
	CallbackURL string
	Identity    outbound.ProviderIdentityVerifier
}

var providerAuthorization = map[domain.Provider]struct{ endpoint, scope string }{
	domain.ProviderGoogle: {"https://accounts.google.com/o/oauth2/v2/auth", "openid email"},
	domain.ProviderGitHub: {"https://github.com/login/oauth/authorize", "user:email"},
}

type ProviderLoginService struct {
	repository    outbound.ProviderAttemptRepository
	sourceLimit   func(context.Context, string) error
	callbackLimit func(context.Context, string) error
	verifierKey   []byte
	clients       map[domain.Provider]ProviderClient
	sessions      outbound.ProviderSessionRepository
	signer        outbound.TokenSigner
	failureLimit  func(context.Context, string) error
}

var _ inbound.ProviderLoginService = (*ProviderLoginService)(nil)

func NewProviderLoginService(repository outbound.ProviderAttemptRepository, sourceLimit, callbackLimit func(context.Context, string) error,
	verifierKey []byte, clients map[domain.Provider]ProviderClient,
) *ProviderLoginService {
	return &ProviderLoginService{repository: repository, sourceLimit: sourceLimit, callbackLimit: callbackLimit, verifierKey: verifierKey, clients: clients}
}

// WithSessions enables CreateProviderSession. The failure limit counts each
// rejected handoff from a source.
func (s *ProviderLoginService) WithSessions(sessions outbound.ProviderSessionRepository, signer outbound.TokenSigner,
	failureLimit func(context.Context, string) error,
) *ProviderLoginService {
	s.sessions, s.signer, s.failureLimit = sessions, signer, failureLimit
	return s
}

func (s *ProviderLoginService) StartProviderLogin(ctx context.Context, input inbound.StartProviderLoginInput) (inbound.StartProviderLoginResult, error) {
	provider := domain.Provider(input.Provider)
	authorization, known := providerAuthorization[provider]
	if !known {
		return inbound.StartProviderLoginResult{}, domain.ErrInvalidProvider
	}
	client, configured := s.clients[provider]
	if !configured || s.repository == nil || s.sourceLimit == nil || len(s.verifierKey) != 32 {
		return inbound.StartProviderLoginResult{}, ErrProviderLoginUnavailable
	}
	if err := s.sourceLimit(ctx, input.Source); err != nil {
		return inbound.StartProviderLoginResult{}, err
	}
	secrets, err := domain.NewProviderAttempt(provider)
	if err != nil {
		return inbound.StartProviderLoginResult{}, ErrProviderLoginUnavailable
	}
	expiresAt, err := s.repository.CreateProviderAttempt(ctx, outbound.ProviderAttempt{
		Provider:             string(provider),
		AttemptTokenVerifier: domain.ProviderSecretVerifier(s.verifierKey, domain.ProviderSecretAttemptToken, secrets.AttemptToken),
		StateVerifier:        domain.ProviderSecretVerifier(s.verifierKey, domain.ProviderSecretState, secrets.State),
		CodeVerifier:         secrets.CodeVerifier,
		Nonce:                secrets.Nonce,
		CallbackURL:          client.CallbackURL,
	})
	if err != nil {
		return inbound.StartProviderLoginResult{}, unavailable(err)
	}
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {client.ClientID},
		"redirect_uri":          {client.CallbackURL},
		"scope":                 {authorization.scope},
		"state":                 {secrets.State},
		"code_challenge":        {domain.ProviderCodeChallenge(secrets.CodeVerifier)},
		"code_challenge_method": {"S256"},
	}
	if secrets.Nonce != "" {
		query.Set("nonce", secrets.Nonce)
	}
	return inbound.StartProviderLoginResult{
		AuthorizationURL: authorization.endpoint + "?" + query.Encode(),
		AttemptToken:     secrets.AttemptToken,
		ExpiresAt:        expiresAt,
	}, nil
}

func (s *ProviderLoginService) CompleteProviderCallback(ctx context.Context, input inbound.CompleteProviderCallbackInput) (string, error) {
	if s.repository == nil || s.callbackLimit == nil || len(s.verifierKey) != 32 {
		return "", ErrProviderLoginUnavailable
	}
	// The limit counts every callback, including invalid ones.
	if err := s.callbackLimit(ctx, input.Source); err != nil {
		return "", err
	}
	provider := domain.Provider(input.Provider)
	if _, known := providerAuthorization[provider]; !known || input.State == "" {
		return "", ErrInvalidProviderCallback
	}
	identity := s.clients[provider].Identity
	if identity == nil {
		return "", ErrProviderLoginUnavailable
	}
	proof, found, err := s.repository.ConsumeProviderState(ctx, string(provider),
		domain.ProviderSecretVerifier(s.verifierKey, domain.ProviderSecretState, input.State))
	if err != nil {
		return "", unavailable(err)
	}
	if !found {
		return "", ErrInvalidProviderCallback
	}
	result, err := s.verify(ctx, identity, proof, input)
	if err != nil {
		// The state is already consumed, so a lost failure record cannot reopen the attempt.
		_ = s.repository.FailProviderAttempt(context.WithoutCancel(ctx), proof.ID)
		return "", err
	}
	code, err := domain.NewProviderHandoffCode()
	if err != nil {
		return "", ErrProviderLoginUnavailable
	}
	stored, err := s.repository.RecordProviderResult(ctx, proof.ID, result,
		domain.ProviderSecretVerifier(s.verifierKey, domain.ProviderSecretHandoffCode, code))
	if err != nil {
		return "", unavailable(err)
	}
	if !stored {
		return "", ErrInvalidProviderCallback
	}
	return code, nil
}

func (s *ProviderLoginService) verify(ctx context.Context, identity outbound.ProviderIdentityVerifier, proof outbound.ProviderAttemptProof,
	input inbound.CompleteProviderCallbackInput,
) (outbound.ProviderIdentity, error) {
	if input.Denied || input.Code == "" {
		return outbound.ProviderIdentity{}, ErrInvalidProviderCallback
	}
	result, err := identity.VerifyProviderIdentity(ctx, outbound.ProviderCodeExchange{
		Code: input.Code, CodeVerifier: proof.CodeVerifier, Nonce: proof.Nonce, CallbackURL: proof.CallbackURL,
	})
	switch {
	case err == nil && result.Subject != "":
		return result, nil
	case err == nil, errors.Is(err, domain.ErrInvalidProviderProof):
		return outbound.ProviderIdentity{}, ErrInvalidProviderCallback
	default:
		return outbound.ProviderIdentity{}, unavailable(err)
	}
}

func (s *ProviderLoginService) CreateProviderSession(ctx context.Context, input inbound.CreateProviderSessionInput) (inbound.CreateProviderSessionResult, error) {
	if !domain.ValidProviderSecret(input.AttemptToken) {
		return inbound.CreateProviderSessionResult{}, domain.ErrInvalidAttemptToken
	}
	if !domain.ValidProviderSecret(input.HandoffCode) {
		return inbound.CreateProviderSessionResult{}, domain.ErrInvalidHandoffCode
	}
	if s.repository == nil || s.sessions == nil || s.signer == nil || s.failureLimit == nil || len(s.verifierKey) != 32 {
		return inbound.CreateProviderSessionResult{}, ErrProviderLoginUnavailable
	}
	attempt := domain.ProviderSecretVerifier(s.verifierKey, domain.ProviderSecretAttemptToken, input.AttemptToken)
	handoff := domain.ProviderSecretVerifier(s.verifierKey, domain.ProviderSecretHandoffCode, input.HandoffCode)
	var result inbound.CreateProviderSessionResult
	linked := false
	err := s.sessions.WithinProviderSessionTransaction(ctx, func(tx outbound.ProviderSessionTransaction) error {
		var err error
		result, linked, err = s.claimSession(ctx, tx, attempt, handoff)
		return err
	})
	switch {
	case errors.Is(err, ErrProviderHandoffRejected):
		return inbound.CreateProviderSessionResult{}, s.rejectHandoff(ctx, attempt, input.Source)
	case err != nil:
		return inbound.CreateProviderSessionResult{}, unavailable(err)
	case !linked:
		return inbound.CreateProviderSessionResult{}, ErrProviderAccountUnavailable
	}
	return result, nil
}

// claimSession claims the provider result and issues a session for its linked
// account. An unlinked result stays claimed, so it cannot be retried.
func (s *ProviderLoginService) claimSession(ctx context.Context, tx outbound.ProviderSessionTransaction, attempt, handoff [32]byte,
) (inbound.CreateProviderSessionResult, bool, error) {
	claim, claimed, err := tx.ClaimProviderResult(ctx, attempt, handoff)
	if err != nil {
		return inbound.CreateProviderSessionResult{}, false, err
	}
	if !claimed {
		return inbound.CreateProviderSessionResult{}, false, ErrProviderHandoffRejected
	}
	account, found, err := tx.LockLinkedAccount(ctx, claim.Provider, claim.Subject)
	if err != nil || !found {
		return inbound.CreateProviderSessionResult{}, false, err
	}
	tokens, err := NewSessionService(tx, s.signer).Issue(ctx, account.Subject)
	if err != nil {
		return inbound.CreateProviderSessionResult{}, false, err
	}
	return inbound.CreateProviderSessionResult{
		Subject: account.Subject, EmailVerified: account.EmailVerified,
		AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken,
		AccessTokenExpiresAt: tokens.AccessTokenExpiresAt, RefreshTokenExpiresAt: tokens.RefreshTokenExpiresAt,
		SessionExpiresAt: tokens.SessionExpiresAt,
	}, true, nil
}

func (s *ProviderLoginService) rejectHandoff(ctx context.Context, attempt [32]byte, source string) error {
	if err := s.repository.RecordFailedProviderHandoff(ctx, attempt); err != nil {
		return unavailable(err)
	}
	if err := s.failureLimit(ctx, source); err != nil {
		return err
	}
	return ErrProviderHandoffRejected
}

func unavailable(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return ErrProviderLoginUnavailable
}
