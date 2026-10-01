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
}

var _ inbound.ProviderLoginService = (*ProviderLoginService)(nil)

func NewProviderLoginService(repository outbound.ProviderAttemptRepository, sourceLimit, callbackLimit func(context.Context, string) error,
	verifierKey []byte, clients map[domain.Provider]ProviderClient,
) *ProviderLoginService {
	return &ProviderLoginService{repository: repository, sourceLimit: sourceLimit, callbackLimit: callbackLimit, verifierKey: verifierKey, clients: clients}
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

func unavailable(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return ErrProviderLoginUnavailable
}
