package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	input "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	output "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type signupRepository struct {
	commitError error
	transaction *signupTransaction
}

func (r *signupRepository) WithinTransaction(_ context.Context, fn func(output.AccountTransaction) error) error {
	if err := fn(r.transaction); err != nil {
		return err
	}
	return r.commitError
}

type signupTransaction struct{ steps []string }

func (t *signupTransaction) GetActiveAccountForSession(context.Context, string, string) (output.AccountState, error) {
	return output.AccountState{}, errors.New("unexpected account lookup")
}

func (t *signupTransaction) CanIssueCode(context.Context, string) (bool, error) {
	return false, errors.New("unexpected code limit check")
}

func (t *signupTransaction) ReplaceVerificationChallenge(context.Context, string) error {
	return errors.New("unexpected challenge replacement")
}

func (t *signupTransaction) CreateAccount(_ context.Context, _, _, _ string) error {
	t.steps = append(t.steps, "account")
	return nil
}

func (t *signupTransaction) Create(_ context.Context, _ string, _ []byte) (output.SessionRecord, error) {
	t.steps = append(t.steps, "session")
	now := time.Now()
	return output.SessionRecord{ID: "session", CreatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(time.Hour)}, nil
}

func (t *signupTransaction) CreateChallenge(_ context.Context, _, _ string, _ [32]byte) (string, error) {
	t.steps = append(t.steps, "challenge")
	return "challenge", nil
}

func (t *signupTransaction) StoreDelivery(_ context.Context, _ string, _ output.DeliveryMaterial) error {
	t.steps = append(t.steps, "delivery")
	return nil
}

func (t *signupTransaction) CreateOutboxEvent(_ context.Context, _ string) error {
	t.steps = append(t.steps, "outbox")
	return nil
}

type signupSigner struct{}

func (signupSigner) Sign(output.AccessTokenClaims) (string, error) { return "access", nil }

type signupProtector struct{}

func (signupProtector) Protect(_, _, _, _, _ string) (output.DeliveryMaterial, error) {
	return output.DeliveryMaterial{KeyVersion: 1, Nonce: make([]byte, 12), Ciphertext: make([]byte, 17)}, nil
}

func TestSignupCommitsBeforeReturningTokens(t *testing.T) {
	for _, test := range []struct {
		name      string
		commitErr error
		wantToken bool
	}{
		{name: "commit succeeds", wantToken: true},
		{name: "commit fails", commitErr: errors.New("commit failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := &signupTransaction{}
			repository := &signupRepository{transaction: tx, commitError: test.commitErr}
			service := app.NewSignupService(repository, signupSigner{}, signupProtector{},
				func(context.Context, string) error { return nil },
				func(context.Context, string) (bool, error) { return false, nil }, make([]byte, 32))
			result, err := service.CreateAccount(context.Background(), input.CreateAccountInput{
				Email: "User@EXAMPLE.COM", Password: "correct horse battery staple", Source: "192.0.2.1",
			})
			if (err == nil) != test.wantToken || (result.AccessToken != "") != test.wantToken {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			if len(tx.steps) != 5 {
				t.Fatalf("transaction steps = %v", tx.steps)
			}
		})
	}
}

func TestSignupRejectsBeforeStartingTransaction(t *testing.T) {
	limitFailure := errors.New("rate limit unavailable")
	for _, test := range []struct {
		name     string
		email    string
		password string
		limitErr error
		checkErr error
		want     error
	}{
		{"limit failure", "User@example.com", "correct horse battery staple", limitFailure, nil, limitFailure},
		{"invalid email", "invalid", "correct horse battery staple", nil, nil, domain.ErrInvalidEmail},
		{"short password", "User@example.com", "short", nil, nil, domain.ErrInvalidPassword},
		{"breach service unavailable", "User@example.com", "correct horse battery staple", nil, errors.New("offline"), domain.ErrPasswordCheckUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := &signupTransaction{}
			service := app.NewSignupService(&signupRepository{transaction: tx}, signupSigner{}, signupProtector{},
				func(context.Context, string) error { return test.limitErr },
				func(context.Context, string) (bool, error) { return false, test.checkErr }, make([]byte, 32))
			result, err := service.CreateAccount(context.Background(), input.CreateAccountInput{
				Email: test.email, Password: test.password, Source: "192.0.2.1",
			})
			if !errors.Is(err, test.want) || result.AccessToken != "" || len(tx.steps) != 0 {
				t.Fatalf("result = %+v, steps = %v, error = %v", result, tx.steps, err)
			}
		})
	}
}
