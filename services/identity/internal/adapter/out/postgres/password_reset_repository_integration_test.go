//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

func testPasswordResetRepository(t *testing.T, pool *pgxpool.Pool) {
	ctx := context.Background()
	const subject = "password-reset-subject"
	const email = "Reset@example.com"
	const oldPassword = "old password 12345"
	const newPassword = "new password 12345"
	compromised := func(context.Context, string) (bool, error) { return false, nil }
	oldHash, err := domain.HashPassword(ctx, oldPassword, compromised)
	if err != nil {
		t.Fatal(err)
	}
	queries := identitysqlc.New(pool)
	if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
		Subject: subject, EmailLocal: "Reset", EmailDomain: "example.com", PasswordHash: oldHash,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.MarkEmailVerified(ctx, subject); err != nil {
		t.Fatal(err)
	}
	var verifiedAt time.Time
	if err := pool.QueryRow(ctx, `SELECT email_verified_at FROM identity_accounts WHERE subject=$1`, subject).Scan(&verifiedAt); err != nil {
		t.Fatal(err)
	}
	oldRefresh := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	oldSessionID := ""
	for _, seed := range []byte{7, 8} {
		hash := sha256.Sum256(bytes.Repeat([]byte{seed}, 32))
		session, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: hash[:]})
		if err != nil {
			t.Fatal(err)
		}
		if oldSessionID == "" {
			oldSessionID = uuid.UUID(session.ID.Bytes).String()
		}
	}
	key := bytes.Repeat([]byte{9}, 32)
	code, verifier, _, err := domain.NewChallenge(key, subject, email, domain.PurposePasswordReset, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
		AccountSubject: subject, Purpose: string(domain.PurposePasswordReset), EmailLocal: "Reset",
		EmailDomain: "example.com", CodeVerifier: verifier[:],
	})
	if err != nil {
		t.Fatal(err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := token.NewSigner(privateKey, "reset-key", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	repo := identitypostgres.NewAccountRepository(pool)
	allow := func(context.Context, string) error { return nil }
	service := app.NewPasswordResetService(repo, allow, allow, allow, compromised, key)
	input := inbound.ResetPasswordInput{Email: email, Code: code, NewPassword: newPassword, Source: "192.0.2.50"}

	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_reset_notice() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN RAISE EXCEPTION 'notice store unavailable'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER reject_reset_notice BEFORE INSERT ON identity_password_change_notices
	FOR EACH ROW EXECUTE FUNCTION reject_reset_notice()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP TRIGGER IF EXISTS reject_reset_notice ON identity_password_change_notices`)
		_, _ = pool.Exec(ctx, `DROP FUNCTION IF EXISTS reject_reset_notice()`)
	})
	if err := service.ResetPassword(ctx, input); !errors.Is(err, app.ErrPasswordResetUnavailable) {
		t.Fatalf("notice failure = %v", err)
	}
	var storedHash string
	var activeSessions int
	var consumed bool
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM identity_accounts WHERE subject=$1`, subject).Scan(&storedHash); err != nil || storedHash != oldHash {
		t.Fatalf("password changed despite notice failure: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_sessions WHERE account_subject=$1 AND revoked_at IS NULL`, subject).Scan(&activeSessions); err != nil || activeSessions != 2 {
		t.Fatalf("sessions revoked despite notice failure: %d, %v", activeSessions, err)
	}
	if err := pool.QueryRow(ctx, `SELECT consumed_at IS NOT NULL FROM identity_challenges WHERE id=$1`, challenge.ID).Scan(&consumed); err != nil || consumed {
		t.Fatalf("code consumed despite notice failure: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_reset_notice ON identity_password_change_notices`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DROP FUNCTION reject_reset_notice()`); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() { defer workers.Done(); <-start; results <- service.ResetPassword(ctx, input) }()
	}
	close(start)
	workers.Wait()
	close(results)
	success, rejected := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, app.ErrInvalidPasswordResetCode):
			rejected++
		default:
			t.Fatalf("concurrent reset = %v", err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("concurrent reset: success=%d rejected=%d", success, rejected)
	}
	if err := service.ResetPassword(ctx, input); !errors.Is(err, app.ErrInvalidPasswordResetCode) {
		t.Fatalf("consumed code = %v", err)
	}
	wrongPurposeCode, wrongPurposeVerifier, _, err := domain.NewChallenge(key, subject, email, domain.PurposeVerifyEmail, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
		AccountSubject: subject, Purpose: string(domain.PurposeVerifyEmail), EmailLocal: "Reset",
		EmailDomain: "example.com", CodeVerifier: wrongPurposeVerifier[:],
	}); err != nil {
		t.Fatal(err)
	}
	wrongPurpose := input
	wrongPurpose.Code = wrongPurposeCode
	if err := service.ResetPassword(ctx, wrongPurpose); !errors.Is(err, app.ErrInvalidPasswordResetCode) {
		t.Fatalf("wrong-purpose code = %v", err)
	}
	var notices int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_password_change_notices WHERE account_subject=$1 AND email_local='Reset'`, subject).Scan(&notices); err != nil || notices != 1 {
		t.Fatalf("queued notices=%d error=%v", notices, err)
	}
	var verifiedAfter time.Time
	var accountEmail string
	if err := pool.QueryRow(ctx, `SELECT email_verified_at, email_local || '@' || email_domain FROM identity_accounts WHERE subject=$1 AND retired_at IS NULL`,
		subject).Scan(&verifiedAfter, &accountEmail); err != nil || !verifiedAfter.Equal(verifiedAt) || accountEmail != email {
		t.Fatalf("reset changed the account identity: verified=%v email=%v error=%v", verifiedAfter.Equal(verifiedAt), accountEmail == email, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_sessions WHERE account_subject=$1 AND revoked_at IS NULL`, subject).Scan(&activeSessions); err != nil || activeSessions != 0 {
		t.Fatalf("active old sessions=%d error=%v", activeSessions, err)
	}
	if _, err := app.NewSessionCheckService(identitypostgres.NewSessionRepository(pool)).CheckSession(ctx,
		inbound.CheckSessionInput{Subject: subject, SessionID: oldSessionID}); err == nil {
		t.Fatal("old access session still works")
	}
	refresh := app.NewSessionRefreshService(identitypostgres.NewSessionRefreshRepository(pool), signer)
	if _, err := refresh.RefreshSession(ctx, oldRefresh); !errors.Is(err, app.ErrUnauthenticatedRefresh) {
		t.Fatalf("old refresh still works: %v", err)
	}
	login := app.NewPasswordLoginService(repo, signer, func(context.Context, string, string) error { return nil })
	if _, err := login.CreatePasswordSession(ctx, inbound.CreatePasswordSessionInput{Email: email, Password: oldPassword, Source: input.Source}); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("old password still works: %v", err)
	}
	fresh, err := login.CreatePasswordSession(ctx, inbound.CreatePasswordSessionInput{Email: email, Password: newPassword, Source: input.Source})
	if err != nil || fresh.AccessToken == "" {
		t.Fatalf("new password login: %v", err)
	}

	// Keep the reset account lock while old-password login and refresh race it.
	secondCode, secondVerifier, _, err := domain.NewChallenge(key, subject, email, domain.PurposePasswordReset, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
		AccountSubject: subject, Purpose: string(domain.PurposePasswordReset), EmailLocal: "Reset",
		EmailDomain: "example.com", CodeVerifier: secondVerifier[:],
	}); err != nil {
		t.Fatal(err)
	}
	wrongCode := "000000"
	if secondCode == wrongCode {
		wrongCode = "000001"
	}
	wrongInput := inbound.ResetPasswordInput{Email: email, Code: wrongCode, NewPassword: "third password 12345", Source: input.Source}
	if err := service.ResetPassword(ctx, wrongInput); !errors.Is(err, app.ErrInvalidPasswordResetCode) {
		t.Fatalf("wrong code = %v", err)
	}
	var wrongGuesses int
	if err := pool.QueryRow(ctx, `SELECT wrong_guesses FROM identity_challenges WHERE account_subject=$1 AND purpose='password-reset' AND consumed_at IS NULL`, subject).Scan(&wrongGuesses); err != nil || wrongGuesses != 1 {
		t.Fatalf("wrong guesses=%d error=%v", wrongGuesses, err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	blocking := app.NewPasswordResetService(repo, allow, allow, allow, func(context.Context, string) (bool, error) {
		close(entered)
		<-release
		return false, nil
	}, key)
	resetDone := make(chan error, 1)
	go func() {
		resetDone <- blocking.ResetPassword(ctx, inbound.ResetPasswordInput{
			Email: email, Code: secondCode, NewPassword: "third password 12345", Source: input.Source,
		})
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("reset did not acquire account lock")
	}
	loginDone := make(chan error, 1)
	refreshDone := make(chan error, 1)
	go func() {
		_, err := login.CreatePasswordSession(ctx, inbound.CreatePasswordSessionInput{Email: email, Password: newPassword, Source: input.Source})
		loginDone <- err
	}()
	go func() { _, err := refresh.RefreshSession(ctx, fresh.RefreshToken); refreshDone <- err }()
	select {
	case err := <-loginDone:
		t.Fatalf("old-password login committed during reset: %v", err)
	case err := <-refreshDone:
		t.Fatalf("old refresh committed during reset: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-resetDone; err != nil {
		t.Fatalf("racing reset: %v", err)
	}
	if err := <-loginDone; !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("racing old-password login: %v", err)
	}
	if err := <-refreshDone; !errors.Is(err, app.ErrUnauthenticatedRefresh) {
		t.Fatalf("racing old refresh: %v", err)
	}

	for _, terminal := range []string{"expired", "replaced"} {
		terminalCode, terminalVerifier, _, err := domain.NewChallenge(key, subject, email, domain.PurposePasswordReset, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		challenge, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
			AccountSubject: subject, Purpose: string(domain.PurposePasswordReset), EmailLocal: "Reset",
			EmailDomain: "example.com", CodeVerifier: terminalVerifier[:],
		})
		if err != nil {
			t.Fatal(err)
		}
		if terminal == "expired" {
			_, err = pool.Exec(ctx, `UPDATE identity_challenges SET issued_at=issued_at-INTERVAL '11 minutes',
				expires_at=expires_at-INTERVAL '11 minutes' WHERE id=$1`, challenge.ID)
		} else {
			_, err = pool.Exec(ctx, `UPDATE identity_challenges SET replaced_at=statement_timestamp() WHERE id=$1`, challenge.ID)
		}
		if err != nil {
			t.Fatal(err)
		}
		terminalInput := input
		terminalInput.Code = terminalCode
		if err := service.ResetPassword(ctx, terminalInput); !errors.Is(err, app.ErrInvalidPasswordResetCode) {
			t.Fatalf("%s code = %v", terminal, err)
		}
		if terminal == "expired" {
			if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET replaced_at=statement_timestamp() WHERE id=$1`, challenge.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	exhaustedCode, exhaustedVerifier, _, err := domain.NewChallenge(key, subject, email, domain.PurposePasswordReset, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
		AccountSubject: subject, Purpose: string(domain.PurposePasswordReset), EmailLocal: "Reset",
		EmailDomain: "example.com", CodeVerifier: exhaustedVerifier[:],
	}); err != nil {
		t.Fatal(err)
	}
	wrongCode = "000000"
	if exhaustedCode == wrongCode {
		wrongCode = "000001"
	}
	for range 5 {
		if err := service.ResetPassword(ctx, inbound.ResetPasswordInput{
			Email: email, Code: wrongCode,
			NewPassword: "fourth password 12345", Source: input.Source,
		}); !errors.Is(err, app.ErrInvalidPasswordResetCode) {
			t.Fatalf("wrong guess = %v", err)
		}
	}
	if err := service.ResetPassword(ctx, inbound.ResetPasswordInput{
		Email: email, Code: exhaustedCode,
		NewPassword: "fourth password 12345", Source: input.Source,
	}); !errors.Is(err, app.ErrInvalidPasswordResetCode) {
		t.Fatalf("exhausted code = %v", err)
	}
}
