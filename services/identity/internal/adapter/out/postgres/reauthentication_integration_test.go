//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	identityhttp "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type commitDelayKey struct{}

type commitDelayTracer struct {
	ready   chan struct{}
	release chan struct{}
}

func (*commitDelayTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "GetTransactionCheckTime") || strings.Contains(data.SQL, "QueueSessionReauthenticationCheck") {
		return context.WithValue(ctx, commitDelayKey{}, true)
	}
	return ctx
}

func (d *commitDelayTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if active, _ := ctx.Value(commitDelayKey{}).(bool); active {
		select {
		case d.ready <- struct{}{}:
		case <-ctx.Done():
			return
		}
		select {
		case <-d.release:
		case <-ctx.Done():
		}
	}
}

// sensitiveTestChange creates an account only through the guarded transaction.
func sensitiveTestChange(ctx context.Context, pool *pgxpool.Pool, subject, sessionID, marker string, work func() error) error {
	err := identitypostgres.NewAccountRepository(pool).WithinTransaction(ctx, func(tx outbound.SignupTransaction) error {
		guarded, ok := tx.(outbound.ReauthenticationTransaction)
		if !ok {
			return errors.New("transaction has no re-authentication guard")
		}
		if err := app.RequireReauthentication(ctx, guarded, subject, sessionID); err != nil {
			return err
		}
		if err := tx.CreateAccount(ctx, marker, marker+"@example.com", "$argon2id$test"); err != nil {
			return err
		}
		if work != nil {
			return work()
		}
		return nil
	})
	return identityhttp.ReauthenticationRPCError(err)
}

func requireReauthenticationError(t *testing.T, err error) {
	t.Helper()
	result := status.Convert(err)
	if result.Code() != codes.PermissionDenied || len(result.Details()) != 1 {
		t.Fatalf("guard error = %v", err)
	}
	detail, ok := result.Details()[0].(*errdetails.ErrorInfo)
	if !ok || detail.GetReason() != "REAUTHENTICATION_REQUIRED" || detail.GetDomain() != "identity.flowspace" {
		t.Fatalf("guard detail = %v", detail)
	}
}

func assertInitialReauthenticationWindow(t *testing.T, pool *pgxpool.Pool, subject string) {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT id::text, created_at, reauthenticated_at FROM identity_sessions WHERE account_subject=$1 AND revoked_at IS NULL`, subject)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		var created, authenticated time.Time
		if err := rows.Scan(&id, &created, &authenticated); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if !created.Equal(authenticated) {
			rows.Close()
			t.Fatal("initial proof time differs from session creation")
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("login created no session")
	}
	for _, id := range ids {
		if err := sensitiveTestChange(t.Context(), pool, subject, id, uuid.NewString(), nil); err != nil {
			t.Fatalf("fresh login guard = %v", err)
		}
	}
}

func assertRevokedReauthenticationWindows(t *testing.T, pool *pgxpool.Pool, subject string) {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT id::text FROM identity_sessions WHERE account_subject=$1`, subject)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("no sessions to check")
	}
	for _, id := range ids {
		assertRevokedReauthenticationWindow(t, pool, subject, id)
	}
}

func assertRevokedReauthenticationWindow(t *testing.T, pool *pgxpool.Pool, subject, id string) {
	t.Helper()
	if err := sensitiveTestChange(t.Context(), pool, subject, id, uuid.NewString(), nil); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("revoked window = %v", err)
	}
}

func testReauthenticationMigration(t *testing.T, db *sql.DB, pool *pgxpool.Pool) {
	ctx := t.Context()
	if err := goose.DownToContext(ctx, db, ".", 9); err != nil {
		t.Fatal(err)
	}
	const subject = "legacy-reauthentication"
	if _, err := identitysqlc.New(pool).CreateAccount(ctx, identitysqlc.CreateAccountParams{Subject: subject, EmailLocal: subject, EmailDomain: "example.com", PasswordHash: "$argon2id$test"}); err != nil {
		t.Fatal(err)
	}
	var id string
	hash := sha256.Sum256([]byte(subject))
	if err := pool.QueryRow(ctx, `INSERT INTO identity_sessions(account_subject,refresh_token_hash,idle_expires_at,absolute_expires_at) VALUES ($1,$2,statement_timestamp()+INTERVAL '30 days',statement_timestamp()+INTERVAL '90 days') RETURNING id::text`, subject, hash[:]).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	var missing bool
	if err := pool.QueryRow(ctx, `SELECT reauthenticated_at IS NULL FROM identity_sessions WHERE id=$1`, id).Scan(&missing); err != nil || !missing {
		t.Fatalf("legacy proof is not NULL: %v", err)
	}
	requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil))
}

