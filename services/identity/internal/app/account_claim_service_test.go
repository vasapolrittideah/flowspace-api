package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type accountClaimRepository struct {
	account   outbound.ClaimAccount
	found     bool
	tx        *accountClaimTransaction
	lookupErr error
	commitErr error
}

func (r *accountClaimRepository) FindAccountForClaim(context.Context, string) (outbound.ClaimAccount, bool, error) {
	return r.account, r.found, r.lookupErr
}

func (r *accountClaimRepository) WithinAccountClaimTransaction(_ context.Context, fn func(outbound.AccountClaimTransaction) error) error {
	if err := fn(r.tx); err != nil {
		return err
	}
	return r.commitErr
}

type accountClaimTransaction struct {
	account      outbound.ClaimAccount
	found        bool
	challenge    outbound.ChallengeState
	hasChallenge bool
	wrongGuesses int
	consumed     bool
	retired      bool
	revoked      bool
	created      string
	verified     bool
	newSession   bool
}

func (t *accountClaimTransaction) GetAccountForClaim(context.Context, string) (outbound.ClaimAccount, bool, error) {
	return t.account, t.found, nil
}

func (t *accountClaimTransaction) GetCurrentClaimChallenge(context.Context, string) (outbound.ChallengeState, bool, error) {
	return t.challenge, t.hasChallenge, nil
}

func (t *accountClaimTransaction) IncrementChallengeWrongGuess(context.Context, string) error {
	t.wrongGuesses++
	return nil
}

func (t *accountClaimTransaction) ConsumeChallenge(context.Context, string) (bool, error) {
	if t.consumed {
		return false, nil
	}
	t.consumed = true
	return true, nil
}

func (t *accountClaimTransaction) RetireAndRevokeAccount(context.Context, string) (bool, error) {
	t.revoked = true
	t.retired = true
	return true, nil
}

func (t *accountClaimTransaction) CreateAccount(_ context.Context, subject, _, _ string) error {
	t.created = subject
	return nil
}

func (t *accountClaimTransaction) MarkEmailVerified(context.Context, string) (bool, error) {
	t.verified = true
	return true, nil
}

func (t *accountClaimTransaction) Create(_ context.Context, _ string, _ []byte) (outbound.SessionRecord, error) {
	t.newSession = true
	now := time.Now()
	return outbound.SessionRecord{ID: "session", CreatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(24 * time.Hour)}, nil
}

type claimTokenSigner struct{}

func (claimTokenSigner) Sign(claims outbound.AccessTokenClaims) (string, error) {
	return "access:" + claims.Subject, nil
}

func newAccountClaimService(repo *accountClaimRepository) *app.AccountClaimService {
	return app.NewAccountClaimService(repo, claimTokenSigner{},
		func(context.Context, string) error { return nil }, func(context.Context, string) error { return nil },
		func(context.Context, string) (bool, error) { return false, nil }, make([]byte, 32))
}

