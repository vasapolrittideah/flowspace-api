package app

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type codeReauthenticationRepository struct {
	account                    outbound.AccountState
	window                     domain.ReauthenticationWindow
	now, last, deadline        time.Time
	challenge                  outbound.ChallengeState
	commitErr, errorAt         error
	fail                       string
	allowed                    bool
	outbox, deliveries, proofs int
}

func (r *codeReauthenticationRepository) step(name string) error {
	if r.fail == name {
		return r.errorAt
	}
	return nil
}

func (r *codeReauthenticationRepository) WithinReauthenticationCodeIssueTransaction(_ context.Context, fn func(outbound.ReauthenticationCodeIssueTransaction) error) error {
	pending := *r
	pending.deadline = time.Time{}
	if err := fn(&pending); err != nil {
		return err
	}
	if r.commitErr != nil {
		return r.commitErr
	}
	*r = pending
	return nil
}

func (r *codeReauthenticationRepository) WithinCodeReauthenticationTransaction(_ context.Context, fn func(outbound.CodeReauthenticationTransaction) error) (domain.ReauthenticationWindow, error) {
	pending := *r
	pending.deadline = time.Time{}
	if err := fn(&pending); err != nil {
		return domain.ReauthenticationWindow{}, err
	}
	if r.commitErr != nil {
		return domain.ReauthenticationWindow{}, r.commitErr
	}
	*r = pending
	return r.window, nil
}

func (r *codeReauthenticationRepository) GetActiveAccountForSession(context.Context, string, string) (outbound.AccountState, error) {
	return r.account, r.step("account")
}

func (r *codeReauthenticationRepository) LockSessionForReauthentication(context.Context, string, string) (domain.ReauthenticationWindow, time.Time, error) {
	return r.window, r.now, r.step("session")
}

func (r *codeReauthenticationRepository) RequireCommitBefore(deadline time.Time) {
	if r.deadline.IsZero() || deadline.Before(r.deadline) {
		r.deadline = deadline
	}
}

func (r *codeReauthenticationRepository) LatestReauthenticationCodeIssue(context.Context, string) (time.Time, error) {
	return r.last, r.step("latest")
}

func (r *codeReauthenticationRepository) CanIssueCode(context.Context, string) (bool, error) {
	return r.allowed, r.step("allow")
}

func (r *codeReauthenticationRepository) ReplaceReauthenticationChallenge(context.Context, string) error {
	return r.step("replace")
}

func (r *codeReauthenticationRepository) CreateReauthenticationChallenge(_ context.Context, _, email string, verifier [32]byte) (string, error) {
	r.challenge = outbound.ChallengeState{ID: "challenge", Email: email, Verifier: verifier, ExpiresAt: r.now.Add(10 * time.Minute)}
	r.last = r.now
	return "challenge", r.step("create")
}

func (r *codeReauthenticationRepository) StoreDelivery(context.Context, string, outbound.DeliveryMaterial) error {
	r.deliveries++
	return r.step("delivery")
}

func (r *codeReauthenticationRepository) CreateOutboxEvent(context.Context, string) error {
	r.outbox++
	return r.step("outbox")
}

func (r *codeReauthenticationRepository) GetCurrentReauthenticationChallenge(context.Context, string) (outbound.ChallengeState, bool, error) {
	return r.challenge, r.challenge.ID != "", r.step("challenge")
}

func (r *codeReauthenticationRepository) IncrementChallengeWrongGuess(context.Context, string) error {
	r.challenge.WrongGuesses++
	return r.step("guess")
}

func (r *codeReauthenticationRepository) ConsumeChallenge(context.Context, string) (bool, error) {
	if r.fail == "used" {
		return false, nil
	}
	r.challenge.ID = ""
	return true, r.step("consume")
}

func (r *codeReauthenticationRepository) ReauthenticateSession(context.Context, string, string) error {
	r.proofs++
	r.window.AuthenticatedAt = r.now
	return r.step("proof")
}

type reauthenticationCodeProtector struct {
	code, purpose string
	err           error
}

func (p *reauthenticationCodeProtector) Protect(_, purpose, _, _, code string) (outbound.DeliveryMaterial, error) {
	p.code = code
	p.purpose = purpose
	return outbound.DeliveryMaterial{}, p.err
}

