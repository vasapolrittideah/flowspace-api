//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/github"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

// fakeGitHub accepts one authorization code whose PKCE verifier matches the
// challenge in the last authorization URL.
type fakeGitHub struct {
	challenge, userID, emails string
	emailStatus               int
}

func (g *fakeGitHub) serve(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.ParseForm() != nil || r.PostForm.Get("code") != "github-code" || domain.ProviderCodeChallenge(r.PostForm.Get("code_verifier")) != g.challenge {
			_, _ = w.Write([]byte(`{"error":"bad_verification_code"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"github-access","token_type":"bearer"}`))
	})
	authorized := func(handler func(http.ResponseWriter)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer github-access" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			handler(w)
		}
	}
	mux.Handle("GET /user", authorized(func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"id":` + g.userID + `,"email":"profile@gmail.com"}`))
	}))
	mux.Handle("GET /user/emails", authorized(func(w http.ResponseWriter) {
		w.WriteHeader(g.emailStatus)
		_, _ = w.Write([]byte(g.emails))
	}))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func testGitHubProviderLogin(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f providerOnlyFixture) {
	provider := &fakeGitHub{}
	server := provider.serve(t)
	allow := func(context.Context, string) error { return nil }
	identity := github.NewProviderIdentity("github-client", "github-secret", github.Endpoints{
		CodeExchange: server.URL + "/token", User: server.URL + "/user", Emails: server.URL + "/user/emails",
	}, server.Client())
	service := app.NewProviderLoginService(postgres.NewProviderAttemptRepository(pool), allow, allow, f.key, map[domain.Provider]app.ProviderClient{
		domain.ProviderGitHub: {ClientID: "github-client", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/github", Identity: identity},
		domain.ProviderGoogle: {ClientID: "google-client", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/google", Identity: identity},
	}).WithSessions(postgres.NewAccountRepository(pool), f.signer, f.protector, allow)

	start := func(t *testing.T) (string, string) {
		t.Helper()
		started, err := service.StartProviderLogin(ctx, inbound.StartProviderLoginInput{Provider: "github", Source: "192.0.2.95"})
		if err != nil {
			t.Fatal(err)
		}
		authorization, err := url.Parse(started.AuthorizationURL)
		if err != nil {
			t.Fatal(err)
		}
		provider.challenge = authorization.Query().Get("code_challenge")
		return started.AttemptToken, authorization.Query().Get("state")
	}
	callback := func(providerName, state string) (string, error) {
		return service.CompleteProviderCallback(ctx, inbound.CompleteProviderCallbackInput{
			Provider: providerName, State: state, Code: "github-code", Source: "192.0.2.95",
		})
	}
	login := func(t *testing.T, userID, emails string, emailStatus int) (inbound.CreateProviderSessionResult, error) {
		t.Helper()
		provider.userID, provider.emails, provider.emailStatus = userID, emails, emailStatus
		attempt, state := start(t)
		code, err := callback("github", state)
		if err != nil {
			t.Fatal(err)
		}
		return service.CreateProviderSession(ctx, inbound.CreateProviderSessionInput{AttemptToken: attempt, HandoffCode: code, Source: "192.0.2.95"})
	}
	count := func(t *testing.T, query string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("a new GitHub identity with a verified primary email gets an unverified account, link, session, and code", func(t *testing.T) {
		result, err := login(t, "4001", `[{"email":"GitHub.User@gmail.com","primary":true,"verified":true}]`, http.StatusOK)
		if err != nil || result.Subject == "" || result.EmailVerified || result.AccessToken == "" || strings.Contains(result.AccessToken, "github-access") {
			t.Fatalf("result = %q verified %t, %v", result.Subject, result.EmailVerified, err)
		}
		if n := count(t, `SELECT count(*) FROM identity_accounts WHERE subject = $1 AND email_local = 'GitHub.User' AND email_domain = 'gmail.com'
			AND password_hash IS NULL AND email_verified_at IS NULL`, result.Subject); n != 1 {
			t.Fatalf("accounts = %d", n)
		}
		if n := count(t, `SELECT count(*) FROM identity_provider_links WHERE provider = 'github' AND provider_subject = '4001' AND account_subject = $1`, result.Subject); n != 1 {
			t.Fatalf("links = %d", n)
		}
		if n := count(t, `SELECT count(*) FROM identity_sessions WHERE account_subject = $1`, result.Subject); n != 1 {
			t.Fatalf("sessions = %d", n)
		}
		if n := count(t, `SELECT count(*) FROM identity_challenges WHERE account_subject = $1 AND purpose = 'verify-email'`, result.Subject); n != 1 {
			t.Fatalf("verification challenges = %d", n)
		}

		for name, emails := range map[string]struct {
			body   string
			status int
		}{
			"changed email":         {`[{"email":"changed@example.org","primary":true,"verified":true}]`, http.StatusOK},
			"absent email":          {`[]`, http.StatusOK},
			"email endpoint failed": {`{"message":"unavailable"}`, http.StatusInternalServerError},
		} {
			again, err := login(t, "4001", emails.body, emails.status)
			if err != nil || again.Subject != result.Subject || again.EmailVerified {
				t.Fatalf("%s returning login = %q, %v", name, again.Subject, err)
			}
		}
		if n := count(t, `SELECT count(*) FROM identity_accounts WHERE subject = $1 AND email_local = 'GitHub.User' AND email_verified_at IS NULL`, result.Subject); n != 1 {
			t.Fatal("a returning GitHub login changed the stored email or its state")
		}
		if n := count(t, `SELECT count(*) FROM identity_sessions WHERE account_subject = $1`, result.Subject); n != 4 {
			t.Fatalf("sessions = %d", n)
		}
	})

	t.Run("a new GitHub identity without a verified primary email creates no records", func(t *testing.T) {
		for userID, emails := range map[string]string{
			"4002": `[{"email":"unverified@gmail.com","primary":true,"verified":false},{"email":"secondary@gmail.com","primary":false,"verified":true}]`,
			"4003": `[]`,
		} {
			result, err := login(t, userID, emails, http.StatusOK)
			if !errors.Is(err, app.ErrProviderAccountUnavailable) || result != (inbound.CreateProviderSessionResult{}) {
				t.Fatalf("%s result = %q, %v", userID, result.Subject, err)
			}
			if _, err := login(t, userID, emails, http.StatusInternalServerError); !errors.Is(err, app.ErrProviderAccountUnavailable) {
				t.Fatalf("%s with a failed email endpoint = %v", userID, err)
			}
		}
		if n := count(t, `SELECT count(*) FROM identity_provider_links WHERE provider = 'github' AND provider_subject IN ('4002', '4003')`); n != 0 {
			t.Fatalf("links = %d", n)
		}
		if n := count(t, `SELECT count(*) FROM identity_accounts WHERE email_domain = 'gmail.com'
			AND email_local IN ('unverified', 'secondary', 'profile')`); n != 0 {
			t.Fatalf("accounts = %d", n)
		}
	})

	t.Run("a wrong provider or PKCE verifier yields no handoff", func(t *testing.T) {
		provider.userID, provider.emails, provider.emailStatus = "4004", `[]`, http.StatusOK
		_, state := start(t)
		if _, err := callback("google", state); !errors.Is(err, app.ErrInvalidProviderCallback) {
			t.Fatalf("wrong provider err = %v", err)
		}
		provider.challenge = "altered"
		if _, err := callback("github", state); !errors.Is(err, app.ErrInvalidProviderCallback) {
			t.Fatalf("wrong PKCE err = %v", err)
		}
		if _, err := callback("github", state); !errors.Is(err, app.ErrInvalidProviderCallback) {
			t.Fatalf("replayed state err = %v", err)
		}
		if n := count(t, `SELECT count(*) FROM identity_provider_login_attempts WHERE provider = 'github' AND provider_subject = '4004'`); n != 0 {
			t.Fatalf("stored results = %d", n)
		}
	})
}
