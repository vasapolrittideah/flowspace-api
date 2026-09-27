//go:build integration

package postgres_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

func testPasswordLoginRepository(t *testing.T, pool *pgxpool.Pool) {
	ctx := context.Background()
	hash, err := domain.HashPassword(ctx, "correct horse battery staple", func(context.Context, string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	queries := identitysqlc.New(pool)
	if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{Subject: "login-subject", EmailLocal: "Login", EmailDomain: "example.com", PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := token.NewSigner(privateKey, "login-key", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	repo := identitypostgres.NewAccountRepository(pool)
	service := app.NewPasswordLoginService(repo, signer, func(context.Context, string, string) error { return nil })
	input := inbound.CreatePasswordSessionInput{Email: "Login@EXAMPLE.COM", Password: "correct horse battery staple", Source: "192.0.2.1"}
	result, err := service.CreatePasswordSession(ctx, input)
	if err != nil || result.Subject != "login-subject" || result.EmailVerified || result.AccessToken == "" || result.RefreshToken == "" {
		t.Fatalf("unverified login failed: %v", err)
	}
	verifier, err := token.NewVerifier(publicKey, "login-key", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := verifier.Verify(result.AccessToken)
	if err != nil || identity.Subject != "login-subject" || identity.SessionID == "" {
		t.Fatalf("invalid access token: %v", err)
	}
	verified, err := app.NewSessionCheckService(identitypostgres.NewSessionRepository(pool)).CheckSession(ctx, inbound.CheckSessionInput{Subject: identity.Subject, SessionID: identity.SessionID})
	if err != nil || verified {
		t.Fatalf("new session check = %t, error = %v", verified, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_sessions WHERE account_subject = $1`, "login-subject").Scan(&count); err != nil || count != 1 {
		t.Fatalf("session count = %d, error = %v", count, err)
	}
	if _, err := queries.MarkEmailVerified(ctx, "login-subject"); err != nil {
		t.Fatal(err)
	}
	result, err = service.CreatePasswordSession(ctx, input)
	if err != nil || !result.EmailVerified {
		t.Fatalf("verified login failed: %v", err)
	}
	for _, invalid := range []inbound.CreatePasswordSessionInput{
		{Email: "missing@example.com", Password: input.Password, Source: input.Source},
		{Email: input.Email, Password: "wrong-password", Source: input.Source},
	} {
		result, err := service.CreatePasswordSession(ctx, invalid)
		if !errors.Is(err, app.ErrInvalidCredentials) || result.AccessToken != "" {
			t.Fatalf("invalid login returned %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_accounts SET email_verified_at = NULL, retired_at = statement_timestamp() WHERE subject = $1`, "login-subject"); err != nil {
		t.Fatal(err)
	}
	result, err = service.CreatePasswordSession(ctx, input)
	if !errors.Is(err, app.ErrInvalidCredentials) || result.AccessToken != "" {
		t.Fatalf("retired login returned %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_sessions WHERE account_subject = $1`, "login-subject").Scan(&count); err != nil || count != 2 {
		t.Fatalf("final session count = %d, error = %v", count, err)
	}
}