func TestAccountClaimReplacesUnverifiedSubject(t *testing.T) {
	key := make([]byte, 32)
	code, verifier, expiry, err := domain.NewChallenge(key, "old-subject", "User@example.com", domain.PurposeClaimAccount, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tx := &accountClaimTransaction{
		account: outbound.ClaimAccount{Subject: "old-subject"}, found: true,
		challenge: outbound.ChallengeState{ID: "challenge", Email: "User@example.com", Verifier: verifier, ExpiresAt: expiry}, hasChallenge: true,
	}
	repo := &accountClaimRepository{account: tx.account, found: true, tx: tx}
	input := inbound.ClaimAccountInput{Email: "User@EXAMPLE.COM", Code: code, NewPassword: "correct horse battery staple", Source: "192.0.2.1"}
	result, err := newAccountClaimService(repo).ClaimUnverifiedAccount(context.Background(), input)
	if err != nil || result.Subject == "" || result.Subject == "old-subject" || result.AccessToken != "access:"+result.Subject ||
		result.RefreshToken == "" || !tx.consumed || !tx.revoked || !tx.retired || tx.created != result.Subject || !tx.verified || !tx.newSession {
		t.Fatalf("result = %+v, transaction = %+v, error = %v", result, tx, err)
	}
	if _, err := newAccountClaimService(repo).ClaimUnverifiedAccount(context.Background(), input); !errors.Is(err, app.ErrInvalidClaimCode) {
		t.Fatalf("replayed claim = %v", err)
	}
}

func TestAccountClaimRejectsInvalidAndIneligibleCodes(t *testing.T) {
	key := make([]byte, 32)
	_, verificationVerifier, expiry, err := domain.NewChallenge(key, "old-subject", "User@example.com", domain.PurposeVerifyEmail, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		found     bool
		verified  bool
		challenge bool
		expired   bool
	}{
		{name: "missing"},
		{name: "verified", found: true, verified: true},
		{name: "no challenge", found: true},
		{name: "wrong purpose", found: true, challenge: true},
		{name: "expired", found: true, challenge: true, expired: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			challengeExpiry := expiry
			if test.expired {
				challengeExpiry = time.Now().Add(-time.Second)
			}
			tx := &accountClaimTransaction{
				account: outbound.ClaimAccount{Subject: "old-subject", EmailVerified: test.verified}, found: test.found,
				challenge: outbound.ChallengeState{ID: "challenge", Email: "User@example.com", Verifier: verificationVerifier, ExpiresAt: challengeExpiry}, hasChallenge: test.challenge,
			}
			repo := &accountClaimRepository{account: tx.account, found: test.found, tx: tx}
			result, err := newAccountClaimService(repo).ClaimUnverifiedAccount(context.Background(), inbound.ClaimAccountInput{
				Email: "User@example.com", Code: "012345", NewPassword: "correct horse battery staple", Source: "192.0.2.1",
			})
			if !errors.Is(err, app.ErrInvalidClaimCode) || result.Subject != "" || tx.created != "" || tx.retired {
				t.Fatalf("result = %+v, transaction = %+v, error = %v", result, tx, err)
			}
		})
	}
}

func TestAccountClaimValidatesInputAndLimitsBeforeChangingAccounts(t *testing.T) {
	input := inbound.ClaimAccountInput{Email: "User@example.com", Code: "012345", NewPassword: "correct horse battery staple", Source: "192.0.2.1"}
	for _, test := range []struct {
		name         string
		input        inbound.ClaimAccountInput
		sourceLimit  error
		accountLimit error
		lookupErr    error
		want         error
	}{
		{name: "source limit", input: input, sourceLimit: app.ErrRateLimited, want: app.ErrRateLimited},
		{name: "invalid email", input: inbound.ClaimAccountInput{Email: "invalid", NewPassword: input.NewPassword, Source: input.Source}, want: domain.ErrInvalidEmail},
		{name: "invalid password", input: inbound.ClaimAccountInput{Email: input.Email, NewPassword: "short", Source: input.Source}, want: domain.ErrInvalidPassword},
		{name: "lookup failure", input: input, lookupErr: errors.New("database failed"), want: app.ErrClaimUnavailable},
		{name: "account limit", input: input, accountLimit: app.ErrRateLimited, want: app.ErrRateLimited},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &accountClaimRepository{account: outbound.ClaimAccount{Subject: "old-subject"}, found: true, lookupErr: test.lookupErr}
			service := app.NewAccountClaimService(repo, claimTokenSigner{},
				func(context.Context, string) error { return test.sourceLimit },
				func(context.Context, string) error { return test.accountLimit },
				func(context.Context, string) (bool, error) { return false, nil }, make([]byte, 32))
			result, err := service.ClaimUnverifiedAccount(context.Background(), test.input)
			if !errors.Is(err, test.want) || result.AccessToken != "" || result.RefreshToken != "" {
				t.Fatalf("result = %+v, error = %v, want %v", result, err, test.want)
			}
		})
	}
}
