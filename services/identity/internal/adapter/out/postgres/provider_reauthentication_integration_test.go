//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type fixedProviderIdentity struct{ identity *outbound.ProviderIdentity }

func (f fixedProviderIdentity) VerifyProviderIdentity(context.Context, outbound.ProviderCodeExchange) (outbound.ProviderIdentity, error) {
	return *f.identity, nil
}

type providerReauthenticationServices struct {
	login          *app.ProviderLoginService
	reauthenticate *app.ProviderReauthenticationService
}

func testProviderReauthentication(t *testing.T, pool *pgxpool.Pool, db *sql.DB) {
	ctx := t.Context()
	key := bytes.Repeat([]byte{9}, 32)
	identity := &outbound.ProviderIdentity{}
	clients := map[domain.Provider]app.ProviderClient{
		domain.ProviderGoogle: {ClientID: "google-client", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/google", Identity: fixedProviderIdentity{identity}},
		domain.ProviderGitHub: {ClientID: "github-client", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/github", Identity: fixedProviderIdentity{identity}},
	}
	newServices := func(p *pgxpool.Pool) providerReauthenticationServices {
		limits := app.NewLimitService(identitypostgres.NewLimitRepository(p))
		accounts := identitypostgres.NewAccountRepository(p)
		login := app.NewProviderLoginService(identitypostgres.NewProviderAttemptRepository(p), limits.ProviderLoginStart, limits.ProviderCallback, key, clients).
			WithSessions(accounts, loginLimitSigner{}, failingDeliveryProtector{}, limits.ProviderSessionFailure)
		return providerReauthenticationServices{login, app.NewProviderReauthenticationService(login, accounts, limits.ProviderSessionFailure)}
	}
	services := newServices(pool)
	queries := identitysqlc.New(pool)
	session := func(t *testing.T, subject string) string {
		t.Helper()
		hash := sha256.Sum256([]byte(uuid.NewString()))
		row, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: hash[:]})
		if err != nil {
			t.Fatal(err)
		}
		id := uuid.UUID(row.ID.Bytes).String()
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=NULL WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	// seed creates a provider-only account with 2 sessions whose windows are closed.
	seed := func(t *testing.T, provider string) (string, string, string, string) {
		t.Helper()
		subject, providerSubject := uuid.NewString(), uuid.NewString()
		if _, err := queries.CreateProviderAccount(ctx, identitysqlc.CreateProviderAccountParams{Subject: subject, EmailLocal: subject, EmailDomain: "example.com"}); err != nil {
			t.Fatal(err)
		}
		if err := queries.CreateProviderLink(ctx, identitysqlc.CreateProviderLinkParams{Provider: provider, ProviderSubject: providerSubject, AccountSubject: subject}); err != nil {
			t.Fatal(err)
		}
		return subject, providerSubject, session(t, subject), session(t, subject)
	}
	// callback completes the provider callback of a started attempt for the provider identity.
	callback := func(t *testing.T, s providerReauthenticationServices, provider, providerSubject string, result inbound.StartProviderLoginResult, reauthentication bool) string {
		t.Helper()
		authorization, err := url.Parse(result.AuthorizationURL)
		if err != nil {
			t.Fatal(err)
		}
		*identity = outbound.ProviderIdentity{Subject: providerSubject, Email: "other@example.com", EmailVerified: true}
		callback, err := s.login.CompleteProviderCallback(ctx, inbound.CompleteProviderCallbackInput{
			Provider: provider, State: authorization.Query().Get("state"), Code: "provider-code", Source: "192.0.2.240",
		})
		if err != nil || callback.Reauthentication != reauthentication {
			t.Fatalf("callback re-authentication = %t, want %t, error = %v", callback.Reauthentication, reauthentication, err)
		}
		return callback.HandoffCode
	}
	start := func(t *testing.T, s providerReauthenticationServices, subject, sessionID, provider, providerSubject string) (string, string) {
		t.Helper()
		result, err := s.reauthenticate.StartProviderReauthentication(ctx, inbound.StartProviderReauthenticationInput{
			Subject: subject, SessionID: sessionID, Provider: provider, Source: "192.0.2.241",
		})
		if err != nil {
			t.Fatal(err)
		}
		return result.AttemptToken, callback(t, s, provider, providerSubject, result, true)
	}
	complete := func(s providerReauthenticationServices, subject, sessionID, token, code string) (inbound.CompleteProviderReauthenticationResult, error) {
		return s.reauthenticate.CompleteProviderReauthentication(ctx, inbound.CompleteProviderReauthenticationInput{
			Subject: subject, SessionID: sessionID, AttemptToken: token, HandoffCode: code, Source: "192.0.2.242",
		})
	}
	count := func(t *testing.T, query string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("a linked provider opens the window of the starting session only", func(t *testing.T) {
		for _, provider := range []string{"google", "github"} {
			subject, providerSubject, id, other := seed(t, provider)
			token, code := start(t, services, subject, id, provider, providerSubject)
			if count(t, `SELECT count(*) FROM identity_provider_login_attempts WHERE purpose='reauthenticate' AND account_subject=$1 AND session_id=$2`, subject, id) != 1 {
				t.Fatalf("%s attempt is not bound to the session", provider)
			}
			if _, err := complete(services, subject, other, token, code); !errors.Is(err, app.ErrProviderHandoffRejected) {
				t.Fatalf("another session completed the attempt: %v", err)
			}
			if _, err := services.login.CreateProviderSession(ctx, inbound.CreateProviderSessionInput{AttemptToken: token, HandoffCode: code, Source: "192.0.2.243"}); !errors.Is(err, app.ErrProviderHandoffRejected) {
				t.Fatalf("provider login completed the attempt: %v", err)
			}
			result, err := complete(services, subject, id, token, code)
			if err != nil {
				t.Fatal(err)
			}
			var proof time.Time
			if err := pool.QueryRow(ctx, `SELECT reauthenticated_at FROM identity_sessions WHERE id=$1`, id).Scan(&proof); err != nil || !result.ExpiresAt.Equal(proof.Add(10*time.Minute)) {
				t.Fatalf("%s window end = %v, proof = %v, %v", provider, result.ExpiresAt, proof, err)
			}
			if err := sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil); err != nil {
				t.Fatal(err)
			}
			requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, other, uuid.NewString(), nil))
			if count(t, `SELECT count(*) FROM identity_sessions WHERE account_subject=$1`, subject) != 2 ||
				count(t, `SELECT count(*) FROM identity_provider_links WHERE account_subject=$1`, subject) != 1 ||
				count(t, `SELECT count(*) FROM identity_accounts WHERE subject=$1 AND email_local=$1 AND email_verified_at IS NULL`, subject) != 1 {
				t.Fatalf("%s re-authentication changed a session, link, or account", provider)
			}
			if _, err := complete(services, subject, id, token, code); !errors.Is(err, app.ErrProviderHandoffRejected) {
				t.Fatalf("a used handoff completed again: %v", err)
			}
		}
	})

	t.Run("an unlinked or foreign provider account keeps the window closed", func(t *testing.T) {
		subject, _, id, _ := seed(t, "github")
		foreign, foreignProviderSubject, _, _ := seed(t, "github")
		for _, providerSubject := range []string{uuid.NewString(), foreignProviderSubject} {
			token, code := start(t, services, subject, id, "github", providerSubject)
			if _, err := complete(services, subject, id, token, code); !errors.Is(err, app.ErrProviderAccountUnavailable) {
				t.Fatalf("unlinked provider error = %v", err)
			}
			requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil))
			if _, err := complete(services, subject, id, token, code); !errors.Is(err, app.ErrProviderHandoffRejected) {
				t.Fatalf("an unlinked result was claimed twice: %v", err)
			}
		}
		if count(t, `SELECT count(*) FROM identity_provider_links WHERE provider_subject=$1 AND account_subject=$2`, foreignProviderSubject, foreign) != 1 ||
			count(t, `SELECT count(*) FROM identity_provider_links WHERE account_subject=$1`, subject) != 1 {
			t.Fatal("re-authentication changed a provider link")
		}
	})

	t.Run("a login attempt cannot complete re-authentication", func(t *testing.T) {
		subject, providerSubject, id, _ := seed(t, "google")
		result, err := services.login.StartProviderLogin(ctx, inbound.StartProviderLoginInput{Provider: "google", Source: "192.0.2.244"})
		if err != nil {
			t.Fatal(err)
		}
		code := callback(t, services, "google", providerSubject, result, false)
		if _, err := complete(services, subject, id, result.AttemptToken, code); !errors.Is(err, app.ErrProviderHandoffRejected) {
			t.Fatalf("a login attempt re-authenticated: %v", err)
		}
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil))
		if _, err := services.login.CreateProviderSession(ctx, inbound.CreateProviderSessionInput{AttemptToken: result.AttemptToken, HandoffCode: code, Source: "192.0.2.244"}); err != nil {
			t.Fatalf("the login attempt no longer completes login: %v", err)
		}
	})

	t.Run("an older replica cannot claim a re-authentication attempt as login", func(t *testing.T) {
		subject, providerSubject, id, _ := seed(t, "github")
		token, code := start(t, services, subject, id, "github", providerSubject)
		attempt := domain.ProviderSecretVerifier(key, domain.ProviderSecretAttemptToken, token)
		handoff := domain.ProviderSecretVerifier(key, domain.ProviderSecretHandoffCode, code)
		// This is the claim query of provider login before migration 00013.
		_, err := pool.Exec(ctx, `UPDATE identity_provider_login_attempts SET session_claimed_at = statement_timestamp()
			WHERE attempt_token_verifier = $1 AND handoff_code_verifier = $2 AND session_claimed_at IS NULL AND handoff_failures < 5
			AND expires_at > statement_timestamp()`, attempt[:], handoff[:])
		if err == nil || !strings.Contains(err.Error(), "identity_provider_login_attempts_purpose_claim_check") {
			t.Fatalf("an older login claim of a re-authentication attempt = %v", err)
		}
		if _, err := complete(services, subject, id, token, code); err != nil {
			t.Fatalf("the rejected login claim consumed the attempt: %v", err)
		}
	})

	t.Run("an expired or exhausted attempt keeps the window closed", func(t *testing.T) {
		subject, providerSubject, id, _ := seed(t, "github")
		expired, expiredCode := start(t, services, subject, id, "github", providerSubject)
		verifier := domain.ProviderSecretVerifier(key, domain.ProviderSecretAttemptToken, expired)
		if _, err := pool.Exec(ctx, `UPDATE identity_provider_login_attempts SET created_at=created_at-INTERVAL '11 minutes', expires_at=expires_at-INTERVAL '11 minutes'
			WHERE attempt_token_verifier=$1`, verifier[:]); err != nil {
			t.Fatal(err)
		}
		if _, err := complete(services, subject, id, expired, expiredCode); !errors.Is(err, app.ErrProviderHandoffRejected) {
			t.Fatalf("an expired attempt completed: %v", err)
		}
		for _, failures := range []int{4, 5} {
			token, code := start(t, services, subject, id, "github", providerSubject)
			verifier := domain.ProviderSecretVerifier(key, domain.ProviderSecretAttemptToken, token)
			if _, err := pool.Exec(ctx, `UPDATE identity_provider_login_attempts SET handoff_failures=$2 WHERE attempt_token_verifier=$1`, verifier[:], failures); err != nil {
				t.Fatal(err)
			}
			_, err := complete(services, subject, id, token, code)
			if failures == 5 && !errors.Is(err, app.ErrProviderHandoffRejected) || failures == 4 && err != nil {
				t.Fatalf("completion after %d failures = %v", failures, err)
			}
			if failures == 5 {
				continue
			}
			if err := sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=NULL WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
		}
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil))
	})

	t.Run("a link of another provider with the same provider subject does not match", func(t *testing.T) {
		subject, providerSubject, id, _ := seed(t, "github")
		token, code := start(t, services, subject, id, "google", providerSubject)
		if _, err := complete(services, subject, id, token, code); !errors.Is(err, app.ErrProviderAccountUnavailable) {
			t.Fatalf("a GitHub link matched a Google identity: %v", err)
		}
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil))
		if count(t, `SELECT count(*) FROM identity_provider_links WHERE account_subject=$1 AND provider='github' AND provider_subject=$2`, subject, providerSubject) != 1 ||
			count(t, `SELECT count(*) FROM identity_provider_links WHERE account_subject=$1`, subject) != 1 {
			t.Fatal("re-authentication changed a provider link")
		}
	})

	t.Run("concurrent completions from 2 replicas open 1 window", func(t *testing.T) {
		subject, providerSubject, id, _ := seed(t, "github")
		token, code := start(t, services, subject, id, "github", providerSubject)
		replica, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
		if err != nil {
			t.Fatal(err)
		}
		defer replica.Close()
		var group sync.WaitGroup
		failures := make(chan error, 2)
		for _, s := range []providerReauthenticationServices{services, newServices(replica)} {
			group.Go(func() {
				_, err := complete(s, subject, id, token, code)
				failures <- err
			})
		}
		group.Wait()
		close(failures)
		succeeded := 0
		for err := range failures {
			switch {
			case err == nil:
				succeeded++
			case !errors.Is(err, app.ErrProviderHandoffRejected):
				t.Fatal(err)
			}
		}
		if succeeded != 1 {
			t.Fatalf("%d concurrent completions succeeded", succeeded)
		}
	})

	t.Run("start and rejected completion share the provider-login limits across replicas", func(t *testing.T) {
		subject, providerSubject, id, _ := seed(t, "github")
		replica, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
		if err != nil {
			t.Fatal(err)
		}
		defer replica.Close()
		limited := newServices(replica)
		token, code := start(t, services, subject, id, "github", providerSubject)
		if count(t, `SELECT COALESCE(SUM(count),0) FROM identity_limit_counters WHERE scope='source' AND action='provider-login-start' AND counter_key='192.0.2.241'`) == 0 {
			t.Fatal("start did not count against the provider-login start limit")
		}
		if _, err := pool.Exec(ctx, `INSERT INTO identity_limit_counters(scope,counter_key,action,window_start,count) VALUES
			('source','192.0.2.245','provider-login-start',statement_timestamp(),60), ('source','192.0.2.246','provider-session-failure',statement_timestamp(),100)`); err != nil {
			t.Fatal(err)
		}
		attempts := count(t, `SELECT count(*) FROM identity_provider_login_attempts WHERE account_subject=$1`, subject)
		if _, err := limited.reauthenticate.StartProviderReauthentication(ctx, inbound.StartProviderReauthenticationInput{
			Subject: subject, SessionID: id, Provider: "github", Source: "192.0.2.245",
		}); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("full start limit = %v", err)
		}
		if count(t, `SELECT count(*) FROM identity_provider_login_attempts WHERE account_subject=$1`, subject) != attempts {
			t.Fatal("a limited start created an attempt")
		}
		wrong, err := domain.NewProviderHandoffCode()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := limited.reauthenticate.CompleteProviderReauthentication(ctx, inbound.CompleteProviderReauthenticationInput{
			Subject: subject, SessionID: id, AttemptToken: token, HandoffCode: wrong, Source: "192.0.2.246",
		}); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("full failure limit = %v", err)
		}
		if count(t, `SELECT handoff_failures FROM identity_provider_login_attempts WHERE account_subject=$1 AND session_id=$2 AND handoff_code_verifier IS NOT NULL`, subject, id) != 1 {
			t.Fatal("the rejected handoff was not counted on the attempt")
		}
		if _, err := complete(services, subject, id, token, code); err != nil {
			t.Fatalf("a failed completion revoked the attempt or the session: %v", err)
		}
	})

	t.Run("an unavailable limit store or an inactive session opens no window", func(t *testing.T) {
		subject, providerSubject, id, _ := seed(t, "github")
		closed, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
		if err != nil {
			t.Fatal(err)
		}
		closed.Close()
		limits := app.NewLimitService(identitypostgres.NewLimitRepository(closed))
		login := app.NewProviderLoginService(identitypostgres.NewProviderAttemptRepository(pool), limits.ProviderLoginStart, limits.ProviderCallback, key, clients)
		failing := app.NewProviderReauthenticationService(login, identitypostgres.NewAccountRepository(pool), limits.ProviderSessionFailure)
		if _, err := failing.StartProviderReauthentication(ctx, inbound.StartProviderReauthenticationInput{Subject: subject, SessionID: id, Provider: "github", Source: "192.0.2.247"}); !errors.Is(err, app.ErrLimitUnavailable) {
			t.Fatalf("start with an unavailable limit store = %v", err)
		}
		token, code := start(t, services, subject, id, "github", providerSubject)
		wrong, err := domain.NewProviderHandoffCode()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := failing.CompleteProviderReauthentication(ctx, inbound.CompleteProviderReauthenticationInput{Subject: subject, SessionID: id, AttemptToken: token, HandoffCode: wrong, Source: "192.0.2.247"}); !errors.Is(err, app.ErrLimitUnavailable) {
			t.Fatalf("rejected completion with an unavailable limit store = %v", err)
		}
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET revoked_at=clock_timestamp() WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := complete(services, subject, id, token, code); !errors.Is(err, outbound.ErrUnauthenticated) {
			t.Fatalf("a revoked session completed re-authentication: %v", err)
		}
		if count(t, `SELECT count(*) FROM identity_sessions WHERE id=$1 AND reauthenticated_at IS NULL`, id) != 1 {
			t.Fatal("a failed completion opened the window")
		}
	})

	t.Run("the migration gives old attempts the login purpose and its rollback deletes re-authentication attempts", func(t *testing.T) {
		subject, providerSubject, id, _ := seed(t, "github")
		token, _ := start(t, services, subject, id, "github", providerSubject)
		reauthenticationVerifier := domain.ProviderSecretVerifier(key, domain.ProviderSecretAttemptToken, token)
		login, err := services.login.StartProviderLogin(ctx, inbound.StartProviderLoginInput{Provider: "github", Source: "192.0.2.248"})
		if err != nil {
			t.Fatal(err)
		}
		loginVerifier := domain.ProviderSecretVerifier(key, domain.ProviderSecretAttemptToken, login.AttemptToken)
		if err := goose.DownToContext(ctx, db, ".", 12); err != nil {
			t.Fatal(err)
		}
		if count(t, `SELECT count(*) FROM identity_provider_login_attempts WHERE attempt_token_verifier=$1`, loginVerifier[:]) != 1 {
			t.Fatal("rollback deleted a login attempt")
		}
		if count(t, `SELECT count(*) FROM identity_provider_login_attempts WHERE attempt_token_verifier=$1`, reauthenticationVerifier[:]) != 0 {
			t.Fatal("rollback kept a re-authentication attempt")
		}
		var legacy string
		if err := pool.QueryRow(ctx, `INSERT INTO identity_provider_login_attempts(provider,attempt_token_verifier,state_verifier,code_verifier,callback_url,expires_at)
			VALUES ('github',$1,$2,repeat('v',43),'https://api.example.com/v1/provider-login-callbacks/github',statement_timestamp()+INTERVAL '10 minutes') RETURNING id::text`,
			bytes.Repeat([]byte{41}, 32), bytes.Repeat([]byte{42}, 32)).Scan(&legacy); err != nil {
			t.Fatal(err)
		}
		if err := goose.UpContext(ctx, db, "."); err != nil {
			t.Fatal(err)
		}
		if count(t, `SELECT count(*) FROM identity_provider_login_attempts WHERE id=$1 AND purpose='login' AND account_subject IS NULL AND session_id IS NULL`, legacy) != 1 {
			t.Fatal("an existing attempt did not get the login purpose")
		}
	})
}
