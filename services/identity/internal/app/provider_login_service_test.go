package app_test

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type providerAttemptRepository struct {
	attempts []outbound.ProviderAttempt
	err      error
}

func (r *providerAttemptRepository) CreateProviderAttempt(_ context.Context, attempt outbound.ProviderAttempt) (time.Time, error) {
	if r.err != nil {
		return time.Time{}, r.err
	}
	r.attempts = append(r.attempts, attempt)
	return time.Date(2026, 10, 1, 12, 10, 0, 0, time.UTC), nil
}

var providerClients = map[domain.Provider]app.ProviderClient{
	domain.ProviderGoogle: {ClientID: "google-client", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/google"},
	domain.ProviderGitHub: {ClientID: "github-client", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/github"},
}

func TestStartProviderLoginBindsProofsToOneAttempt(t *testing.T) {
	for _, test := range []struct {
		provider, endpoint, scope string
		nonce                     bool
	}{
		{"google", "https://accounts.google.com/o/oauth2/v2/auth", "openid email", true},
		{"github", "https://github.com/login/oauth/authorize", "user:email", false},
	} {
		t.Run(test.provider, func(t *testing.T) {
			repository := &providerAttemptRepository{}
			key := make([]byte, 32)
			var limited string
			service := app.NewProviderLoginService(repository, func(_ context.Context, source string) error {
				limited = source
				return nil
			}, key, providerClients)

			result, err := service.StartProviderLogin(t.Context(), inbound.StartProviderLoginInput{Provider: test.provider, Source: "192.0.2.1"})
			if err != nil {
				t.Fatal(err)
			}
			if limited != "192.0.2.1" || len(repository.attempts) != 1 || !result.ExpiresAt.Equal(time.Date(2026, 10, 1, 12, 10, 0, 0, time.UTC)) {
				t.Fatalf("limit source %q, %d attempts, expiry %v", limited, len(repository.attempts), result.ExpiresAt)
			}
			checkProviderAuthorization(t, result, repository.attempts[0], key, test.provider, test.endpoint, test.scope, test.nonce)
		})
	}
}

func checkProviderAuthorization(t *testing.T, result inbound.StartProviderLoginResult, attempt outbound.ProviderAttempt, key []byte,
	provider, endpoint, scope string, nonce bool,
) {
	t.Helper()
	authorization, err := url.Parse(result.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	query := authorization.Query()
	authorization.RawQuery = ""
	client := providerClients[domain.Provider(provider)]
	if authorization.String() != endpoint || query.Get("client_id") != client.ClientID ||
		query.Get("redirect_uri") != client.CallbackURL || attempt.CallbackURL != client.CallbackURL ||
		query.Get("response_type") != "code" || query.Get("scope") != scope || attempt.Provider != provider {
		t.Fatalf("authorization URL = %s", result.AuthorizationURL)
	}
	if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") != domain.ProviderCodeChallenge(attempt.CodeVerifier) ||
		query.Has("code_verifier") {
		t.Fatal("authorization URL does not bind the PKCE verifier")
	}
	checkProviderProofs(t, result, attempt, key, query, nonce)
}

func checkProviderProofs(t *testing.T, result inbound.StartProviderLoginResult, attempt outbound.ProviderAttempt, key []byte, query url.Values, nonce bool) {
	t.Helper()
	if attempt.StateVerifier != domain.ProviderSecretVerifier(key, domain.ProviderSecretState, query.Get("state")) ||
		attempt.AttemptTokenVerifier != domain.ProviderSecretVerifier(key, domain.ProviderSecretAttemptToken, result.AttemptToken) {
		t.Fatal("attempt does not store keyed verifiers of the state and attempt token")
	}
	if query.Get("state") == result.AttemptToken || query.Get("state") == "" || result.AttemptToken == "" {
		t.Fatal("attempt token is not separate from the state")
	}
	if nonce != (attempt.Nonce != "") || query.Get("nonce") != attempt.Nonce {
		t.Fatalf("nonce in URL %q, stored %q", query.Get("nonce"), attempt.Nonce)
	}
}

func TestStartProviderLoginRetryCreatesNewAttempt(t *testing.T) {
	repository := &providerAttemptRepository{}
	service := app.NewProviderLoginService(repository, func(context.Context, string) error { return nil }, make([]byte, 32), providerClients)
	first, err := service.StartProviderLogin(t.Context(), inbound.StartProviderLoginInput{Provider: "google", Source: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.StartProviderLogin(t.Context(), inbound.StartProviderLoginInput{Provider: "google", Source: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.attempts) != 2 || first.AttemptToken == second.AttemptToken || repository.attempts[0].StateVerifier == repository.attempts[1].StateVerifier {
		t.Fatal("retry did not create a separate attempt")
	}
}

func TestStartProviderLoginFailures(t *testing.T) {
	allow := func(context.Context, string) error { return nil }
	for _, test := range []struct {
		name     string
		provider string
		limit    func(context.Context, string) error
		clients  map[domain.Provider]app.ProviderClient
		storeErr error
		want     error
	}{
		{"unknown provider", "okta", allow, providerClients, nil, domain.ErrInvalidProvider},
		{"rate limited", "google", func(context.Context, string) error { return app.ErrRateLimited }, providerClients, nil, app.ErrRateLimited},
		{"limit unavailable", "google", func(context.Context, string) error { return app.ErrLimitUnavailable }, providerClients, nil, app.ErrLimitUnavailable},
		{"not configured", "github", allow, map[domain.Provider]app.ProviderClient{domain.ProviderGoogle: providerClients[domain.ProviderGoogle]}, nil, app.ErrProviderLoginUnavailable},
		{"store unavailable", "google", allow, providerClients, errors.New("database down"), app.ErrProviderLoginUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &providerAttemptRepository{err: test.storeErr}
			service := app.NewProviderLoginService(repository, test.limit, make([]byte, 32), test.clients)
			result, err := service.StartProviderLogin(t.Context(), inbound.StartProviderLoginInput{Provider: test.provider, Source: "192.0.2.1"})
			if !errors.Is(err, test.want) || result != (inbound.StartProviderLoginResult{}) || len(repository.attempts) != 0 {
				t.Fatalf("error = %v, result = %+v", err, result)
			}
		})
	}
}
