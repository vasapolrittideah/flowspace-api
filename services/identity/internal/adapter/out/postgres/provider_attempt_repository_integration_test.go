//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type storedProviderAttempt struct {
	provider, codeVerifier, callbackURL string
	nonce                               *string
	tokenVerifier, stateVerifier        []byte
	createdAt, expiresAt                time.Time
}

func testProviderAttemptRepository(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	key := bytes.Repeat([]byte{7}, 32)
	repository := postgres.NewProviderAttemptRepository(pool)
	service := app.NewProviderLoginService(repository, func(context.Context, string) error { return nil }, func(context.Context, string) error { return nil }, key,
		map[domain.Provider]app.ProviderClient{
			domain.ProviderGoogle: {ClientID: "google-client", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/google"},
			domain.ProviderGitHub: {ClientID: "github-client", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/github"},
		})
	load := func(t *testing.T, token string) storedProviderAttempt {
		t.Helper()
		verifier := domain.ProviderSecretVerifier(key, domain.ProviderSecretAttemptToken, token)
		var attempt storedProviderAttempt
		err := pool.QueryRow(ctx, `SELECT provider, code_verifier, callback_url, nonce, attempt_token_verifier, state_verifier, created_at, expires_at
			FROM identity_provider_login_attempts WHERE attempt_token_verifier = $1`, verifier[:]).Scan(
			&attempt.provider, &attempt.codeVerifier, &attempt.callbackURL, &attempt.nonce,
			&attempt.tokenVerifier, &attempt.stateVerifier, &attempt.createdAt, &attempt.expiresAt)
		if err != nil {
			t.Fatal(err)
		}
		return attempt
	}

	t.Run("provider attempts bind proofs and expire after ten minutes", func(t *testing.T) {
		for _, provider := range []string{"google", "github"} {
			before := time.Now()
			result, err := service.StartProviderLogin(ctx, inbound.StartProviderLoginInput{Provider: provider, Source: "192.0.2.1"})
			if err != nil {
				t.Fatal(err)
			}
			authorization, err := url.Parse(result.AuthorizationURL)
			if err != nil {
				t.Fatal(err)
			}
			query := authorization.Query()
			attempt := load(t, result.AttemptToken)
			state := domain.ProviderSecretVerifier(key, domain.ProviderSecretState, query.Get("state"))
			if attempt.provider != provider || attempt.callbackURL != query.Get("redirect_uri") || !bytes.Equal(attempt.stateVerifier, state[:]) ||
				domain.ProviderCodeChallenge(attempt.codeVerifier) != query.Get("code_challenge") {
				t.Fatalf("%s attempt does not match its authorization URL", provider)
			}
			if (provider == "google") != (attempt.nonce != nil) || attempt.nonce != nil && *attempt.nonce != query.Get("nonce") {
				t.Fatalf("%s attempt nonce does not match", provider)
			}
			if attempt.expiresAt.Sub(attempt.createdAt) != 10*time.Minute || !result.ExpiresAt.Equal(attempt.expiresAt) ||
				result.ExpiresAt.Before(before.Add(9*time.Minute)) || result.ExpiresAt.After(time.Now().Add(11*time.Minute)) {
				t.Fatalf("%s attempt expires at %v", provider, result.ExpiresAt)
			}
		}
	})

	t.Run("a retried start creates a new attempt and keeps the old one", func(t *testing.T) {
		first, err := service.StartProviderLogin(ctx, inbound.StartProviderLoginInput{Provider: "google", Source: "192.0.2.1"})
		if err != nil {
			t.Fatal(err)
		}
		original := load(t, first.AttemptToken)
		second, err := service.StartProviderLogin(ctx, inbound.StartProviderLoginInput{Provider: "google", Source: "192.0.2.1"})
		if err != nil {
			t.Fatal(err)
		}
		retried := load(t, second.AttemptToken)
		unchanged := load(t, first.AttemptToken)
		if bytes.Equal(original.stateVerifier, retried.stateVerifier) || !bytes.Equal(original.stateVerifier, unchanged.stateVerifier) ||
			!original.expiresAt.Equal(unchanged.expiresAt) || original.codeVerifier != unchanged.codeVerifier {
			t.Fatal("retry changed the old attempt or reused its proofs")
		}
	})

	t.Run("attempt verifiers are unique", func(t *testing.T) {
		attempt := outbound.ProviderAttempt{
			Provider: "github", AttemptTokenVerifier: [32]byte{1}, StateVerifier: [32]byte{2},
			CodeVerifier: string(bytes.Repeat([]byte{'a'}, 43)), CallbackURL: "https://api.example.com/v1/provider-login-callbacks/github",
		}
		if _, err := repository.CreateProviderAttempt(ctx, attempt); err != nil {
			t.Fatal(err)
		}
		attempt.AttemptTokenVerifier = [32]byte{3}
		if _, err := repository.CreateProviderAttempt(ctx, attempt); err == nil {
			t.Fatal("stored a second attempt with the same state")
		}
		attempt.AttemptTokenVerifier, attempt.StateVerifier = [32]byte{1}, [32]byte{4}
		if _, err := repository.CreateProviderAttempt(ctx, attempt); err == nil {
			t.Fatal("stored a second attempt with the same attempt token")
		}
	})

	t.Run("a callback consumes the state once and stores one result", func(t *testing.T) {
		attempt := outbound.ProviderAttempt{
			Provider: "google", AttemptTokenVerifier: [32]byte{10}, StateVerifier: [32]byte{11},
			CodeVerifier: string(bytes.Repeat([]byte{'b'}, 43)), Nonce: "nonce", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/google",
		}
		if _, err := repository.CreateProviderAttempt(ctx, attempt); err != nil {
			t.Fatal(err)
		}
		if _, found, err := repository.ConsumeProviderState(ctx, "github", attempt.StateVerifier); err != nil || found {
			t.Fatalf("state was accepted on the wrong provider route: %t, %v", found, err)
		}
		proof, found, err := repository.ConsumeProviderState(ctx, "google", attempt.StateVerifier)
		if err != nil || !found || proof.CodeVerifier != attempt.CodeVerifier || proof.Nonce != "nonce" || proof.CallbackURL != attempt.CallbackURL {
			t.Fatalf("proof = %+v, %t, %v", proof, found, err)
		}
		if _, found, err := repository.ConsumeProviderState(ctx, "google", attempt.StateVerifier); err != nil || found {
			t.Fatalf("state was consumed twice: %t, %v", found, err)
		}
		identity := outbound.ProviderIdentity{Subject: "google-subject", Email: "user@gmail.com", EmailVerified: true}
		if stored, err := repository.RecordProviderResult(ctx, proof.ID, identity, [32]byte{12}); err != nil || !stored {
			t.Fatalf("result stored = %t, %v", stored, err)
		}
		if stored, err := repository.RecordProviderResult(ctx, proof.ID, outbound.ProviderIdentity{Subject: "other"}, [32]byte{13}); err != nil || stored {
			t.Fatalf("result was replaced: %t, %v", stored, err)
		}
		if err := repository.FailProviderAttempt(ctx, proof.ID); err != nil {
			t.Fatal(err)
		}
		var subject string
		var failed bool
		err = pool.QueryRow(ctx, `SELECT provider_subject, failed_at IS NOT NULL FROM identity_provider_login_attempts WHERE id = $1`, proof.ID).Scan(&subject, &failed)
		if err != nil || subject != "google-subject" || failed {
			t.Fatalf("stored result = %q, failed %t, %v", subject, failed, err)
		}
	})

	t.Run("a failed callback stores no result", func(t *testing.T) {
		attempt := outbound.ProviderAttempt{
			Provider: "google", AttemptTokenVerifier: [32]byte{20}, StateVerifier: [32]byte{21},
			CodeVerifier: string(bytes.Repeat([]byte{'c'}, 43)), Nonce: "nonce", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/google",
		}
		if _, err := repository.CreateProviderAttempt(ctx, attempt); err != nil {
			t.Fatal(err)
		}
		proof, found, err := repository.ConsumeProviderState(ctx, "google", attempt.StateVerifier)
		if err != nil || !found {
			t.Fatalf("consume = %t, %v", found, err)
		}
		if err := repository.FailProviderAttempt(ctx, proof.ID); err != nil {
			t.Fatal(err)
		}
		if stored, err := repository.RecordProviderResult(ctx, proof.ID, outbound.ProviderIdentity{Subject: "google-subject"}, [32]byte{22}); err != nil || stored {
			t.Fatalf("failed attempt stored a result: %t, %v", stored, err)
		}
	})

	t.Run("an expired attempt cannot be consumed", func(t *testing.T) {
		attempt := outbound.ProviderAttempt{
			Provider: "github", AttemptTokenVerifier: [32]byte{30}, StateVerifier: [32]byte{31},
			CodeVerifier: string(bytes.Repeat([]byte{'d'}, 43)), CallbackURL: "https://api.example.com/v1/provider-login-callbacks/github",
		}
		if _, err := repository.CreateProviderAttempt(ctx, attempt); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE identity_provider_login_attempts
			SET created_at = created_at - INTERVAL '11 minutes', expires_at = expires_at - INTERVAL '11 minutes'
			WHERE state_verifier = $1`, attempt.StateVerifier[:]); err != nil {
			t.Fatal(err)
		}
		if _, found, err := repository.ConsumeProviderState(ctx, "github", attempt.StateVerifier); err != nil || found {
			t.Fatalf("expired state was consumed: %t, %v", found, err)
		}
	})

	t.Run("concurrent callbacks consume one state once", func(t *testing.T) {
		attempt := outbound.ProviderAttempt{
			Provider: "google", AttemptTokenVerifier: [32]byte{40}, StateVerifier: [32]byte{41},
			CodeVerifier: string(bytes.Repeat([]byte{'e'}, 43)), Nonce: "nonce", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/google",
		}
		if _, err := repository.CreateProviderAttempt(ctx, attempt); err != nil {
			t.Fatal(err)
		}
		var group sync.WaitGroup
		results := make(chan bool, 10)
		for range 10 {
			group.Go(func() {
				_, found, err := repository.ConsumeProviderState(ctx, "google", attempt.StateVerifier)
				if err != nil {
					t.Error(err)
				}
				results <- found
			})
		}
		group.Wait()
		close(results)
		consumed := 0
		for found := range results {
			if found {
				consumed++
			}
		}
		if consumed != 1 {
			t.Fatalf("state consumed %d times", consumed)
		}
	})
}
