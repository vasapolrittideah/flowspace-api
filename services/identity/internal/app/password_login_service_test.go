package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type loginRepository struct {
	account      outbound.PasswordAccount
	locked       *outbound.PasswordAccount
	found        bool
	err          error
	lockErr      error
	commitErr    error
	lookups      atomic.Int32
	transactions atomic.Int32
}

func (r *loginRepository) FindPasswordAccount(context.Context, string) (outbound.PasswordAccount, bool, error) {
	r.lookups.Add(1)
	return r.account, r.found, r.err
}

func (r *loginRepository) WithinPasswordSessionTransaction(_ context.Context, fn func(outbound.PasswordSessionTransaction) error) error {
	r.transactions.Add(1)
	account := r.account
	if r.locked != nil {
		account = *r.locked
	}
	if err := fn(&loginTransaction{account: account, lockErr: r.lockErr}); err != nil {
		return err
	}
	return r.commitErr
}

type loginTransaction struct {
	account  outbound.PasswordAccount
	lockErr  error
	sessions int
}

func (t *loginTransaction) LockPasswordAccount(context.Context, string) (outbound.PasswordAccount, error) {
	return t.account, t.lockErr
}

func (t *loginTransaction) Create(_ context.Context, _ string, _ []byte) (outbound.SessionRecord, error) {
	t.sessions++
	now := time.Now()
	return outbound.SessionRecord{ID: "session", CreatedAt: now, IdleExpiresAt: now.Add(30 * 24 * time.Hour), AbsoluteExpiresAt: now.Add(90 * 24 * time.Hour)}, nil
}

type loginSigner struct{}

func (loginSigner) Sign(outbound.AccessTokenClaims) (string, error) { return "access", nil }

func TestPasswordLoginLimitsBeforeHashAndGenericFailure(t *testing.T) {
	repo := &loginRepository{}
	limitErr := errors.New("limit unavailable")
	service := NewPasswordLoginService(repo, loginSigner{}, func(context.Context, string, string) error { return limitErr })
	input := inbound.CreatePasswordSessionInput{Email: "User@EXAMPLE.COM", Password: "wrong-password", Source: "192.0.2.1"}
	if _, err := service.CreatePasswordSession(context.Background(), input); !errors.Is(err, limitErr) || repo.lookups.Load() != 0 {
		t.Fatalf("limit result = %v, lookups = %d", err, repo.lookups.Load())
	}
	service = NewPasswordLoginService(repo, loginSigner{}, func(context.Context, string, string) error { return nil })
	if result, err := service.CreatePasswordSession(context.Background(), input); !errors.Is(err, ErrInvalidCredentials) || result.AccessToken != "" || repo.transactions.Load() != 0 {
		t.Fatalf("unknown login returned %v", err)
	}
	repo.found = true
	repo.account = outbound.PasswordAccount{Subject: "subject", PasswordHash: unknownAccountVerifier}
	if result, err := service.CreatePasswordSession(context.Background(), input); !errors.Is(err, ErrInvalidCredentials) || result.AccessToken != "" || repo.transactions.Load() != 0 {
		t.Fatalf("wrong password returned %v", err)
	}
}

