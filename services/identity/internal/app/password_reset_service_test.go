package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type resetPreflightRepository struct {
	account      outbound.PasswordRecoveryAccount
	found        bool
	reads        int
	findErr      error
	getErr       error
	challengeErr error
}

func (r *resetPreflightRepository) FindPasswordAccount(context.Context, string) (outbound.PasswordAccount, bool, error) {
	r.reads++
	return outbound.PasswordAccount{Subject: r.account.Subject, EmailVerified: r.account.EmailVerified}, r.found, r.findErr
}

func (r *resetPreflightRepository) WithinPasswordResetTransaction(_ context.Context, fn func(outbound.PasswordResetTransaction) error) error {
	return fn(resetPreflightTransaction{account: r.account, found: r.found, getErr: r.getErr, challengeErr: r.challengeErr})
}

type resetPreflightTransaction struct {
	account      outbound.PasswordRecoveryAccount
	found        bool
	getErr       error
	challengeErr error
}

func (t resetPreflightTransaction) GetAccountForPasswordRecovery(context.Context, string) (outbound.PasswordRecoveryAccount, bool, error) {
	return t.account, t.found, t.getErr
}

func (t resetPreflightTransaction) GetCurrentPasswordResetChallenge(context.Context, string) (outbound.ChallengeState, bool, error) {
	return outbound.ChallengeState{}, false, t.challengeErr
}

func (resetPreflightTransaction) IncrementChallengeWrongGuess(context.Context, string) error {
	return nil
}

func (resetPreflightTransaction) ConsumeChallenge(context.Context, string) (bool, error) {
	return false, nil
}

func (resetPreflightTransaction) ReplacePasswordResetChallenge(context.Context, string) error {
	return nil
}

func (resetPreflightTransaction) DeleteChallengeDelivery(context.Context, string) error { return nil }

func (resetPreflightTransaction) UpdatePasswordHash(context.Context, string, string) error {
	return nil
}

func (resetPreflightTransaction) RevokePasswordSessions(context.Context, string) error { return nil }

func (resetPreflightTransaction) QueuePasswordChangeNotice(context.Context, string) error { return nil }

func TestPasswordResetValidatesBeforeExpensiveWorkAndHidesEligibility(t *testing.T) {
	for _, test := range []struct {
		name    string
		account outbound.PasswordRecoveryAccount
		found   bool
	}{
		{"missing", outbound.PasswordRecoveryAccount{}, false},
		{"unverified", outbound.PasswordRecoveryAccount{Subject: "subject", HasPassword: true}, true},
		{"provider only", outbound.PasswordRecoveryAccount{Subject: "subject", EmailVerified: true}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &resetPreflightRepository{account: test.account, found: test.found}
			limits := 0
			limit := func(context.Context, string) error { limits++; return nil }
			service := app.NewPasswordResetService(repository, limit, limit, limit,
				func(context.Context, string) (bool, error) {
					t.Fatal("password hashing ran without proof")
					return false, nil
				}, make([]byte, 32))
			input := inbound.ResetPasswordInput{Email: "User@example.com", Code: "123456", NewPassword: "fresh password 123", Source: "192.0.2.1"}
			if err := service.ResetPassword(context.Background(), input); !errors.Is(err, app.ErrInvalidPasswordResetCode) {
				t.Fatalf("invalid code = %v", err)
			}
			if repository.reads != 1 || limits != 2+map[bool]int{true: 1, false: 0}[test.found] {
				t.Fatalf("reads=%d limits=%d", repository.reads, limits)
			}
			input.NewPassword = "short"
			if err := service.ResetPassword(context.Background(), input); !errors.Is(err, domain.ErrInvalidPassword) || repository.reads != 1 {
				t.Fatalf("password policy = %v, reads=%d", err, repository.reads)
			}
			input.NewPassword = "fresh password 123"
			input.Code = "１２３４５６"
			if err := service.ResetPassword(context.Background(), input); !errors.Is(err, app.ErrInvalidPasswordResetCode) || repository.reads != 1 {
				t.Fatalf("non-ASCII code = %v, reads=%d", err, repository.reads)
			}
		})
	}
	service := app.NewPasswordResetService(&resetPreflightRepository{},
		func(context.Context, string) error { return context.Canceled },
		func(context.Context, string) error { t.Fatal("email limit ran after cancellation"); return nil },
		func(context.Context, string) error { t.Fatal("account limit ran after cancellation"); return nil },
		func(context.Context, string) (bool, error) { t.Fatal("hash ran after cancellation"); return false, nil }, make([]byte, 32))
	if err := service.ResetPassword(context.Background(), inbound.ResetPasswordInput{Source: "192.0.2.1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestPasswordResetFailsClosedWhenStateIsUnavailable(t *testing.T) {
	input := inbound.ResetPasswordInput{Email: "User@example.com", Code: "123456", NewPassword: "fresh password 123", Source: "192.0.2.1"}
	allow := func(context.Context, string) error { return nil }
	compromised := func(context.Context, string) (bool, error) {
		t.Fatal("hash ran without verified code")
		return false, nil
	}
	newService := func(repo *resetPreflightRepository, accountLimit func(context.Context, string) error) *app.PasswordResetService {
		return app.NewPasswordResetService(repo, allow, allow, accountLimit, compromised, make([]byte, 32))
	}
	if err := app.NewPasswordResetService(nil, allow, allow, allow, compromised, make([]byte, 32)).ResetPassword(context.Background(), input); !errors.Is(err, app.ErrPasswordResetUnavailable) {
		t.Fatalf("missing repository = %v", err)
	}
	limited := &resetPreflightRepository{}
	emailLimited := app.NewPasswordResetService(limited, allow,
		func(context.Context, string) error { return app.ErrRateLimited }, allow, compromised, make([]byte, 32))
	if err := emailLimited.ResetPassword(context.Background(), input); !errors.Is(err, app.ErrRateLimited) || limited.reads != 0 {
		t.Fatalf("email guess limit = %v, account reads=%d", err, limited.reads)
	}
	if err := newService(&resetPreflightRepository{}, allow).ResetPassword(context.Background(), inbound.ResetPasswordInput{
		Email: "not-an-email", Code: input.Code, NewPassword: input.NewPassword, Source: input.Source,
	}); !errors.Is(err, domain.ErrInvalidEmail) {
		t.Fatalf("invalid email = %v", err)
	}
	for _, test := range []struct {
		name  string
		repo  resetPreflightRepository
		limit func(context.Context, string) error
		want  error
	}{
		{"account lookup", resetPreflightRepository{findErr: errors.New("database down")}, allow, app.ErrPasswordResetUnavailable},
		{
			"account limit",
			resetPreflightRepository{found: true, account: outbound.PasswordRecoveryAccount{Subject: "subject"}},
			func(context.Context, string) error { return app.ErrRateLimited }, app.ErrRateLimited,
		},
		{"account lock", resetPreflightRepository{getErr: errors.New("database down")}, allow, app.ErrPasswordResetUnavailable},
		{"challenge read", resetPreflightRepository{found: true, account: outbound.PasswordRecoveryAccount{
			Subject: "subject", EmailVerified: true, HasPassword: true,
		}, challengeErr: errors.New("database down")}, allow, app.ErrPasswordResetUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := newService(&test.repo, test.limit).ResetPassword(context.Background(), input); !errors.Is(err, test.want) {
				t.Fatalf("reset error = %v", err)
			}
		})
	}
}
