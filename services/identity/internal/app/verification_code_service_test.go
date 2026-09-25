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

type verificationRepository struct {
	tx        *verificationTransaction
	commitErr error
}

func (r verificationRepository) WithinTransaction(_ context.Context, fn func(outbound.AccountTransaction) error) error {
	if err := fn(r.tx); err != nil {
		return err
	}
	return r.commitErr
}

func (r verificationRepository) WithinVerificationTransaction(_ context.Context, fn func(outbound.VerificationTransaction) error) error {
	if err := fn(r.tx); err != nil {
		return err
	}
	return r.commitErr
}

type verificationTransaction struct {
	signupTransaction
	account        outbound.AccountState
	challenge      outbound.ChallengeState
	consumptions   int
	consumeBlocked bool
	markBlocked    bool
	markErr        error
	allowed        bool
	err            error
	steps          []string
}

func (t *verificationTransaction) GetCurrentVerificationChallenge(context.Context, string) (outbound.ChallengeState, bool, error) {
	return t.challenge, t.challenge.ID != "", nil
}

func (t *verificationTransaction) IncrementChallengeWrongGuess(context.Context, string) error {
	t.challenge.WrongGuesses++
	return nil
}

func (t *verificationTransaction) ConsumeChallenge(context.Context, string) (bool, error) {
	if t.consumeBlocked {
		return false, nil
	}
	t.consumptions++
	return true, nil
}

func (t *verificationTransaction) MarkEmailVerified(context.Context, string) (bool, error) {
	if t.markErr != nil {
		return false, t.markErr
	}
	if t.markBlocked {
		return false, nil
	}
	t.account.EmailVerified = true
	return true, nil
}

func (t *verificationTransaction) GetActiveAccountForSession(_ context.Context, subject, sessionID string) (outbound.AccountState, error) {
	t.steps = append(t.steps, "account:"+subject+":"+sessionID)
	return t.account, t.err
}

func (t *verificationTransaction) CanIssueCode(context.Context, string) (bool, error) {
	t.steps = append(t.steps, "limit")
	return t.allowed, nil
}

func (t *verificationTransaction) ReplaceVerificationChallenge(context.Context, string) error {
	t.steps = append(t.steps, "replace")
	return nil
}

func (t *verificationTransaction) CreateChallenge(_ context.Context, subject, email string, _ [32]byte) (string, error) {
	t.steps = append(t.steps, "challenge:"+subject+":"+email)
	return "challenge", nil
}

func (t *verificationTransaction) StoreDelivery(_ context.Context, _ string, _ outbound.DeliveryMaterial) error {
	t.steps = append(t.steps, "delivery")
	return nil
}

func (t *verificationTransaction) CreateOutboxEvent(_ context.Context, _ string) error {
	t.steps = append(t.steps, "outbox")
	return nil
}

func TestRequestVerificationCode(t *testing.T) {
	tx := &verificationTransaction{account: outbound.AccountState{Email: "User@example.com"}, allowed: true}
	repository := verificationRepository{tx: tx}
	sourceCalls := 0
	service := app.NewVerificationCodeService(repository, signupProtector{}, func(_ context.Context, source string) error {
		sourceCalls++
		if source != "192.0.2.1" {
			t.Fatalf("source = %q", source)
		}
		return nil
	}, nil, nil, make([]byte, 32))
	err := service.RequestEmailVerificationCode(context.Background(), inbound.RequestEmailVerificationCodeInput{
		Subject: "subject", SessionID: "session", Source: "192.0.2.1",
	})
	if err != nil || sourceCalls != 1 {
		t.Fatalf("request = %v, source calls = %d", err, sourceCalls)
	}
	want := []string{"account:subject:session", "limit", "replace", "challenge:subject:User@example.com", "delivery", "outbox"}
	if len(tx.steps) != len(want) {
		t.Fatalf("transaction steps = %v, want %v", tx.steps, want)
	}
	for i := range want {
		if tx.steps[i] != want[i] {
			t.Fatalf("transaction steps = %v, want %v", tx.steps, want)
		}
	}
	repository.commitErr = errors.New("commit failed")
	if err := app.NewVerificationCodeService(repository, signupProtector{}, func(context.Context, string) error { return nil }, nil, nil, make([]byte, 32)).RequestEmailVerificationCode(
		context.Background(), inbound.RequestEmailVerificationCodeInput{Subject: "subject", SessionID: "session", Source: "192.0.2.1"},
	); !errors.Is(err, app.ErrVerificationCodeUnavailable) {
		t.Fatalf("commit failure = %v", err)
	}
}

