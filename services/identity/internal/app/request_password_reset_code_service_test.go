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

type recoveryRequestRepository struct {
	tx  *recoveryRequestTransaction
	err error
}

func (r recoveryRequestRepository) WithinRequestPasswordResetCodeTransaction(_ context.Context, fn func(outbound.PasswordResetCodeTransaction) error) error {
	if err := fn(r.tx); err != nil {
		return err
	}
	return r.err
}

type recoveryRequestTransaction struct {
	account        outbound.PasswordRecoveryAccount
	found, allowed bool
	steps          []string
}

func (t *recoveryRequestTransaction) GetAccountForPasswordRecovery(_ context.Context, email string) (outbound.PasswordRecoveryAccount, bool, error) {
	t.steps = append(t.steps, "lookup:"+email)
	return t.account, t.found, nil
}

func (t *recoveryRequestTransaction) CanIssueCode(context.Context, string) (bool, error) {
	t.steps = append(t.steps, "can-issue")
	return t.allowed, nil
}

func (t *recoveryRequestTransaction) ReplacePasswordResetChallenge(context.Context, string) error {
	t.steps = append(t.steps, "replace")
	return nil
}

func (t *recoveryRequestTransaction) CreatePasswordResetChallenge(context.Context, string, string, [32]byte) (string, error) {
	t.steps = append(t.steps, "create")
	return "challenge", nil
}

func (t *recoveryRequestTransaction) StoreDelivery(context.Context, string, outbound.DeliveryMaterial) error {
	t.steps = append(t.steps, "delivery")
	return nil
}

func (t *recoveryRequestTransaction) CreateOutboxEvent(context.Context, string) error {
	t.steps = append(t.steps, "outbox")
	return nil
}

func TestRequestPasswordResetCodeQueuesOnlyEligibleAccount(t *testing.T) {
	for _, test := range []struct {
		name                   string
		account                outbound.PasswordRecoveryAccount
		found, allowed, queued bool
	}{
		{"eligible", outbound.PasswordRecoveryAccount{Subject: "subject", EmailVerified: true, HasPassword: true}, true, true, true},
		{"missing", outbound.PasswordRecoveryAccount{}, false, true, false},
		{"unverified", outbound.PasswordRecoveryAccount{Subject: "subject", HasPassword: true}, true, true, false},
		{"provider only", outbound.PasswordRecoveryAccount{Subject: "subject", EmailVerified: true}, true, true, false},
		{"cooldown", outbound.PasswordRecoveryAccount{Subject: "subject", EmailVerified: true, HasPassword: true}, true, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := &recoveryRequestTransaction{account: test.account, found: test.found, allowed: test.allowed}
			protector := &claimProtector{}
			sourceCalls, emailCalls := 0, 0
			service := app.NewPasswordResetCodeService(recoveryRequestRepository{tx: tx}, protector,
				func(_ context.Context, source string) error {
					sourceCalls++
					if source != "192.0.2.1" {
						t.Fatal(source)
					}
					return nil
				},
				func(_ context.Context, email string) error {
					emailCalls++
					if email != "User@example.com" {
						t.Fatal(email)
					}
					return nil
				},
				make([]byte, 32))
			started := time.Now()
			err := service.RequestPasswordResetCode(context.Background(), inbound.RequestPasswordResetCodeInput{Email: "User@EXAMPLE.COM", Source: "192.0.2.1"})
			if err != nil || sourceCalls != 1 || emailCalls != 1 || time.Since(started) < 100*time.Millisecond {
				t.Fatalf("response=%v source=%d email=%d duration=%s", err, sourceCalls, emailCalls, time.Since(started))
			}
			queued := len(tx.steps) > 0 && tx.steps[len(tx.steps)-1] == "outbox"
			if queued != test.queued {
				t.Fatalf("steps=%v", tx.steps)
			}
			if queued && (protector.purpose != string(domain.PurposePasswordReset) || protector.email != "User@example.com" || len(protector.code) != 6) {
				t.Fatalf("delivery=%+v", protector)
			}
		})
	}
}

func TestRequestPasswordResetCodeFailsClosed(t *testing.T) {
	tx := &recoveryRequestTransaction{account: outbound.PasswordRecoveryAccount{Subject: "subject", EmailVerified: true, HasPassword: true}, found: true, allowed: true}
	input := inbound.RequestPasswordResetCodeInput{Email: "User@example.com", Source: "192.0.2.1"}
	service := app.NewPasswordResetCodeService(recoveryRequestRepository{tx: tx}, &claimProtector{},
		func(context.Context, string) error { return app.ErrRateLimited }, func(context.Context, string) error { return nil }, make([]byte, 32))
	if err := service.RequestPasswordResetCode(context.Background(), input); !errors.Is(err, app.ErrRateLimited) || len(tx.steps) != 0 {
		t.Fatalf("source limit=%v steps=%v", err, tx.steps)
	}
	service = app.NewPasswordResetCodeService(recoveryRequestRepository{tx: tx}, &claimProtector{},
		func(context.Context, string) error { return nil }, func(context.Context, string) error { return app.ErrLimitUnavailable }, make([]byte, 32))
	if err := service.RequestPasswordResetCode(context.Background(), input); !errors.Is(err, app.ErrLimitUnavailable) || len(tx.steps) != 0 {
		t.Fatalf("email limit=%v steps=%v", err, tx.steps)
	}
	service = app.NewPasswordResetCodeService(recoveryRequestRepository{tx: tx, err: errors.New("commit failed")}, &claimProtector{},
		func(context.Context, string) error { return nil }, func(context.Context, string) error { return nil }, make([]byte, 32))
	if err := service.RequestPasswordResetCode(context.Background(), input); !errors.Is(err, app.ErrPasswordResetCodeUnavailable) {
		t.Fatalf("transaction=%v", err)
	}
	input.Email = "invalid"
	if err := service.RequestPasswordResetCode(context.Background(), input); !errors.Is(err, domain.ErrInvalidEmail) {
		t.Fatalf("email=%v", err)
	}
	input.Email = "User@example.com"
	service = app.NewPasswordResetCodeService(nil, &claimProtector{},
		func(context.Context, string) error { return nil }, func(context.Context, string) error { return nil }, make([]byte, 32))
	if err := service.RequestPasswordResetCode(context.Background(), input); !errors.Is(err, app.ErrPasswordResetCodeUnavailable) {
		t.Fatalf("missing repository=%v", err)
	}
	service = app.NewPasswordResetCodeService(recoveryRequestRepository{tx: tx, err: context.Canceled}, &claimProtector{},
		func(context.Context, string) error { return nil }, func(context.Context, string) error { return nil }, make([]byte, 32))
	if err := service.RequestPasswordResetCode(context.Background(), input); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled transaction=%v", err)
	}
	service = app.NewPasswordResetCodeService(recoveryRequestRepository{tx: tx}, &claimProtector{},
		func(context.Context, string) error { return nil }, func(context.Context, string) error { return nil }, make([]byte, 32))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.RequestPasswordResetCode(ctx, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled response=%v", err)
	}
}
