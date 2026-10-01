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

	consumed      []string
	proof         outbound.ProviderAttemptProof
	found         bool
	consumeErr    error
	recorded      []outbound.ProviderIdentity
	handoff       [32]byte
	recordAllowed bool
	recordErr     error
	failed        []string
}

func (r *providerAttemptRepository) ConsumeProviderState(_ context.Context, provider string, state [32]byte) (outbound.ProviderAttemptProof, bool, error) {
	r.consumed = append(r.consumed, provider+":"+string(state[:4]))
	return r.proof, r.found, r.consumeErr
}

func (r *providerAttemptRepository) RecordProviderResult(_ context.Context, id string, identity outbound.ProviderIdentity, handoff [32]byte) (bool, error) {
	if id != r.proof.ID {
		return false, errors.New("wrong attempt")
	}
	r.recorded, r.handoff = append(r.recorded, identity), handoff
	return r.recordAllowed, r.recordErr
}

func (r *providerAttemptRepository) FailProviderAttempt(_ context.Context, id string) error {
	r.failed = append(r.failed, id)
	return nil
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
			}, allowLimit, key, providerClients)

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
	service := app.NewProviderLoginService(repository, allowLimit, allowLimit, make([]byte, 32), providerClients)
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
	allow := allowLimit
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
			service := app.NewProviderLoginService(repository, test.limit, allowLimit, make([]byte, 32), test.clients)
			result, err := service.StartProviderLogin(t.Context(), inbound.StartProviderLoginInput{Provider: test.provider, Source: "192.0.2.1"})
			if !errors.Is(err, test.want) || result != (inbound.StartProviderLoginResult{}) || len(repository.attempts) != 0 {
				t.Fatalf("error = %v, result = %+v", err, result)
			}
		})
	}
}

func allowLimit(context.Context, string) error { return nil }

type providerIdentityStub struct {
	exchanges []outbound.ProviderCodeExchange
	identity  outbound.ProviderIdentity
	err       error
}

func (s *providerIdentityStub) VerifyProviderIdentity(_ context.Context, exchange outbound.ProviderCodeExchange) (outbound.ProviderIdentity, error) {
	s.exchanges = append(s.exchanges, exchange)
	return s.identity, s.err
}

type providerCallbackFixture struct {
	repository *providerAttemptRepository
	identity   *providerIdentityStub
	limited    []string
	service    *app.ProviderLoginService
	key        []byte
}

func newProviderCallbackFixture(limit error) *providerCallbackFixture {
	f := &providerCallbackFixture{
		repository: &providerAttemptRepository{
			found: true, recordAllowed: true,
			proof: outbound.ProviderAttemptProof{ID: "attempt", CodeVerifier: "verifier", Nonce: "nonce", CallbackURL: providerClients[domain.ProviderGoogle].CallbackURL},
		},
		identity: &providerIdentityStub{identity: outbound.ProviderIdentity{Subject: "google-subject", Email: "user@gmail.com", EmailVerified: true}},
		key:      make([]byte, 32),
	}
	clients := map[domain.Provider]app.ProviderClient{domain.ProviderGoogle: providerClients[domain.ProviderGoogle], domain.ProviderGitHub: providerClients[domain.ProviderGitHub]}
	google := clients[domain.ProviderGoogle]
	google.Identity = f.identity
	clients[domain.ProviderGoogle] = google
	f.service = app.NewProviderLoginService(f.repository, allowLimit, func(_ context.Context, source string) error {
		f.limited = append(f.limited, source)
		return limit
	}, f.key, clients)
	return f
}

func (f *providerCallbackFixture) complete(t *testing.T, input inbound.CompleteProviderCallbackInput) (string, error) {
	t.Helper()
	if input.Source == "" {
		input.Source = "192.0.2.1"
	}
	return f.service.CompleteProviderCallback(t.Context(), input)
}

func TestCompleteProviderCallbackStoresResultAndReturnsHandoffCode(t *testing.T) {
	f := newProviderCallbackFixture(nil)
	code, err := f.complete(t, inbound.CompleteProviderCallbackInput{Provider: "google", State: "state", Code: "provider-code"})
	if err != nil {
		t.Fatal(err)
	}
	state := domain.ProviderSecretVerifier(f.key, domain.ProviderSecretState, "state")
	if len(f.limited) != 1 || len(f.repository.consumed) != 1 || f.repository.consumed[0] != "google:"+string(state[:4]) {
		t.Fatalf("limits %v, consumed %v", f.limited, f.repository.consumed)
	}
	want := outbound.ProviderCodeExchange{Code: "provider-code", CodeVerifier: "verifier", Nonce: "nonce", CallbackURL: providerClients[domain.ProviderGoogle].CallbackURL}
	if len(f.identity.exchanges) != 1 || f.identity.exchanges[0] != want {
		t.Fatalf("exchange = %+v", f.identity.exchanges)
	}
	if len(f.repository.recorded) != 1 || f.repository.recorded[0] != f.identity.identity ||
		f.repository.handoff != domain.ProviderSecretVerifier(f.key, domain.ProviderSecretHandoffCode, code) || len(f.repository.failed) != 0 {
		t.Fatal("result or handoff verifier was not stored")
	}
	if code == "" || code == "state" || code == "provider-code" {
		t.Fatalf("handoff code = %q", code)
	}
}