func testReauthenticationGuard(t *testing.T, pool *pgxpool.Pool) {
	ctx := t.Context()
	queries := identitysqlc.New(pool)
	seed := func(t *testing.T) (string, string, []byte) {
		t.Helper()
		subject := uuid.NewString()
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{Subject: subject, EmailLocal: subject, EmailDomain: "example.com", PasswordHash: "$argon2id$test"}); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256([]byte(subject))
		session, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: hash[:]})
		if err != nil {
			t.Fatal(err)
		}
		return subject, uuid.UUID(session.ID.Bytes).String(), hash[:]
	}
	t.Run("open window commits the sensitive change", func(t *testing.T) {
		subject, id, _ := seed(t)
		assertInitialReauthenticationWindow(t, pool, subject)
		marker := uuid.NewString()
		if err := sensitiveTestChange(ctx, pool, subject, id, marker, nil); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_accounts WHERE subject=$1`, marker).Scan(&count); err != nil || count != 1 {
			t.Fatalf("successful change did not persist: %d, %v", count, err)
		}
	})
	t.Run("closed and absent windows change nothing", func(t *testing.T) {
		for _, proof := range []string{"NULL", "clock_timestamp()-INTERVAL '10 minutes'"} {
			subject, id, _ := seed(t)
			if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=`+proof+` WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
			marker := uuid.NewString()
			requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, marker, nil))
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_accounts WHERE subject=$1`, marker).Scan(&count); err != nil || count != 0 {
				t.Fatalf("closed window changed state: %d, %v", count, err)
			}
		}
	})
	t.Run("refresh preserves the closed window", func(t *testing.T) {
		subject, id, hash := seed(t)
		var before, after time.Time
		if err := pool.QueryRow(ctx, `UPDATE identity_sessions SET reauthenticated_at=clock_timestamp()-INTERVAL '11 minutes' WHERE id=$1 RETURNING reauthenticated_at`, id).Scan(&before); err != nil {
			t.Fatal(err)
		}
		next := sha256.Sum256(hash)
		if _, err := identitypostgres.NewSessionRefreshRepository(pool).Rotate(ctx, hash, next[:], func(outbound.RefreshSessionRecord) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT reauthenticated_at FROM identity_sessions WHERE id=$1`, id).Scan(&after); err != nil || !after.Equal(before) {
			t.Fatalf("refresh moved proof time: %v", err)
		}
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil))
	})
	t.Run("commit after the final check crosses expiry and rolls back", func(t *testing.T) {
		for attempt := range 3 {
			subject, id, _ := seed(t)
			if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=clock_timestamp()-INTERVAL '10 minutes'+INTERVAL '2 seconds' WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
			request, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			delay := &commitDelayTracer{ready: make(chan struct{}), release: make(chan struct{})}
			configuration := pool.Config().Copy()
			configuration.ConnConfig.Tracer = delay
			delayedPool, err := pgxpool.NewWithConfig(request, configuration)
			if err != nil {
				t.Fatal(err)
			}
			defer delayedPool.Close()
			marker := uuid.NewString()
			result := make(chan error, 1)
			go func() { result <- sensitiveTestChange(request, delayedPool, subject, id, marker, nil) }()
			select {
			case err := <-result:
				cancel()
				if status.Code(err) == codes.PermissionDenied && attempt < 2 {
					continue
				}
				t.Fatalf("change never reached the final check: %v", err)
			case <-delay.ready:
			case <-request.Done():
				t.Fatal(request.Err())
			}
			if _, err := pool.Exec(request, `SELECT pg_sleep(GREATEST(0,EXTRACT(EPOCH FROM (reauthenticated_at+INTERVAL '10 minutes'-clock_timestamp())))+0.02) FROM identity_sessions WHERE id=$1`, id); err != nil {
				cancel()
				t.Fatal(err)
			}
			close(delay.release)
			requireReauthenticationError(t, <-result)
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_accounts WHERE subject=$1`, marker).Scan(&count); err != nil || count != 0 {
				t.Fatalf("late commit changed state: %d, %v", count, err)
			}
			return
		}
	})
	t.Run("two sessions of one subject keep separate windows", func(t *testing.T) {
		subject, openID, _ := seed(t)
		hash := sha256.Sum256([]byte(openID))
		closed, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: hash[:]})
		if err != nil {
			t.Fatal(err)
		}
		closedID := uuid.UUID(closed.ID.Bytes).String()
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=NULL WHERE id=$1`, closedID); err != nil {
			t.Fatal(err)
		}
		if err := sensitiveTestChange(ctx, pool, subject, openID, uuid.NewString(), nil); err != nil {
			t.Fatal(err)
		}
		marker := uuid.NewString()
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, closedID, marker, nil))
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_accounts WHERE subject=$1`, marker).Scan(&count); err != nil || count != 0 {
			t.Fatalf("closed session changed state: %d, %v", count, err)
		}
	})
	t.Run("inactive and mismatched sessions fail closed", func(t *testing.T) {
		for _, update := range []string{
			`UPDATE identity_sessions SET revoked_at=clock_timestamp() WHERE id=$1`,
			`UPDATE identity_sessions SET idle_expires_at=created_at+INTERVAL '1 microsecond' WHERE id=$1`,
			`UPDATE identity_sessions SET idle_expires_at=created_at+INTERVAL '1 microsecond',absolute_expires_at=created_at+INTERVAL '1 microsecond' WHERE id=$1`,
		} {
			subject, id, _ := seed(t)
			if _, err := pool.Exec(ctx, update, id); err != nil {
				t.Fatal(err)
			}
			if err := sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("inactive session=%v", err)
			}
		}
		subject, id, _ := seed(t)
		for _, pair := range [][2]string{{"other-subject", id}, {subject, uuid.NewString()}, {subject, "invalid-uuid"}} {
			if err := sensitiveTestChange(ctx, pool, pair[0], pair[1], uuid.NewString(), nil); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("wrong identity=%v", err)
			}
		}
	})
	t.Run("work errors and cancellations roll back the sensitive change", func(t *testing.T) {
		for _, canceled := range []bool{false, true} {
			subject, id, _ := seed(t)
			marker := uuid.NewString()
			request, cancel := context.WithCancel(ctx)
			err := sensitiveTestChange(request, pool, subject, id, marker, func() error {
				if canceled {
					cancel()
					return nil
				}
				return errors.New("private database error")
			})
			cancel()
			want := codes.Unavailable
			if canceled {
				want = codes.Canceled
			}
			if status.Code(err) != want {
				t.Fatalf("failed change = %v, want %v", err, want)
			}
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_accounts WHERE subject=$1`, marker).Scan(&count); err != nil || count != 0 {
				t.Fatalf("failed work changed state: %d, %v", count, err)
			}
		}
	})
}

// switchableProviderIdentity returns a fixed GitHub identity, or err when it is set.
type switchableProviderIdentity struct{ err *error }

func (s switchableProviderIdentity) VerifyProviderIdentity(context.Context, outbound.ProviderCodeExchange) (outbound.ProviderIdentity, error) {
	if *s.err != nil {
		return outbound.ProviderIdentity{}, *s.err
	}
	return outbound.ProviderIdentity{Subject: "github-api-subject", Email: "Other@Example.com", EmailVerified: true}, nil
}

var reauthenticationCallbackCode = regexp.MustCompile(`<code>([A-Za-z0-9_-]{43})</code>`)

// reauthenticationRoutes serves the public re-authentication, login, and
// logout routes through the generated REST gateway, and the provider callback.
type reauthenticationRoutes struct {
	t        *testing.T
	handler  http.Handler
	verifier *token.Verifier
}

func (r reauthenticationRoutes) call(method, path, body, access string, want int) []byte {
	r.t.Helper()
	request := httptest.NewRequestWithContext(r.t.Context(), method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if access != "" {
		request.Header.Set("Authorization", "Bearer "+access)
	}
	request.RemoteAddr = "192.0.2.180:42"
	response := httptest.NewRecorder()
	r.handler.ServeHTTP(response, request)
	if response.Code != want {
		r.t.Fatalf("%s %s status = %d, want %d, body = %s", method, path, response.Code, want, response.Body)
	}
	return response.Body.Bytes()
}

// login creates a password session and returns its access token and session ID.
func (r reauthenticationRoutes) login(email, password string) (string, string) {
	r.t.Helper()
	var session struct{ AccessToken string }
	if err := json.Unmarshal(r.call(http.MethodPost, "/v1/password-sessions", fmt.Sprintf(`{"email":%q,"password":%q}`, email, password), "", http.StatusOK), &session); err != nil {
		r.t.Fatal(err)
	}
	identity, err := r.verifier.Verify(session.AccessToken)
	if err != nil {
		r.t.Fatal(err)
	}
	return session.AccessToken, identity.SessionID
}

// startProvider starts GitHub re-authentication with access and returns the
// attempt token and the callback query.
func (r reauthenticationRoutes) startProvider(access string) (string, string) {
	r.t.Helper()
	var started struct{ AuthorizationURL, AttemptToken string }
	if err := json.Unmarshal(r.call(http.MethodPost, "/v1/provider-reauthentication-attempts", `{"provider":"PROVIDER_GITHUB"}`, access, http.StatusOK), &started); err != nil {
		r.t.Fatal(err)
	}
	authorization, err := url.Parse(started.AuthorizationURL)
	if err != nil {
		r.t.Fatal(err)
	}
	return started.AttemptToken, "code=provider-code&state=" + url.QueryEscape(authorization.Query().Get("state"))
}

// providerProof starts GitHub re-authentication with access, completes the
// callback page, and returns the completion request body.
func (r reauthenticationRoutes) providerProof(access string) string {
	r.t.Helper()
	attempt, query := r.startProvider(access)
	page := r.call(http.MethodGet, "/v1/provider-login-callbacks/github?"+query, "", "", http.StatusOK)
	match := reauthenticationCallbackCode.FindSubmatch(page)
	if match == nil || !strings.Contains(string(page), "Copy your re-authentication code") {
		r.t.Fatalf("callback page = %s", page)
	}
	return fmt.Sprintf(`{"attemptToken":%q,"handoffCode":%q}`, attempt, match[1])
}

func reauthenticationEndTime(t *testing.T, body []byte) time.Time {
	t.Helper()
	var result struct{ ReauthenticationExpiresAt time.Time }
	if err := json.Unmarshal(body, &result); err != nil || result.ReauthenticationExpiresAt.IsZero() {
		t.Fatalf("re-authentication response = %s, %v", body, err)
	}
	return result.ReauthenticationExpiresAt
}

// testReauthenticationAPI runs the re-authentication methods through the
// public REST routes with the production services and PostgreSQL.
func testReauthenticationAPI(t *testing.T, pool *pgxpool.Pool) {
	ctx := t.Context()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := token.NewSigner(privateKey, "reauthentication-api", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := token.NewVerifier(map[string]ed25519.PublicKey{"reauthentication-api": publicKey}, "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{6}, 32)
	var providerErr error
	clients := map[domain.Provider]app.ProviderClient{domain.ProviderGitHub: {
		ClientID: "github-client", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/github", Identity: switchableProviderIdentity{&providerErr},
	}}
	accounts := identitypostgres.NewAccountRepository(pool)
	limits := app.NewLimitService(identitypostgres.NewLimitRepository(pool))
	providerLogin := app.NewProviderLoginService(identitypostgres.NewProviderAttemptRepository(pool), limits.ProviderLoginStart, limits.ProviderCallback, key, clients)
	identity := identityhttp.NewIdentityHandler(nil, nil, nil, nil, verifier, nil).
		WithPasswordLogin(app.NewPasswordLoginService(accounts, signer, limits.PasswordLogin)).
		WithPasswordReauthentication(app.NewPasswordReauthenticationService(accounts, limits.PasswordLogin)).
		WithProviderReauthentication(app.NewProviderReauthenticationService(providerLogin, accounts, limits.ProviderSessionFailure)).
		WithCurrentSessionLogout(app.NewCurrentSessionLogoutService(identitypostgres.NewSessionRepository(pool))).
		WithAllSessionLogout(app.NewAllSessionLogoutService(accounts))
	gateway := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerServer(ctx, gateway, identity); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/provider-login-callbacks/{provider}", identityhttp.NewProviderCallbackHandler(providerLogin, nil))
	mux.Handle("/", identityhttp.NewIdentityRequestHandler(gateway, nil))
	routes := reauthenticationRoutes{t: t, handler: mux, verifier: verifier}

	const subject, email, password = "reauthentication-api", "Reauth.Api@example.com", "correct café battery staple"
	hash, err := domain.HashPassword(ctx, password, func(context.Context, string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO identity_accounts (subject, email_local, email_domain, password_hash, email_verified_at)
		VALUES ($1, 'Reauth.Api', 'example.com', $2, statement_timestamp())`, subject, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO identity_provider_links (provider, provider_subject, account_subject) VALUES ('github', 'github-api-subject', $1)`, subject); err != nil {
		t.Fatal(err)
	}
	closeWindow := func(t *testing.T, sessionID string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=reauthenticated_at-INTERVAL '10 minutes' WHERE id=$1`, sessionID); err != nil {
			t.Fatal(err)
		}
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, sessionID, uuid.NewString(), nil))
	}
	requireInactive := func(t *testing.T, sessionID, step string) {
		t.Helper()
		if err := sensitiveTestChange(ctx, pool, subject, sessionID, uuid.NewString(), nil); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("sensitive change after %s = %v", step, err)
		}
	}

	t.Run("a closed window opens with a password and keeps its end time for a provider proof", func(t *testing.T) {
		access, session := routes.login(email, password)
		if err := sensitiveTestChange(ctx, pool, subject, session, uuid.NewString(), nil); err != nil {
			t.Fatalf("a new session has no open window: %v", err)
		}
		closeWindow(t, session)
		opened := reauthenticationEndTime(t, routes.call(http.MethodPost, "/v1/password-reauthentications", fmt.Sprintf(`{"password":%q}`, password), access, http.StatusOK))
		kept := reauthenticationEndTime(t, routes.call(http.MethodPost, "/v1/provider-reauthentications", routes.providerProof(access), access, http.StatusOK))
		if !kept.Equal(opened) {
			t.Fatalf("the provider proof moved the window from %v to %v", opened, kept)
		}
		if err := sensitiveTestChange(ctx, pool, subject, session, uuid.NewString(), nil); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a provider proof of 1 session leaves another session closed", func(t *testing.T) {
		first, firstSession := routes.login(email, password)
		second, secondSession := routes.login(email, password)
		closeWindow(t, firstSession)
		closeWindow(t, secondSession)
		proof := routes.providerProof(first)
		routes.call(http.MethodPost, "/v1/provider-reauthentications", proof, second, http.StatusBadRequest)
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, secondSession, uuid.NewString(), nil))
		reauthenticationEndTime(t, routes.call(http.MethodPost, "/v1/provider-reauthentications", proof, first, http.StatusOK))
		if err := sensitiveTestChange(ctx, pool, subject, firstSession, uuid.NewString(), nil); err != nil {
			t.Fatal(err)
		}
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, secondSession, uuid.NewString(), nil))
	})

	t.Run("an unavailable database or provider keeps a closed window closed", func(t *testing.T) {
		access, session := routes.login(email, password)
		closeWindow(t, session)
		closed, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
		if err != nil {
			t.Fatal(err)
		}
		closed.Close()
		unavailable := identitypostgres.NewAccountRepository(closed)
		passwordService := app.NewPasswordReauthenticationService(unavailable, limits.PasswordLogin)
		if _, err := passwordService.ReauthenticateWithPassword(ctx, inbound.ReauthenticateWithPasswordInput{Subject: subject, SessionID: session, Password: password, Source: "192.0.2.182"}); !errors.Is(err, app.ErrReauthenticationUnavailable) {
			t.Fatalf("password proof with an unavailable database = %v", err)
		}
		codeService := app.NewCodeReauthenticationService(unavailable, failingDeliveryProtector{}, limits.CodeRequest, limits.WrongCode, limits.AccountWrongCode, key)
		if _, err := codeService.ReauthenticateWithCode(ctx, inbound.ReauthenticateWithCodeInput{Subject: subject, SessionID: session, Code: "012345", Source: "192.0.2.182"}); !errors.Is(err, app.ErrReauthenticationUnavailable) {
			t.Fatalf("code proof with an unavailable database = %v", err)
		}
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, session, uuid.NewString(), nil))

		providerErr = domain.ErrProviderUnavailable
		attempt, query := routes.startProvider(access)
		page := routes.call(http.MethodGet, "/v1/provider-login-callbacks/github?"+query, "", "", http.StatusServiceUnavailable)
		providerErr = nil
		if reauthenticationCallbackCode.Match(page) {
			t.Fatalf("an unavailable provider produced a handoff code: %s", page)
		}
		guessed, err := domain.NewProviderHandoffCode()
		if err != nil {
			t.Fatal(err)
		}
		routes.call(http.MethodPost, "/v1/provider-reauthentications", fmt.Sprintf(`{"attemptToken":%q,"handoffCode":%q}`, attempt, guessed), access, http.StatusBadRequest)
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, session, uuid.NewString(), nil))

		proof := routes.providerProof(access)
		var request struct{ AttemptToken, HandoffCode string }
		if err := json.Unmarshal([]byte(proof), &request); err != nil {
			t.Fatal(err)
		}
		providerService := app.NewProviderReauthenticationService(providerLogin, unavailable, limits.ProviderSessionFailure)
		if _, err := providerService.CompleteProviderReauthentication(ctx, inbound.CompleteProviderReauthenticationInput{
			Subject: subject, SessionID: session, AttemptToken: request.AttemptToken, HandoffCode: request.HandoffCode, Source: "192.0.2.182",
		}); !errors.Is(err, app.ErrReauthenticationUnavailable) {
			t.Fatalf("provider completion with an unavailable database = %v", err)
		}
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, session, uuid.NewString(), nil))
		// The outage consumed nothing, so the same handoff completes later.
		reauthenticationEndTime(t, routes.call(http.MethodPost, "/v1/provider-reauthentications", proof, access, http.StatusOK))
	})

	t.Run("logout, all-session logout, and password reset end an open window", func(t *testing.T) {
		access, session := routes.login(email, password)
		routes.call(http.MethodPost, "/v1/session-logouts", `{}`, access, http.StatusOK)
		requireInactive(t, session, "logout")
		access, session = routes.login(email, password)
		routes.call(http.MethodPost, "/v1/account-session-logouts", `{}`, access, http.StatusOK)
		requireInactive(t, session, "all-session logout")
		access, session = routes.login(email, password)
		if err := sensitiveTestChange(ctx, pool, subject, session, uuid.NewString(), nil); err != nil {
			t.Fatal(err)
		}
		code, verifier, _, err := domain.NewChallenge(key, subject, email, domain.PurposePasswordReset, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO identity_challenges (account_subject, purpose, email_local, email_domain, code_verifier, expires_at)
			VALUES ($1, 'password-reset', 'Reauth.Api', 'example.com', $2, statement_timestamp() + INTERVAL '10 minutes')`, subject, verifier[:]); err != nil {
			t.Fatal(err)
		}
		allow := func(context.Context, string) error { return nil }
		reset := app.NewPasswordResetService(accounts, allow, allow, allow, func(context.Context, string) (bool, error) { return false, nil }, key)
		if err := reset.ResetPassword(ctx, inbound.ResetPasswordInput{Email: email, Code: code, NewPassword: password + " again", Source: "192.0.2.181"}); err != nil {
			t.Fatal(err)
		}
		requireInactive(t, session, "password reset")
		routes.call(http.MethodPost, "/v1/password-reauthentications", fmt.Sprintf(`{"password":%q}`, password+" again"), access, http.StatusUnauthorized)
	})
}