func TestRequestVerificationCodeRejectsBeforeReplacement(t *testing.T) {
	for _, test := range []struct {
		name       string
		account    outbound.AccountState
		accountErr error
		allowed    bool
		limitErr   error
		want       error
	}{
		{"verified", outbound.AccountState{Email: "User@example.com", EmailVerified: true}, nil, true, nil, app.ErrEmailAlreadyVerified},
		{"missing session", outbound.AccountState{}, outbound.ErrUnauthenticated, true, nil, outbound.ErrUnauthenticated},
		{"cooldown or hourly limit", outbound.AccountState{Email: "User@example.com"}, nil, false, nil, app.ErrRateLimited},
		{"source limit", outbound.AccountState{Email: "User@example.com"}, nil, true, app.ErrRateLimited, app.ErrRateLimited},
		{"canceled transaction", outbound.AccountState{}, context.Canceled, true, nil, context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := &verificationTransaction{account: test.account, err: test.accountErr, allowed: test.allowed}
			service := app.NewVerificationCodeService(verificationRepository{tx: tx}, signupProtector{},
				func(context.Context, string) error { return test.limitErr }, nil, nil, make([]byte, 32))
			err := service.RequestEmailVerificationCode(context.Background(), inbound.RequestEmailVerificationCodeInput{
				Subject: "subject", SessionID: "session", Source: "192.0.2.1",
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			for _, step := range tx.steps {
				if step == "replace" || step == "outbox" {
					t.Fatalf("rejected request ran %q", step)
				}
			}
		})
	}
}

func TestVerifyEmailConsumesCurrentCodeAndReturnsVerifiedOnRetry(t *testing.T) {
	key := make([]byte, 32)
	code, verifier, expires, err := domain.NewChallenge(key, "subject", "User@example.com", domain.PurposeVerifyEmail, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tx := &verificationTransaction{account: outbound.AccountState{Email: "User@example.com"}, challenge: outbound.ChallengeState{
		ID: "challenge", Email: "User@example.com", Verifier: verifier, ExpiresAt: expires,
	}}
	service := app.NewVerificationCodeService(verificationRepository{tx: tx}, signupProtector{},
		func(context.Context, string) error { return nil }, func(context.Context, string) error { return nil },
		func(context.Context, string) error { return nil }, key)
	input := inbound.VerifyEmailInput{Subject: "subject", SessionID: "session", Source: "192.0.2.1", Code: code}
	if err := service.VerifyEmail(context.Background(), input); err != nil {
		t.Fatalf("verify = %v", err)
	}
	if !tx.account.EmailVerified || tx.consumptions != 1 {
		t.Fatalf("verified = %v, consumptions = %d", tx.account.EmailVerified, tx.consumptions)
	}
	if err := service.VerifyEmail(context.Background(), input); err != nil || tx.consumptions != 1 {
		t.Fatalf("retry = %v, consumptions = %d", err, tx.consumptions)
	}
}

func TestVerifyEmailAppliesSharedGuessLimitsBeforeChangingState(t *testing.T) {
	for _, blocked := range []string{"source", "account"} {
		t.Run(blocked, func(t *testing.T) {
			tx := &verificationTransaction{account: outbound.AccountState{Email: "User@example.com"}}
			calls := []string{}
			limit := func(scope string) func(context.Context, string) error {
				return func(context.Context, string) error {
					calls = append(calls, scope)
					if scope == blocked {
						return app.ErrRateLimited
					}
					return nil
				}
			}
			service := app.NewVerificationCodeService(verificationRepository{tx: tx}, nil, nil,
				limit("source"), limit("account"), make([]byte, 32))
			err := service.VerifyEmail(context.Background(), inbound.VerifyEmailInput{
				Subject: "subject", SessionID: "session", Source: "192.0.2.1", Code: "123456",
			})
			if !errors.Is(err, app.ErrRateLimited) || len(tx.steps) != 0 || tx.consumptions != 0 || len(calls) == 0 || calls[0] != "source" {
				t.Fatalf("error = %v, steps = %v, consumptions = %d, limits = %v", err, tx.steps, tx.consumptions, calls)
			}
			if blocked == "account" && (len(calls) != 2 || calls[1] != "account") {
				t.Fatalf("limits = %v", calls)
			}
		})
	}
}

func TestVerifyEmailRejectsInvalidCodesAndFailedTransitions(t *testing.T) {
	key := make([]byte, 32)
	code, verifier, expires, err := domain.NewChallenge(key, "subject", "User@example.com", domain.PurposeVerifyEmail, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name           string
		code           string
		accountErr     error
		consumeBlocked bool
		markBlocked    bool
		markErr        error
		want           error
	}{
		{"short code", "12345", nil, false, false, nil, app.ErrInvalidVerificationCode},
		{"non ASCII code", "１２３４５６", nil, false, false, nil, app.ErrInvalidVerificationCode},
		{"revoked session", code, outbound.ErrUnauthenticated, false, false, nil, outbound.ErrUnauthenticated},
		{"concurrent consumption", code, nil, true, false, nil, app.ErrInvalidVerificationCode},
		{"mark failed", code, nil, false, false, errors.New("database failed"), app.ErrVerificationCodeUnavailable},
		{"mark changed no rows", code, nil, false, true, nil, app.ErrVerificationCodeUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := &verificationTransaction{
				account: outbound.AccountState{Email: "User@example.com"}, err: test.accountErr,
				challenge:      outbound.ChallengeState{ID: "challenge", Email: "User@example.com", Verifier: verifier, ExpiresAt: expires},
				consumeBlocked: test.consumeBlocked, markBlocked: test.markBlocked, markErr: test.markErr,
			}
			service := app.NewVerificationCodeService(verificationRepository{tx: tx}, nil, nil,
				func(context.Context, string) error { return nil }, func(context.Context, string) error { return nil }, key)
			if err := service.VerifyEmail(context.Background(), inbound.VerifyEmailInput{
				Subject: "subject", SessionID: "session", Source: "192.0.2.1", Code: test.code,
			}); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
