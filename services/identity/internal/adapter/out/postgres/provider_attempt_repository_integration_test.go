//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	deliverycrypto "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
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

type providerHandoff struct{ token, code string }

func testProviderSessionRepository(t *testing.T, ctx context.Context, pool *pgxpool.Pool, dsn string) {
	key := bytes.Repeat([]byte{8}, 32)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := token.NewSigner(privateKey, "provider-key", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := token.NewVerifier(map[string]ed25519.PublicKey{"provider-key": publicKey}, "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	protector, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{3}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	allow := func(context.Context, string) error { return nil }
	newService := func(pool *pgxpool.Pool) *app.ProviderLoginService {
		limits := app.NewLimitService(postgres.NewLimitRepository(pool))
		return app.NewProviderLoginService(postgres.NewProviderAttemptRepository(pool), allow, allow, key, nil).
			WithSessions(postgres.NewAccountRepository(pool), signer, protector, limits.ProviderSessionFailure)
	}
	service := newService(pool)
	attempts := postgres.NewProviderAttemptRepository(pool)
	queries := identitysqlc.New(pool)
	seed := func(t *testing.T, providerSubject string, identity outbound.ProviderIdentity) providerHandoff {
		t.Helper()
		handoff := providerHandoff{}
		for _, target := range []*string{&handoff.token, &handoff.code} {
			value, err := domain.NewProviderHandoffCode()
			if err != nil {
				t.Fatal(err)
			}
			*target = value
		}
		state := [32]byte{}
		copy(state[:], providerSubject+handoff.token)
		attempt := outbound.ProviderAttempt{
			Provider: "google", AttemptTokenVerifier: domain.ProviderSecretVerifier(key, domain.ProviderSecretAttemptToken, handoff.token),
			StateVerifier: state, CodeVerifier: strings.Repeat("v", 43), Nonce: "nonce", CallbackURL: "https://api.example.com/v1/provider-login-callbacks/google",
		}
		if _, err := attempts.CreateProviderAttempt(ctx, attempt); err != nil {
			t.Fatal(err)
		}
		proof, found, err := attempts.ConsumeProviderState(ctx, "google", state)
		if err != nil || !found {
			t.Fatalf("consume = %t, %v", found, err)
		}
		identity.Subject = providerSubject
		stored, err := attempts.RecordProviderResult(ctx, proof.ID, identity, domain.ProviderSecretVerifier(key, domain.ProviderSecretHandoffCode, handoff.code))
		if err != nil || !stored {
			t.Fatalf("stored = %t, %v", stored, err)
		}
		return handoff
	}
	link := func(t *testing.T, subject, providerSubject string, verified bool) {
		t.Helper()
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: subject, EmailLocal: subject, EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
		if verified {
			if _, err := pool.Exec(ctx, `UPDATE identity_accounts SET email_verified_at = statement_timestamp() WHERE subject = $1`, subject); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := pool.Exec(ctx, `INSERT INTO identity_provider_links (provider, provider_subject, account_subject) VALUES ('google', $1, $2)`,
			providerSubject, subject); err != nil {
			t.Fatal(err)
		}
	}
	claim := func(service *app.ProviderLoginService, handoff providerHandoff, source string) (inbound.CreateProviderSessionResult, error) {
		return service.CreateProviderSession(ctx, inbound.CreateProviderSessionInput{AttemptToken: handoff.token, HandoffCode: handoff.code, Source: source})
	}
	sessions := func(t *testing.T, subject string) int {
		t.Helper()
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_sessions WHERE account_subject = $1`, subject).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}

	t.Run("a provider link is unique", func(t *testing.T) {
		link(t, "link-owner", "google-unique", false)
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: "link-other", EmailLocal: "link-other", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO identity_provider_links (provider, provider_subject, account_subject) VALUES ('google', 'google-unique', 'link-other')`); err == nil {
			t.Fatal("linked one provider identity twice")
		}
	})

	t.Run("a linked identity keeps the account email and state when its provider email changes", func(t *testing.T) {
		link(t, "provider-linked", "google-linked", true)
		for _, identity := range []outbound.ProviderIdentity{
			{Email: "changed@gmail.com", EmailVerified: true},
			{Email: "unverified@example.org"},
			{},
		} {
			handoff := seed(t, "google-linked", identity)
			result, err := claim(service, handoff, "192.0.2.80")
			if err != nil || result.Subject != "provider-linked" || !result.EmailVerified || result.AccessToken == "" || result.RefreshToken == "" ||
				!result.SessionExpiresAt.After(result.RefreshTokenExpiresAt) || !result.RefreshTokenExpiresAt.After(result.AccessTokenExpiresAt) {
				t.Fatalf("result = %+v, %v", result.Subject, err)
			}
			if _, err := claim(service, handoff, "192.0.2.80"); !errors.Is(err, app.ErrProviderHandoffRejected) {
				t.Fatalf("claimed a handoff twice: %v", err)
			}
		}
		var local string
		var verified bool
		if err := pool.QueryRow(ctx, `SELECT email_local, email_verified_at IS NOT NULL FROM identity_accounts WHERE subject = 'provider-linked'`).Scan(&local, &verified); err != nil ||
			local != "provider-linked" || !verified {
			t.Fatalf("account email = %q, verified %t, %v", local, verified, err)
		}
		if count := sessions(t, "provider-linked"); count != 3 {
			t.Fatalf("session count = %d", count)
		}
	})

	t.Run("an unlinked identity is consumed without a session", func(t *testing.T) {
		handoff := seed(t, "google-unlinked", outbound.ProviderIdentity{Email: "new@gmail.com"})
		if _, err := claim(service, handoff, "192.0.2.81"); !errors.Is(err, app.ErrProviderAccountUnavailable) {
			t.Fatalf("err = %v", err)
		}
		if _, err := claim(service, handoff, "192.0.2.81"); !errors.Is(err, app.ErrProviderHandoffRejected) {
			t.Fatalf("retry err = %v", err)
		}
	})

	t.Run("wrong, expired, and exhausted proofs create no session", func(t *testing.T) {
		link(t, "provider-proofs", "google-proofs", false)
		handoff := seed(t, "google-proofs", outbound.ProviderIdentity{})
		other := seed(t, "google-proofs", outbound.ProviderIdentity{})
		if _, err := claim(service, providerHandoff{handoff.token, other.code}, "192.0.2.82"); !errors.Is(err, app.ErrProviderHandoffRejected) {
			t.Fatalf("mixed proofs err = %v", err)
		}
		for range 4 {
			wrong := providerHandoff{handoff.token, strings.Repeat("A", 43)}
			if _, err := claim(service, wrong, "192.0.2.82"); !errors.Is(err, app.ErrProviderHandoffRejected) {
				t.Fatalf("wrong code err = %v", err)
			}
		}
		if _, err := claim(service, handoff, "192.0.2.82"); !errors.Is(err, app.ErrProviderHandoffRejected) {
			t.Fatalf("exhausted attempt err = %v", err)
		}
		verifier := domain.ProviderSecretVerifier(key, domain.ProviderSecretAttemptToken, other.token)
		if _, err := pool.Exec(ctx, `UPDATE identity_provider_login_attempts
			SET created_at = created_at - INTERVAL '11 minutes', expires_at = expires_at - INTERVAL '11 minutes'
			WHERE attempt_token_verifier = $1`, verifier[:]); err != nil {
			t.Fatal(err)
		}
		if _, err := claim(service, other, "192.0.2.82"); !errors.Is(err, app.ErrProviderHandoffRejected) {
			t.Fatalf("expired attempt err = %v", err)
		}
		if count := sessions(t, "provider-proofs"); count != 0 {
			t.Fatalf("session count = %d", count)
		}
	})

	t.Run("a pending or failed attempt cannot be claimed", func(t *testing.T) {
		link(t, "provider-pending", "google-pending", false)
		token, err := domain.NewProviderHandoffCode()
		if err != nil {
			t.Fatal(err)
		}
		attempt := outbound.ProviderAttempt{
			Provider: "github", AttemptTokenVerifier: domain.ProviderSecretVerifier(key, domain.ProviderSecretAttemptToken, token),
			StateVerifier: [32]byte{90}, CodeVerifier: strings.Repeat("w", 43), CallbackURL: "https://api.example.com/v1/provider-login-callbacks/github",
		}
		if _, err := attempts.CreateProviderAttempt(ctx, attempt); err != nil {
			t.Fatal(err)
		}
		pending := providerHandoff{token, strings.Repeat("A", 43)}
		if _, err := claim(service, pending, "192.0.2.83"); !errors.Is(err, app.ErrProviderHandoffRejected) {
			t.Fatalf("pending err = %v", err)
		}
		proof, _, err := attempts.ConsumeProviderState(ctx, "github", attempt.StateVerifier)
		if err != nil {
			t.Fatal(err)
		}
		if err := attempts.FailProviderAttempt(ctx, proof.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := claim(service, pending, "192.0.2.83"); !errors.Is(err, app.ErrProviderHandoffRejected) {
			t.Fatalf("failed err = %v", err)
		}
	})

	t.Run("concurrent claims create one session", func(t *testing.T) {
		link(t, "provider-race", "google-race", false)
		handoff := seed(t, "google-race", outbound.ProviderIdentity{})
		var group sync.WaitGroup
		results := make(chan error, 10)
		for range 10 {
			group.Go(func() {
				_, err := claim(service, handoff, "192.0.2.84")
				results <- err
			})
		}
		group.Wait()
		close(results)
		created := 0
		for err := range results {
			switch {
			case err == nil:
				created++
			case !errors.Is(err, app.ErrProviderHandoffRejected):
				t.Fatal(err)
			}
		}
		if created != 1 || sessions(t, "provider-race") != 1 {
			t.Fatalf("created %d sessions", created)
		}
	})

	t.Run("failed claims share one source limit across replicas", func(t *testing.T) {
		secondPool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(secondPool.Close)
		replicas := []*app.ProviderLoginService{service, newService(secondPool)}
		var group sync.WaitGroup
		results := make(chan error, 101)
		for i := range 101 {
			group.Go(func() {
				token, err := domain.NewProviderHandoffCode()
				if err != nil {
					results <- err
					return
				}
				_, err = claim(replicas[i%2], providerHandoff{token, strings.Repeat("A", 43)}, "192.0.2.85")
				results <- err
			})
		}
		group.Wait()
		close(results)
		rejected, limited := 0, 0
		for err := range results {
			switch {
			case errors.Is(err, app.ErrProviderHandoffRejected):
				rejected++
			case errors.Is(err, app.ErrRateLimited):
				limited++
			default:
				t.Fatal(err)
			}
		}
		if rejected != 100 || limited != 1 {
			t.Fatalf("rejected %d and limited %d claims", rejected, limited)
		}
	})

	fixture := providerOnlyFixture{key: key, signer: signer, verifier: verifier, protector: protector, service: service, seed: seed}
	t.Run("new provider identities", func(t *testing.T) {
		testProviderOnlyAccounts(t, ctx, pool, fixture)
	})
	t.Run("GitHub login", func(t *testing.T) {
		testGitHubProviderLogin(t, ctx, pool, fixture)
	})
}

type providerOnlyFixture struct {
	key       []byte
	signer    outbound.TokenSigner
	verifier  *token.Verifier
	protector *deliverycrypto.DeliveryProtector
	service   *app.ProviderLoginService
	seed      func(*testing.T, string, outbound.ProviderIdentity) providerHandoff
}

type storedProviderAccount struct {
	subject, local, domain string
	passwordless, verified bool
	links, sessions        int
}

func testProviderOnlyAccounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f providerOnlyFixture) {
	claim := func(service *app.ProviderLoginService, handoff providerHandoff) (inbound.CreateProviderSessionResult, error) {
		return service.CreateProviderSession(ctx, inbound.CreateProviderSessionInput{AttemptToken: handoff.token, HandoffCode: handoff.code, Source: "192.0.2.90"})
	}
	load := func(t *testing.T, local, domain string) (storedProviderAccount, bool) {
		t.Helper()
		account := storedProviderAccount{local: local, domain: domain}
		err := pool.QueryRow(ctx, `SELECT account.subject, account.password_hash IS NULL, account.email_verified_at IS NOT NULL,
			(SELECT count(*) FROM identity_provider_links WHERE account_subject = account.subject),
			(SELECT count(*) FROM identity_sessions WHERE account_subject = account.subject)
			FROM identity_accounts AS account WHERE email_local = $1 AND email_domain = $2 AND retired_at IS NULL`, local, domain).
			Scan(&account.subject, &account.passwordless, &account.verified, &account.links, &account.sessions)
		if errors.Is(err, pgx.ErrNoRows) {
			return storedProviderAccount{}, false
		}
		if err != nil {
			t.Fatal(err)
		}
		return account, true
	}
	checkSession := func(t *testing.T, result inbound.CreateProviderSessionResult) bool {
		t.Helper()
		identity, err := f.verifier.Verify(result.AccessToken)
		if err != nil || identity.Subject != result.Subject {
			t.Fatalf("access token = %+v, %v", identity, err)
		}
		verified, err := app.NewSessionCheckService(postgres.NewSessionRepository(pool)).CheckSession(ctx, inbound.CheckSessionInput{Subject: identity.Subject, SessionID: identity.SessionID})
		if err != nil {
			t.Fatal(err)
		}
		return verified
	}

	t.Run("Gmail and Workspace identities start verified without a password", func(t *testing.T) {
		for _, test := range []struct{ providerSubject, email, hostedDomain, local, domain string }{
			{"google-gmail", "New.User@GMAIL.com", "", "New.User", "gmail.com"},
			{"google-workspace", "new@workspace.example", "workspace.example", "new", "workspace.example"},
		} {
			result, err := claim(f.service, f.seed(t, test.providerSubject, outbound.ProviderIdentity{
				Email: test.email, EmailVerified: true, HostedDomain: test.hostedDomain,
			}))
			account, found := load(t, test.local, test.domain)
			if err != nil || !found || result.Subject != account.subject || !result.EmailVerified ||
				account != (storedProviderAccount{account.subject, test.local, test.domain, true, true, 1, 1}) {
				t.Fatalf("%s result = %+v, account = %+v, %v", test.email, result.Subject, account, err)
			}
			if !checkSession(t, result) {
				t.Fatalf("%s session check is unverified", test.email)
			}
			again, err := claim(f.service, f.seed(t, test.providerSubject, outbound.ProviderIdentity{}))
			if err != nil || again.Subject != account.subject {
				t.Fatalf("%s returning login = %q, %v", test.email, again.Subject, err)
			}
		}
		login := app.NewPasswordLoginService(postgres.NewAccountRepository(pool), f.signer, func(context.Context, string, string) error { return nil })
		if _, err := login.CreatePasswordSession(ctx, inbound.CreatePasswordSessionInput{
			Email: "New.User@gmail.com", Password: "correct horse battery staple", Source: "192.0.2.90",
		}); !errors.Is(err, app.ErrInvalidCredentials) {
			t.Fatalf("password login for a provider-only account = %v", err)
		}
		allow := func(context.Context, string) error { return nil }
		recovery := app.NewPasswordResetCodeService(postgres.NewAccountRepository(pool), f.protector, allow, allow, f.key)
		if err := recovery.RequestPasswordResetCode(ctx, inbound.RequestPasswordResetCodeInput{Email: "New.User@gmail.com", Source: "192.0.2.90"}); err != nil {
			t.Fatalf("password reset code for a provider-only account = %v", err)
		}
		var resetChallenges int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenges WHERE email_local = 'New.User' AND purpose = 'password-reset'`).
			Scan(&resetChallenges); err != nil || resetChallenges != 0 {
			t.Fatalf("provider-only account received %d password reset codes: %v", resetChallenges, err)
		}
	})

	t.Run("a third-party Google email needs a Flowspace code before Workspace access", func(t *testing.T) {
		result, err := claim(f.service, f.seed(t, "google-third-party", outbound.ProviderIdentity{Email: "third@party.example", EmailVerified: true}))
		account, found := load(t, "third", "party.example")
		if err != nil || !found || result.EmailVerified || account != (storedProviderAccount{result.Subject, "third", "party.example", true, false, 1, 1}) {
			t.Fatalf("result = %+v, account = %+v, %v", result.Subject, account, err)
		}
		if checkSession(t, result) {
			t.Fatal("unverified provider account passed the session check")
		}
		var challengeID, purpose string
		var material outbound.DeliveryMaterial
		var published bool
		if err := pool.QueryRow(ctx, `SELECT challenge.id::text, challenge.purpose, delivery.key_version, delivery.nonce, delivery.ciphertext,
			event.published_at IS NOT NULL
			FROM identity_challenges AS challenge
			JOIN identity_challenge_deliveries AS delivery ON delivery.challenge_id = challenge.id
			JOIN identity_outbox_events AS event ON event.challenge_id = challenge.id
			WHERE challenge.account_subject = $1`, result.Subject).
			Scan(&challengeID, &purpose, &material.KeyVersion, &material.Nonce, &material.Ciphertext, &published); err != nil || purpose != "verify-email" || published {
			t.Fatalf("challenge purpose = %q, published %t, %v", purpose, published, err)
		}
		email, code, err := f.protector.Open(challengeID, purpose, result.Subject, material)
		if err != nil || email != "third@party.example" {
			t.Fatalf("delivery email = %q, %v", email, err)
		}
		identity, err := f.verifier.Verify(result.AccessToken)
		if err != nil {
			t.Fatal(err)
		}
		allow := func(context.Context, string) error { return nil }
		if err := app.NewEmailVerificationService(postgres.NewAccountRepository(pool), allow, allow, f.key).VerifyEmail(ctx, inbound.VerifyEmailInput{
			Subject: identity.Subject, SessionID: identity.SessionID, Source: "192.0.2.90", Code: code,
		}); err != nil {
			t.Fatal(err)
		}
		if !checkSession(t, result) {
			t.Fatal("verified provider account failed the session check")
		}
	})

	t.Run("email collisions and unusable emails fail alike without records", func(t *testing.T) {
		if _, err := identitysqlc.New(pool).CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: "collision-owner", EmailLocal: "taken", EmailDomain: "gmail.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
		identities := map[string]outbound.ProviderIdentity{
			"collision":  {Email: "taken@gmail.com", EmailVerified: true},
			"unverified": {Email: "unverified@gmail.com"},
			"missing":    {},
		}
		durations := map[string][]time.Duration{}
		for i := range 5 {
			for name, identity := range identities {
				handoff := f.seed(t, "google-"+name+"-"+strconv.Itoa(i), identity)
				started := time.Now()
				result, err := claim(f.service, handoff)
				durations[name] = append(durations[name], time.Since(started))
				if !errors.Is(err, app.ErrProviderAccountUnavailable) || result != (inbound.CreateProviderSessionResult{}) {
					t.Fatalf("%s result = %+v, %v", name, result.Subject, err)
				}
			}
		}
		for name, samples := range durations {
			slices.Sort(samples)
			collision := slices.Sorted(slices.Values(durations["collision"]))
			if samples[2] > 3*collision[2] || collision[2] > 3*samples[2] {
				t.Fatalf("%s median timing = %s, collision = %s", name, samples[2], collision[2])
			}
		}
		account, _ := load(t, "taken", "gmail.com")
		if account != (storedProviderAccount{"collision-owner", "taken", "gmail.com", false, false, 0, 0}) {
			t.Fatalf("collision changed the owner: %+v", account)
		}
		var links int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_provider_links WHERE provider_subject LIKE 'google-collision-%'
			OR provider_subject LIKE 'google-unverified-%' OR provider_subject LIKE 'google-missing-%'`).Scan(&links); err != nil || links != 0 {
			t.Fatalf("links = %d, %v", links, err)
		}
		if _, found := load(t, "unverified", "gmail.com"); found {
			t.Fatal("unverified provider email created an account")
		}
	})

	t.Run("concurrent new identities with one email create one account", func(t *testing.T) {
		handoffs := make([]providerHandoff, 10)
		for i := range handoffs {
			handoffs[i] = f.seed(t, "google-race-new-"+strconv.Itoa(i), outbound.ProviderIdentity{Email: "race@example.org", EmailVerified: true})
		}
		var group sync.WaitGroup
		results := make(chan error, len(handoffs))
		for _, handoff := range handoffs {
			group.Go(func() {
				_, err := claim(f.service, handoff)
				results <- err
			})
		}
		group.Wait()
		close(results)
		created := 0
		for err := range results {
			switch {
			case err == nil:
				created++
			case !errors.Is(err, app.ErrProviderAccountUnavailable):
				t.Fatal(err)
			}
		}
		account, found := load(t, "race", "example.org")
		if created != 1 || !found || account.links != 1 || account.sessions != 1 {
			t.Fatalf("created %d, account = %+v", created, account)
		}
	})

	t.Run("a failed delivery write rolls back the account, link, session, and claim", func(t *testing.T) {
		failing := app.NewProviderLoginService(postgres.NewProviderAttemptRepository(pool), nil, nil, f.key, nil).
			WithSessions(postgres.NewAccountRepository(pool), f.signer, failingDeliveryProtector{}, func(context.Context, string) error { return nil })
		handoff := f.seed(t, "google-rollback", outbound.ProviderIdentity{Email: "rollback@example.org", EmailVerified: true})
		if _, err := claim(failing, handoff); !errors.Is(err, app.ErrProviderLoginUnavailable) {
			t.Fatalf("err = %v", err)
		}
		if _, found := load(t, "rollback", "example.org"); found {
			t.Fatal("a failed delivery write kept the account")
		}
		result, err := claim(f.service, handoff)
		account, found := load(t, "rollback", "example.org")
		if err != nil || !found || account.subject != result.Subject || account.links != 1 || account.sessions != 1 {
			t.Fatalf("retry after rollback = %+v, %v", account, err)
		}
	})
}