func TestCompleteProviderCallbackRejectsInvalidCallbacks(t *testing.T) {
	for _, test := range []struct {
		name            string
		input           inbound.CompleteProviderCallbackInput
		prepare         func(*providerCallbackFixture)
		want            error
		consumed, fails int
	}{
		{"unknown provider", inbound.CompleteProviderCallbackInput{Provider: "okta", State: "state", Code: "code"}, nil, app.ErrInvalidProviderCallback, 0, 0},
		{"missing state", inbound.CompleteProviderCallbackInput{Provider: "google", Code: "code"}, nil, app.ErrInvalidProviderCallback, 0, 0},
		{
			"unknown or used state",
			inbound.CompleteProviderCallbackInput{Provider: "google", State: "state", Code: "code"},
			func(f *providerCallbackFixture) { f.repository.found = false }, app.ErrInvalidProviderCallback, 1, 0,
		},
		{"provider denial", inbound.CompleteProviderCallbackInput{Provider: "google", State: "state", Denied: true}, nil, app.ErrInvalidProviderCallback, 1, 1},
		{"missing code", inbound.CompleteProviderCallbackInput{Provider: "google", State: "state"}, nil, app.ErrInvalidProviderCallback, 1, 1},
		{
			"invalid proof",
			inbound.CompleteProviderCallbackInput{Provider: "google", State: "state", Code: "code"},
			func(f *providerCallbackFixture) { f.identity.err = domain.ErrInvalidProviderProof }, app.ErrInvalidProviderCallback, 1, 1,
		},
		{
			"empty subject",
			inbound.CompleteProviderCallbackInput{Provider: "google", State: "state", Code: "code"},
			func(f *providerCallbackFixture) { f.identity.identity.Subject = "" }, app.ErrInvalidProviderCallback, 1, 1,
		},
		{
			"provider outage",
			inbound.CompleteProviderCallbackInput{Provider: "google", State: "state", Code: "code"},
			func(f *providerCallbackFixture) { f.identity.err = domain.ErrProviderUnavailable }, app.ErrProviderLoginUnavailable, 1, 1,
		},
		{
			"attempt expired before storing",
			inbound.CompleteProviderCallbackInput{Provider: "google", State: "state", Code: "code"},
			func(f *providerCallbackFixture) { f.repository.recordAllowed = false }, app.ErrInvalidProviderCallback, 1, 0,
		},
		{
			"store unavailable",
			inbound.CompleteProviderCallbackInput{Provider: "google", State: "state", Code: "code"},
			func(f *providerCallbackFixture) { f.repository.consumeErr = errors.New("database down") }, app.ErrProviderLoginUnavailable, 1, 0,
		},
		{"provider not configured", inbound.CompleteProviderCallbackInput{Provider: "github", State: "state", Code: "code"}, nil, app.ErrProviderLoginUnavailable, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newProviderCallbackFixture(nil)
			if test.prepare != nil {
				test.prepare(f)
			}
			code, err := f.complete(t, test.input)
			if !errors.Is(err, test.want) || code != "" {
				t.Fatalf("code %q, error %v; want %v", code, err, test.want)
			}
			if len(f.limited) != 1 || len(f.repository.consumed) != test.consumed || len(f.repository.failed) != test.fails {
				t.Fatalf("limits %d, consumed %d, failed %d", len(f.limited), len(f.repository.consumed), len(f.repository.failed))
			}
		})
	}
}

func TestCompleteProviderCallbackAppliesSourceLimitFirst(t *testing.T) {
	for _, limit := range []error{app.ErrRateLimited, app.ErrLimitUnavailable} {
		f := newProviderCallbackFixture(limit)
		code, err := f.complete(t, inbound.CompleteProviderCallbackInput{Provider: "google", State: "state", Code: "code"})
		if !errors.Is(err, limit) || code != "" || len(f.repository.consumed) != 0 || len(f.identity.exchanges) != 0 {
			t.Fatalf("limit %v: code %q, error %v", limit, code, err)
		}
	}
}
