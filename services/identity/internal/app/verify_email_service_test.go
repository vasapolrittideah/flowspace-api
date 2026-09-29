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

func TestVerifyEmailConsumesCurrentCodeAndReturnsVerifiedOnRetry(t *testing.T) {
	key := make([]byte, 32)
	code, verifier, expires, err := domain.NewChallenge(key, "subject", "User@example.com", domain.PurposeVerifyEmail, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tx := &verificationTransaction{account: outbound.AccountState{Email: "User@example.com"}, challenge: outbound.ChallengeState{
		ID: "challenge", Email: "User@example.com", Verifier: verifier, ExpiresAt: expires,
	}}
	service := app.NewVerifyEmailService(verificationRepository{tx: tx},
		func(context.Context, string) error { return nil }, func(context.Context, string) error { return nil }, key)
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
			service := app.NewVerifyEmailService(verificationRepository{tx: tx},
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
			service := app.NewVerifyEmailService(verificationRepository{tx: tx},
				func(context.Context, string) error { return nil }, func(context.Context, string) error { return nil }, key)
			if err := service.VerifyEmail(context.Background(), inbound.VerifyEmailInput{
				Subject: "subject", SessionID: "session", Source: "192.0.2.1", Code: test.code,
			}); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
