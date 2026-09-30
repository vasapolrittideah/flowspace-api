package app

import (
	"context"
	"errors"
	"net/url"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var ErrProviderLoginUnavailable = errors.New("provider login unavailable")

// ProviderClient is the registered OAuth client for one provider.
type ProviderClient struct {
	ClientID    string
	CallbackURL string
}

var providerAuthorization = map[domain.Provider]struct{ endpoint, scope string }{
	domain.ProviderGoogle: {"https://accounts.google.com/o/oauth2/v2/auth", "openid email"},
	domain.ProviderGitHub: {"https://github.com/login/oauth/authorize", "user:email"},
}

type ProviderLoginService struct {
	repository  outbound.ProviderAttemptRepository
	sourceLimit func(context.Context, string) error
	verifierKey []byte
	clients     map[domain.Provider]ProviderClient
}

var _ inbound.ProviderLoginService = (*ProviderLoginService)(nil)

func NewProviderLoginService(repository outbound.ProviderAttemptRepository, sourceLimit func(context.Context, string) error,
	verifierKey []byte, clients map[domain.Provider]ProviderClient,
) *ProviderLoginService {
	return &ProviderLoginService{repository: repository, sourceLimit: sourceLimit, verifierKey: verifierKey, clients: clients}
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
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return inbound.StartProviderLoginResult{}, err
		}
		return inbound.StartProviderLoginResult{}, ErrProviderLoginUnavailable
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
