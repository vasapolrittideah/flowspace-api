package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type passwordReauthenticationRepository struct {
	window                                      domain.ReauthenticationWindow
	now                                         time.Time
	hash                                        string
	accountErr, sessionErr, writeErr, commitErr error
	deadline                                    time.Time
	proofs, limits, hashes                      int
}

func (r *passwordReauthenticationRepository) WithinPasswordReauthenticationTransaction(_ context.Context, fn func(outbound.PasswordReauthenticationTransaction) error) (domain.ReauthenticationWindow, error) {
	if err := fn(r); err != nil {
		return domain.ReauthenticationWindow{}, err
	}
	return r.window, r.commitErr
}

func (*passwordReauthenticationRepository) GetActiveAccountForSession(context.Context, string, string) (outbound.AccountState, error) {
	return outbound.AccountState{Email: "User@example.com"}, nil
}

func (r *passwordReauthenticationRepository) LockPasswordAccount(context.Context, string) (outbound.PasswordAccount, error) {
	return outbound.PasswordAccount{PasswordHash: r.hash}, r.accountErr
}

func (r *passwordReauthenticationRepository) LockSessionForReauthentication(context.Context, string, string) (domain.ReauthenticationWindow, time.Time, error) {
	return r.window, r.now, r.sessionErr
}

func (r *passwordReauthenticationRepository) RequireCommitBefore(deadline time.Time) {
	r.deadline = deadline
}

func (r *passwordReauthenticationRepository) ReauthenticateSession(context.Context, string, string) error {
	r.proofs++
	r.window.AuthenticatedAt = r.now
	return r.writeErr
}

func TestPasswordReauthenticationProofAndRetry(t *testing.T) {
	hash, err := domain.HashPassword(t.Context(), "correct café battery staple", func(context.Context, string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	r := &passwordReauthenticationRepository{hash: hash, now: now, window: domain.ReauthenticationWindow{IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(2 * time.Hour)}}
	service := NewPasswordReauthenticationService(r, func(_ context.Context, source, email string) error {
		r.limits++
		if source != "192.0.2.1" || email != "User@example.com" {
			t.Fatal("wrong limit keys")
		}
		return nil
	})
	password := norm.NFD.String("correct café battery staple")
	input := inbound.ReauthenticateWithPasswordInput{Subject: "subject", SessionID: "session", Password: password, Source: "192.0.2.1"}
	result, err := service.ReauthenticateWithPassword(t.Context(), input)
	if err != nil || !result.ExpiresAt.Equal(now.Add(10*time.Minute)) || r.proofs != 1 {
		t.Fatalf("proof result=%v, err=%v, writes=%d", result, err, r.proofs)
	}
	r.window.AuthenticatedAt = now
	r.now = now.Add(time.Minute)
	result, err = service.ReauthenticateWithPassword(t.Context(), input)
	if err != nil || !result.ExpiresAt.Equal(now.Add(10*time.Minute)) || r.proofs != 1 || r.limits != 2 {
		t.Fatalf("retry result=%v err=%v writes=%d limits=%d", result, err, r.proofs, r.limits)
	}
	input.Password = "wrong password"
	if _, err := service.ReauthenticateWithPassword(t.Context(), input); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("open-window retry skipped password: %v", err)
	}
}

func TestPasswordReauthenticationRejectsBeforeHashAndFailsClosed(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		name                                   string
		limit, account, session, write, commit error
		hash                                   string
		matched                                bool
		want                                   error
	}{
		{name: "wrong password", hash: unknownAccountVerifier, want: ErrInvalidCredentials},
		{name: "no password", account: outbound.ErrUnauthenticated, want: ErrInvalidCredentials},
		{name: "full limit", limit: ErrRateLimited, want: ErrRateLimited},
		{name: "limit unavailable", limit: ErrLimitUnavailable, want: ErrLimitUnavailable},
		{name: "inactive session", session: outbound.ErrUnauthenticated, want: outbound.ErrUnauthenticated},
		{name: "database unavailable", session: errors.New("private database"), want: ErrReauthenticationUnavailable},
		{name: "password read failure", account: errors.New("private hash"), want: ErrReauthenticationUnavailable},
		{name: "write failure", matched: true, write: errors.New("private write"), want: ErrReauthenticationUnavailable},
		{name: "commit failure", matched: true, commit: errors.New("private commit"), want: ErrReauthenticationUnavailable},
		{name: "canceled commit", matched: true, commit: context.Canceled, want: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := &passwordReauthenticationRepository{hash: test.hash, now: now, window: domain.ReauthenticationWindow{IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(time.Hour)}, accountErr: test.account, sessionErr: test.session, writeErr: test.write, commitErr: test.commit}
			s := NewPasswordReauthenticationService(r, func(context.Context, string, string) error { r.limits++; return test.limit })
			s.verify = func(context.Context, string, string) (bool, error) { r.hashes++; return test.matched, nil }
			result, err := s.ReauthenticateWithPassword(t.Context(), inbound.ReauthenticateWithPasswordInput{Subject: "subject", SessionID: "session", Source: "source", Password: "password"})
			if !errors.Is(err, test.want) || !result.ExpiresAt.IsZero() {
				t.Fatalf("failure result=%v err=%v want=%v", result, err, test.want)
			}
			if (test.limit != nil || test.session != nil) && r.hashes != 0 {
				t.Fatal("hashed before session/limit admission")
			}
		})
	}
}

