package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
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

type verificationTransaction struct {
	signupTransaction
	account outbound.AccountState
	allowed bool
	err     error
	steps   []string
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
	}, make([]byte, 32))
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
	if err := app.NewVerificationCodeService(repository, signupProtector{}, func(context.Context, string) error { return nil }, make([]byte, 32)).RequestEmailVerificationCode(
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
				func(context.Context, string) error { return test.limitErr }, make([]byte, 32))
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
