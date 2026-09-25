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

type claimRepository struct {
	tx        *claimTransaction
	commitErr error
}

func (r claimRepository) WithinClaimCodeTransaction(_ context.Context, fn func(outbound.ClaimCodeTransaction) error) error {
	if err := fn(r.tx); err != nil {
		return err
	}
	return r.commitErr
}

type claimTransaction struct {
	account outbound.ClaimAccount
	found   bool
	allowed bool
	steps   []string
}

func (t *claimTransaction) GetAccountForClaim(_ context.Context, email string) (outbound.ClaimAccount, bool, error) {
	t.steps = append(t.steps, "lookup:"+email)
	return t.account, t.found, nil
}

func (t *claimTransaction) CanIssueCode(context.Context, string) (bool, error) {
	t.steps = append(t.steps, "limit")
	return t.allowed, nil
}

func (t *claimTransaction) ReplaceClaimChallenge(context.Context, string) error {
	t.steps = append(t.steps, "replace")
	return nil
}

func (t *claimTransaction) CreateClaimChallenge(_ context.Context, subject, email string, _ [32]byte) (string, error) {
	t.steps = append(t.steps, "challenge:"+subject+":"+email)
	return "challenge", nil
}

func (t *claimTransaction) StoreDelivery(context.Context, string, outbound.DeliveryMaterial) error {
	t.steps = append(t.steps, "delivery")
	return nil
}

func (t *claimTransaction) CreateOutboxEvent(context.Context, string) error {
	t.steps = append(t.steps, "outbox")
	return nil
}

type claimProtector struct {
	purpose string
	subject string
	email   string
	code    string
}

func (p *claimProtector) Protect(_ string, purpose, subject, email, code string) (outbound.DeliveryMaterial, error) {
	p.purpose, p.subject, p.email, p.code = purpose, subject, email, code
	return outbound.DeliveryMaterial{KeyVersion: 1, Nonce: make([]byte, 12), Ciphertext: make([]byte, 17)}, nil
}

func TestRequestClaimCodeCreatesPurposeBoundDelivery(t *testing.T) {
	tx := &claimTransaction{account: outbound.ClaimAccount{Subject: "subject"}, found: true, allowed: true}
	protector := &claimProtector{}
	service := app.NewClaimCodeService(claimRepository{tx: tx}, protector, func(_ context.Context, source string) error {
		if source != "192.0.2.1" {
			t.Fatalf("source = %q", source)
		}
		return nil
	}, make([]byte, 32))
	err := service.RequestUnverifiedAccountClaimCode(context.Background(), inbound.RequestClaimCodeInput{
		Email: "User@EXAMPLE.COM", Source: "192.0.2.1",
	})
	if err != nil || protector.purpose != string(domain.PurposeClaimAccount) || protector.subject != "subject" ||
		protector.email != "User@example.com" || len(protector.code) != 6 {
		t.Fatalf("error = %v, delivery = %+v", err, protector)
	}
	want := []string{"lookup:User@example.com", "limit", "replace", "challenge:subject:User@example.com", "delivery", "outbox"}
	if len(tx.steps) != len(want) {
		t.Fatalf("steps = %v, want %v", tx.steps, want)
	}
	for i := range want {
		if tx.steps[i] != want[i] {
			t.Fatalf("steps = %v, want %v", tx.steps, want)
		}
	}
}

func TestRequestClaimCodeKeepsIneligibleResponsesGeneric(t *testing.T) {
	for _, test := range []struct {
		name    string
		account outbound.ClaimAccount
		found   bool
		allowed bool
	}{
		{"missing", outbound.ClaimAccount{}, false, false},
		{"verified", outbound.ClaimAccount{Subject: "subject", EmailVerified: true}, true, true},
		{"account limit", outbound.ClaimAccount{Subject: "subject"}, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := &claimTransaction{account: test.account, found: test.found, allowed: test.allowed}
			service := app.NewClaimCodeService(claimRepository{tx: tx}, &claimProtector{},
				func(context.Context, string) error { return nil }, make([]byte, 32))
			started := time.Now()
			if err := service.RequestUnverifiedAccountClaimCode(context.Background(), inbound.RequestClaimCodeInput{
				Email: "User@example.com", Source: "192.0.2.1",
			}); err != nil || time.Since(started) < 100*time.Millisecond {
				t.Fatalf("generic request = %v, duration = %s", err, time.Since(started))
			}
			for _, step := range tx.steps {
				if step == "replace" || step == "outbox" {
					t.Fatalf("ineligible request ran %q", step)
				}
			}
		})
	}
}

func TestRequestClaimCodeRejectsInvalidInputAndFailures(t *testing.T) {
	tx := &claimTransaction{account: outbound.ClaimAccount{Subject: "subject"}, found: true, allowed: true}
	input := inbound.RequestClaimCodeInput{Email: "User@example.com", Source: "192.0.2.1"}
	service := app.NewClaimCodeService(claimRepository{tx: tx}, &claimProtector{},
		func(context.Context, string) error { return app.ErrRateLimited }, make([]byte, 32))
	if err := service.RequestUnverifiedAccountClaimCode(context.Background(), input); !errors.Is(err, app.ErrRateLimited) || len(tx.steps) != 0 {
		t.Fatalf("source limit = %v, steps = %v", err, tx.steps)
	}
	service = app.NewClaimCodeService(claimRepository{tx: tx}, &claimProtector{},
		func(context.Context, string) error { return nil }, make([]byte, 32))
	input.Email = "invalid"
	if err := service.RequestUnverifiedAccountClaimCode(context.Background(), input); !errors.Is(err, domain.ErrInvalidEmail) {
		t.Fatalf("invalid email = %v", err)
	}
	input.Email = "User@example.com"
	service = app.NewClaimCodeService(claimRepository{tx: tx, commitErr: errors.New("commit failed")}, &claimProtector{},
		func(context.Context, string) error { return nil }, make([]byte, 32))
	if err := service.RequestUnverifiedAccountClaimCode(context.Background(), input); !errors.Is(err, app.ErrClaimCodeUnavailable) {
		t.Fatalf("commit failure = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service = app.NewClaimCodeService(claimRepository{tx: &claimTransaction{}}, &claimProtector{},
		func(context.Context, string) error { return nil }, make([]byte, 32))
	started := time.Now()
	if err := service.RequestUnverifiedAccountClaimCode(ctx, input); !errors.Is(err, context.Canceled) || time.Since(started) >= 100*time.Millisecond {
		t.Fatalf("canceled request = %v, duration = %s", err, time.Since(started))
	}
}