func TestPasswordLoginReturnsTokensOnlyAfterCommit(t *testing.T) {
	hash, err := domain.HashPassword(context.Background(), "correct horse battery staple", func(context.Context, string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	repo := &loginRepository{found: true, account: outbound.PasswordAccount{Subject: "subject", PasswordHash: hash, EmailVerified: true}}
	service := NewPasswordLoginService(repo, loginSigner{}, func(context.Context, string, string) error { return nil })
	input := inbound.CreatePasswordSessionInput{Email: "User@example.com", Password: "correct horse battery staple", Source: "192.0.2.1"}
	result, err := service.CreatePasswordSession(context.Background(), input)
	if err != nil || result.Subject != "subject" || !result.EmailVerified || result.AccessToken == "" || result.RefreshToken == "" || result.AccessToken == result.RefreshToken || result.SessionExpiresAt.IsZero() || repo.transactions.Load() != 1 {
		t.Fatalf("committed login failed: %v", err)
	}
	repo.commitErr = errors.New("commit failed")
	result, err = service.CreatePasswordSession(context.Background(), input)
	if !errors.Is(err, ErrLoginUnavailable) || result.AccessToken != "" {
		t.Fatalf("commit failure returned %v", err)
	}
}

func TestPasswordLoginRejectsRetiredOrChangedAccountBeforeIssue(t *testing.T) {
	hash, err := domain.HashPassword(context.Background(), "correct horse battery staple", func(context.Context, string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	repo := &loginRepository{found: true, account: outbound.PasswordAccount{Subject: "subject", PasswordHash: hash}}
	service := NewPasswordLoginService(repo, loginSigner{}, func(context.Context, string, string) error { return nil })
	input := inbound.CreatePasswordSessionInput{Email: "User@example.com", Password: "correct horse battery staple", Source: "192.0.2.1"}
	repo.lockErr = outbound.ErrUnauthenticated
	result, err := service.CreatePasswordSession(context.Background(), input)
	if !errors.Is(err, ErrInvalidCredentials) || result.AccessToken != "" {
		t.Fatalf("retired account returned %v", err)
	}
	repo.lockErr = nil
	repo.locked = &outbound.PasswordAccount{Subject: "subject", PasswordHash: unknownAccountVerifier}
	result, err = service.CreatePasswordSession(context.Background(), input)
	if !errors.Is(err, ErrInvalidCredentials) || result.AccessToken != "" {
		t.Fatalf("changed hash returned %v", err)
	}
}

func TestPasswordLoginValidatesBeforeLimitAndNormalizesEmail(t *testing.T) {
	limitCalls := 0
	service := NewPasswordLoginService(&loginRepository{}, loginSigner{}, func(_ context.Context, source, email string) error {
		limitCalls++
		if source != "192.0.2.1" || email != "User@example.com" {
			t.Fatalf("limit keys = %q, %q", source, email)
		}
		return nil
	})
	for _, input := range []inbound.CreatePasswordSessionInput{
		{Email: "invalid", Password: "wrong-password", Source: "192.0.2.1"},
		{Email: "User@example.com", Password: "", Source: "192.0.2.1"},
	} {
		if _, err := service.CreatePasswordSession(context.Background(), input); err == nil {
			t.Fatal("accepted malformed input")
		}
	}
	if limitCalls != 0 {
		t.Fatalf("limit called %d times for malformed input", limitCalls)
	}
	_, _ = service.CreatePasswordSession(context.Background(), inbound.CreatePasswordSessionInput{Email: "User@EXAMPLE.COM", Password: "wrong-password", Source: "192.0.2.1"})
	if limitCalls != 1 {
		t.Fatalf("limit calls = %d", limitCalls)
	}
}

func TestPasswordLoginBoundsHashWork(t *testing.T) {
	service := NewPasswordLoginService(&loginRepository{}, loginSigner{}, func(context.Context, string, string) error { return nil })
	active, maximum := 0, 0
	var mu sync.Mutex
	service.verify = func(context.Context, string, string) (bool, error) {
		mu.Lock()
		active++
		if active > maximum {
			maximum = active
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		return false, nil
	}
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			_, _ = service.CreatePasswordSession(context.Background(), inbound.CreatePasswordSessionInput{Email: "User@example.com", Password: "wrong-password", Source: "192.0.2.1"})
		})
	}
	group.Wait()
	if maximum != 2 {
		t.Fatalf("maximum concurrent hashes = %d", maximum)
	}
}

func TestPasswordLoginHashCapacityAndFailureFailClosed(t *testing.T) {
	service := NewPasswordLoginService(&loginRepository{}, loginSigner{}, func(context.Context, string, string) error { return nil })
	input := inbound.CreatePasswordSessionInput{Email: "User@example.com", Password: "wrong-password", Source: "192.0.2.1"}
	service.hashSlots <- struct{}{}
	service.hashSlots <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.CreatePasswordSession(ctx, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("full hash capacity result = %v", err)
	}
	<-service.hashSlots
	<-service.hashSlots
	service.verify = func(context.Context, string, string) (bool, error) { return false, errors.New("hash failure") }
	result, err := service.CreatePasswordSession(context.Background(), input)
	if !errors.Is(err, ErrLoginUnavailable) || result.AccessToken != "" {
		t.Fatalf("hash failure returned %v", err)
	}
}