func newCodeReauthenticationFixture(t *testing.T) (*CodeReauthenticationService, *codeReauthenticationRepository, *reauthenticationCodeProtector) {
	t.Helper()
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	r := &codeReauthenticationRepository{now: now, allowed: true, account: outbound.AccountState{Email: "User@example.com", EmailVerified: true}, window: domain.ReauthenticationWindow{IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(2 * time.Hour)}}
	p := &reauthenticationCodeProtector{}
	limit := func(context.Context, string) error { return nil }
	return NewCodeReauthenticationService(r, p, limit, limit, limit, bytes.Repeat([]byte{7}, 32)), r, p
}

func TestReauthenticationCodeRequestCooldownAndSingleUse(t *testing.T) {
	s, r, p := newCodeReauthenticationFixture(t)
	input := inbound.RequestReauthenticationCodeInput{Subject: "subject", SessionID: "session", Source: "192.0.2.1"}
	if err := s.RequestReauthenticationCode(t.Context(), input); err != nil || r.outbox != 1 || r.deliveries != 1 || p.purpose != "reauthenticate" {
		t.Fatalf("request err=%v events=%d deliveries=%d purpose=%q", err, r.outbox, r.deliveries, p.purpose)
	}
	original := r.challenge
	r.now = r.now.Add(60*time.Second - time.Nanosecond)
	if err := s.RequestReauthenticationCode(t.Context(), input); err != nil || r.challenge != original || r.outbox != 1 {
		t.Fatalf("cooldown replaced challenge: %v", err)
	}
	r.now = r.now.Add(time.Nanosecond)
	if err := s.RequestReauthenticationCode(t.Context(), input); err != nil || r.outbox != 2 {
		t.Fatalf("exact60second boundary: %v", err)
	}
}