func TestPasswordReauthenticationInputAndHashFailures(t *testing.T) {
	r := &passwordReauthenticationRepository{}
	s := NewPasswordReauthenticationService(r, func(context.Context, string, string) error { return nil })
	for _, input := range []inbound.ReauthenticateWithPasswordInput{{}, {Subject: "subject", SessionID: "session"}} {
		if _, err := s.ReauthenticateWithPassword(t.Context(), input); err == nil {
			t.Fatal("accepted invalid input")
		}
	}
	input := inbound.ReauthenticateWithPasswordInput{Subject: "subject", SessionID: "session", Password: "password"}
	s.verify = func(context.Context, string, string) (bool, error) { return false, errors.New("private verifier") }
	if _, err := s.ReauthenticateWithPassword(t.Context(), input); !errors.Is(err, ErrReauthenticationUnavailable) {
		t.Fatal(err)
	}
	s.hashSlots <- struct{}{}
	s.hashSlots <- struct{}{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.ReauthenticateWithPassword(ctx, input); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := NewPasswordReauthenticationService(nil, nil).ReauthenticateWithPassword(t.Context(), input); !errors.Is(err, ErrReauthenticationUnavailable) {
		t.Fatal(err)
	}
}

func TestPasswordReauthenticationRegistersSessionDeadline(t *testing.T) {
	now := time.Now()
	for _, absoluteFirst := range []bool{false, true} {
		idle, absolute := now.Add(3*time.Minute), now.Add(time.Hour)
		if absoluteFirst {
			idle, absolute = absolute, idle
		}
		r := &passwordReauthenticationRepository{now: now, window: domain.ReauthenticationWindow{IdleExpiresAt: idle, AbsoluteExpiresAt: absolute}}
		s := NewPasswordReauthenticationService(r, func(context.Context, string, string) error { return nil })
		s.verify = func(context.Context, string, string) (bool, error) { return true, nil }
		result, err := s.ReauthenticateWithPassword(t.Context(), inbound.ReauthenticateWithPasswordInput{Subject: "subject", SessionID: "session", Password: "password"})
		if err != nil || !r.deadline.Equal(now.Add(3*time.Minute)) || !result.ExpiresAt.Equal(r.deadline) {
			t.Fatalf("session deadline=%v result=%v err=%v", r.deadline, result, err)
		}
	}
}

type concurrentReauthenticationRepository struct{}

func (concurrentReauthenticationRepository) WithinPasswordReauthenticationTransaction(_ context.Context, fn func(outbound.PasswordReauthenticationTransaction) error) (domain.ReauthenticationWindow, error) {
	now := time.Now()
	r := &passwordReauthenticationRepository{now: now, window: domain.ReauthenticationWindow{IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(time.Hour)}}
	if err := fn(r); err != nil {
		return domain.ReauthenticationWindow{}, err
	}
	return r.window, nil
}

func TestPasswordReauthenticationBoundsHashWork(t *testing.T) {
	s := NewPasswordReauthenticationService(concurrentReauthenticationRepository{}, func(context.Context, string, string) error { return nil })
	var active, maximum atomic.Int32
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	s.verify = func(ctx context.Context, _, _ string) (bool, error) {
		count := active.Add(1)
		for previous := maximum.Load(); count > previous; previous = maximum.Load() {
			if maximum.CompareAndSwap(previous, count) {
				break
			}
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			active.Add(-1)
			return false, ctx.Err()
		}
		time.Sleep(20 * time.Millisecond)
		active.Add(-1)
		return true, nil
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var group sync.WaitGroup
	failures := make(chan error, 8)
	for range 8 {
		group.Go(func() {
			_, err := s.ReauthenticateWithPassword(ctx, inbound.ReauthenticateWithPasswordInput{Subject: "subject", SessionID: "session", Password: "password"})
			failures <- err
		})
	}
	for range 2 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("verifiers did not start")
		}
	}
	if active.Load() != 2 {
		t.Fatalf("active verifiers=%d", active.Load())
	}
	close(release)
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if maximum.Load() != 2 {
		t.Fatalf("maximum concurrent hashes=%d", maximum.Load())
	}
}