func TestReauthenticationCodeSingleUse(t *testing.T) {
	s, r, p := newCodeReauthenticationFixture(t)
	input := inbound.RequestReauthenticationCodeInput{Subject: "subject", SessionID: "session", Source: "192.0.2.1"}
	if err := s.RequestReauthenticationCode(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	result, err := s.ReauthenticateWithCode(t.Context(), inbound.ReauthenticateWithCodeInput{Subject: "subject", SessionID: "session", Source: input.Source, Code: p.code})
	if err != nil || !result.ExpiresAt.Equal(r.now.Add(10*time.Minute)) || r.proofs != 1 || !r.account.EmailVerified {
		t.Fatalf("proof=%v err=%v writes=%d", result, err, r.proofs)
	}
	before := r.window
	if _, err := s.ReauthenticateWithCode(t.Context(), inbound.ReauthenticateWithCodeInput{Subject: "subject", SessionID: "session", Source: input.Source, Code: p.code}); !errors.Is(err, ErrInvalidReauthenticationCode) || r.window != before {
		t.Fatalf("used code retry changed window: %v", err)
	}
}

func TestReauthenticationCodeKeepsOpenWindowAndCommitsWrongGuesses(t *testing.T) {
	s, r, p := newCodeReauthenticationFixture(t)
	r.window.AuthenticatedAt = r.now.Add(-time.Minute)
	request := inbound.RequestReauthenticationCodeInput{Subject: "subject", SessionID: "session", Source: "192.0.2.1"}
	if err := s.RequestReauthenticationCode(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	before := r.window
	input := inbound.ReauthenticateWithCodeInput{Subject: "subject", SessionID: "session", Source: request.Source, Code: "abcdef"}
	if _, err := s.ReauthenticateWithCode(t.Context(), input); !errors.Is(err, ErrInvalidReauthenticationCode) || r.proofs != 0 {
		t.Fatal(err)
	}
	input.Code = "000000"
	if input.Code == p.code {
		input.Code = "111111"
	}
	if _, err := s.ReauthenticateWithCode(t.Context(), input); !errors.Is(err, ErrInvalidReauthenticationCode) || r.challenge.WrongGuesses != 1 || r.window != before {
		t.Fatalf("guess err=%v count=%d", err, r.challenge.WrongGuesses)
	}
	input.Code = p.code
	result, err := s.ReauthenticateWithCode(t.Context(), input)
	if err != nil || !result.ExpiresAt.Equal(before.ExpiresAt()) || r.proofs != 0 {
		t.Fatalf("open-window result=%v err=%v", result, err)
	}
}

func TestReauthenticationCodeRejectsExpiredAndCrossPurposeProof(t *testing.T) {
	for _, purpose := range []domain.CodePurpose{domain.PurposeVerifyEmail, domain.PurposeClaimAccount, domain.PurposePasswordReset, domain.PurposeReauthenticate} {
		s, r, _ := newCodeReauthenticationFixture(t)
		code, verifier, expiry, err := domain.NewChallenge(s.verifierKey, "subject", r.account.Email, purpose, r.now)
		if err != nil {
			t.Fatal(err)
		}
		r.challenge = outbound.ChallengeState{ID: "challenge", Email: r.account.Email, Verifier: verifier, ExpiresAt: expiry}
		if purpose == domain.PurposeReauthenticate {
			r.now = expiry
		}
		if result, err := s.ReauthenticateWithCode(t.Context(), inbound.ReauthenticateWithCodeInput{Subject: "subject", SessionID: "session", Source: "source", Code: code}); !errors.Is(err, ErrInvalidReauthenticationCode) || !result.ExpiresAt.IsZero() || r.proofs != 0 {
			t.Fatalf("accepted expired or cross-purpose code %s: %v", purpose, err)
		}
	}
}

func TestReauthenticationCodeRequestFailuresLeaveNoDelivery(t *testing.T) {
	for _, failure := range []string{"session", "account", "latest", "allow", "replace", "create", "delivery", "outbox", "protector", "commit", "unverified", "limit"} {
		t.Run(failure, func(t *testing.T) {
			s, r, p := newCodeReauthenticationFixture(t)
			r.fail = failure
			r.errorAt = errors.New("private database")
			want := ErrReauthenticationUnavailable
			switch failure {
			case "protector":
				p.err = r.errorAt
			case "commit":
				r.commitErr = r.errorAt
			case "unverified":
				r.account.EmailVerified = false
				want = ErrReauthenticationEmailUnverified
			case "limit":
				s.requestLimit = func(context.Context, string) error { return ErrRateLimited }
				want = ErrRateLimited
			}
			if err := s.RequestReauthenticationCode(t.Context(), inbound.RequestReauthenticationCodeInput{Subject: "subject", SessionID: "session", Source: "source"}); !errors.Is(err, want) || r.outbox != 0 || r.deliveries != 0 {
				t.Fatalf("failure=%v outbox=%d deliveries=%d", err, r.outbox, r.deliveries)
			}
		})
	}
}

func TestReauthenticationCodeGuessLimitsAndCommitFailure(t *testing.T) {
	for _, failure := range []string{"source", "account", "consume", "used", "proof", "canceled", "limit-store", "session-expiry", "code-expiry"} {
		t.Run(failure, func(t *testing.T) {
			s, r, p := newCodeReauthenticationFixture(t)
			if err := s.RequestReauthenticationCode(t.Context(), inbound.RequestReauthenticationCodeInput{Subject: "subject", SessionID: "session", Source: "source"}); err != nil {
				t.Fatal(err)
			}
			before := r.challenge
			beforeWindow := r.window
			r.fail = failure
			r.errorAt = errors.New("private failure")
			want := ErrReauthenticationUnavailable
			switch failure {
			case "source":
				s.guessSourceLimit = func(context.Context, string) error { return ErrRateLimited }
				want = ErrRateLimited
			case "account":
				r.fail = ""
				s.guessAccountLimit = func(context.Context, string) error { return ErrRateLimited }
				want = ErrRateLimited
			case "used":
				want = ErrInvalidReauthenticationCode
			case "canceled":
				r.commitErr = context.Canceled
				want = context.Canceled
			case "limit-store":
				s.guessSourceLimit = func(context.Context, string) error { return ErrLimitUnavailable }
				want = ErrLimitUnavailable
			case "code-expiry":
				r.commitErr = outbound.ErrReauthenticationRequired
				want = ErrInvalidReauthenticationCode
			case "session-expiry":
				r.window.IdleExpiresAt = r.now.Add(time.Second)
				beforeWindow = r.window
				r.commitErr = outbound.ErrReauthenticationRequired
				want = outbound.ErrUnauthenticated
			}
			result, err := s.ReauthenticateWithCode(t.Context(), inbound.ReauthenticateWithCodeInput{Subject: "subject", SessionID: "session", Source: "source", Code: p.code})
			if !errors.Is(err, want) || !result.ExpiresAt.IsZero() || r.proofs != 0 || r.challenge != before || r.window != beforeWindow {
				t.Fatalf("failure=%v result=%v proof=%d", err, result, r.proofs)
			}
		})
	}
}

func TestReauthenticationCodeRepositoryFailures(t *testing.T) {
	for _, failure := range []string{"account", "challenge", "guess", "commit"} {
		t.Run(failure, func(t *testing.T) {
			s, r, p := newCodeReauthenticationFixture(t)
			if err := s.RequestReauthenticationCode(t.Context(), inbound.RequestReauthenticationCodeInput{Subject: "subject", SessionID: "session", Source: "source"}); err != nil {
				t.Fatal(err)
			}
			before, window := r.challenge, r.window
			r.fail, r.errorAt = failure, errors.New("private repository error")
			if failure == "commit" {
				r.commitErr = r.errorAt
			}
			code := p.code
			if failure == "guess" {
				code = "000000"
				if code == p.code {
					code = "111111"
				}
			}
			result, err := s.ReauthenticateWithCode(t.Context(), inbound.ReauthenticateWithCodeInput{Subject: "subject", SessionID: "session", Source: "source", Code: code})
			if !errors.Is(err, ErrReauthenticationUnavailable) || !result.ExpiresAt.IsZero() || r.challenge != before || r.window != window || r.proofs != 0 {
				t.Fatalf("failure=%v result=%v proof=%d", err, result, r.proofs)
			}
		})
	}
}

func TestReauthenticationCodeRegistersEarliestDeadline(t *testing.T) {
	for _, kind := range []string{"code", "idle", "absolute"} {
		t.Run(kind, func(t *testing.T) {
			s, r, p := newCodeReauthenticationFixture(t)
			want := r.now.Add(10 * time.Minute)
			switch kind {
			case "idle":
				r.window.IdleExpiresAt = r.now.Add(3 * time.Minute)
				want = r.window.IdleExpiresAt
			case "absolute":
				r.window.AbsoluteExpiresAt = r.now.Add(3 * time.Minute)
				want = r.window.AbsoluteExpiresAt
			}
			if err := s.RequestReauthenticationCode(t.Context(), inbound.RequestReauthenticationCodeInput{Subject: "subject", SessionID: "session", Source: "source"}); err != nil {
				t.Fatal(err)
			}
			result, err := s.ReauthenticateWithCode(t.Context(), inbound.ReauthenticateWithCodeInput{Subject: "subject", SessionID: "session", Source: "source", Code: p.code})
			if err != nil || !r.deadline.Equal(want) || !result.ExpiresAt.Equal(want) {
				t.Fatalf("deadline=%v result=%v err=%v", r.deadline, result, err)
			}
		})
	}
}

func TestReauthenticationCodeRejectsAccountIssueLimit(t *testing.T) {
	s, r, _ := newCodeReauthenticationFixture(t)
	r.allowed = false
	if err := s.RequestReauthenticationCode(t.Context(), inbound.RequestReauthenticationCodeInput{Subject: "subject", SessionID: "session", Source: "source"}); !errors.Is(err, ErrRateLimited) || r.outbox != 0 {
		t.Fatalf("account issue limit=%v", err)
	}
}

func TestReauthenticationCodeRejectsUnavailableOrExhaustedChallenge(t *testing.T) {
	for _, kind := range []string{"missing", "email", "guesses", "unverified", "inactive"} {
		t.Run(kind, func(t *testing.T) {
			s, r, p := newCodeReauthenticationFixture(t)
			if err := s.RequestReauthenticationCode(t.Context(), inbound.RequestReauthenticationCodeInput{Subject: "subject", SessionID: "session", Source: "source"}); err != nil {
				t.Fatal(err)
			}
			want := ErrInvalidReauthenticationCode
			switch kind {
			case "missing":
				r.challenge.ID = ""
			case "email":
				r.challenge.Email = "previous@example.com"
			case "guesses":
				r.challenge.WrongGuesses = 5
			case "unverified":
				r.account.EmailVerified = false
			case "inactive":
				r.fail = "session"
				r.errorAt = outbound.ErrUnauthenticated
				want = outbound.ErrUnauthenticated
			}
			before := r.challenge
			if result, err := s.ReauthenticateWithCode(t.Context(), inbound.ReauthenticateWithCodeInput{Subject: "subject", SessionID: "session", Code: p.code, Source: "source"}); !errors.Is(err, want) || !result.ExpiresAt.IsZero() || r.proofs != 0 || r.challenge != before {
				t.Fatalf("%s proof=%v err=%v", kind, result, err)
			}
		})
	}
}
