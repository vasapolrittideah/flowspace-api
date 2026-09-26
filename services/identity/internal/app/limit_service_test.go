package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

type recordedLimit struct {
	scope, key, action    string
	maximum, dailyMaximum int
	window, interval      time.Duration
}

type limitRepositoryStub struct {
	requests              []recordedLimit
	loginSource, loginKey string
	loginSourceMaximum    int
	loginEmailMaximum     int
	loginSourceWindow     time.Duration
	loginEmailWindow      time.Duration
	allowed               bool
	err                   error
}

func (s *limitRepositoryStub) Record(_ context.Context, scope, key, action string, maximum, dailyMaximum int, window, interval time.Duration) (bool, error) {
	s.requests = append(s.requests, recordedLimit{scope, key, action, maximum, dailyMaximum, window, interval})
	return s.allowed, s.err
}

func (s *limitRepositoryStub) RecordLogin(_ context.Context, source, emailKey string, sourceMaximum, emailMaximum int, sourceWindow, emailWindow time.Duration) (bool, error) {
	s.loginSource, s.loginKey = source, emailKey
	s.loginSourceMaximum, s.loginEmailMaximum = sourceMaximum, emailMaximum
	s.loginSourceWindow, s.loginEmailWindow = sourceWindow, emailWindow
	return s.allowed, s.err
}

func TestLimitServiceApplySharedPolicies(t *testing.T) {
	for _, test := range []struct {
		name string
		call func(*LimitService) error
		want recordedLimit
	}{
		{"signup", func(l *LimitService) error { return l.Signup(context.Background(), "192.0.2.1") }, recordedLimit{"source", "192.0.2.1", "signup", 10, 0, time.Hour, 0}},
		{"code request", func(l *LimitService) error { return l.CodeRequest(context.Background(), "192.0.2.1") }, recordedLimit{"source", "192.0.2.1", "code-request", 60, 0, time.Hour, 0}},
		{"wrong code", func(l *LimitService) error { return l.WrongCode(context.Background(), "192.0.2.1") }, recordedLimit{"source", "192.0.2.1", "code-guess", 100, 0, time.Hour, 0}},
		{"code issue", func(l *LimitService) error { return l.CodeIssue(context.Background(), "subject") }, recordedLimit{"account", "subject", "code-request", 5, 0, time.Hour, time.Minute}},
		{"account guesses", func(l *LimitService) error { return l.AccountWrongCode(context.Background(), "subject") }, recordedLimit{"account", "subject", "code-guess", 10, 20, time.Hour, 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &limitRepositoryStub{allowed: true}
			if err := test.call(NewLimitService(repository)); err != nil {
				t.Fatal(err)
			}
			if len(repository.requests) == 0 || repository.requests[0] != test.want {
				t.Fatalf("first request = %+v, want %+v", repository.requests, test.want)
			}
		})
	}
}

func TestLimitServiceRejectDeniedAndUnavailable(t *testing.T) {
	repository := &limitRepositoryStub{}
	limits := NewLimitService(repository)
	if err := limits.Signup(context.Background(), "192.0.2.1"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("denied signup = %v", err)
	}
	repository.err = errors.New("database unavailable")
	if err := limits.CodeRequest(context.Background(), "192.0.2.1"); !errors.Is(err, ErrLimitUnavailable) {
		t.Fatalf("repository failure = %v", err)
	}
	if err := limits.Signup(context.Background(), ""); !errors.Is(err, ErrInvalidLimitKey) {
		t.Fatalf("missing source = %v", err)
	}
}

func TestPasswordLoginLimitUsesNormalizedPrivateEmailKey(t *testing.T) {
	repository := &limitRepositoryStub{allowed: true}
	limits := NewLimitService(repository)
	if err := limits.PasswordLogin(context.Background(), "192.0.2.5", "User@EXAMPLE.COM"); err != nil {
		t.Fatal(err)
	}
	firstKey := repository.loginKey
	if repository.loginSource != "192.0.2.5" || repository.loginSourceMaximum != 60 || repository.loginEmailMaximum != 10 || repository.loginSourceWindow != time.Hour || repository.loginEmailWindow != 15*time.Minute {
		t.Fatalf("login policy = %+v", repository)
	}
	if len(firstKey) != 64 || firstKey == "User@example.com" {
		t.Fatalf("login email key is not a digest: %q", firstKey)
	}
	if err := limits.PasswordLogin(context.Background(), "192.0.2.6", "User@example.com"); err != nil {
		t.Fatal(err)
	}
	if repository.loginKey != firstKey {
		t.Fatal("equivalent email addresses used different limit keys")
	}
	if err := limits.PasswordLogin(context.Background(), "192.0.2.6", "user@example.com"); err != nil {
		t.Fatal(err)
	}
	if repository.loginKey == firstKey {
		t.Fatal("distinct local parts used the same limit key")
	}
}

func TestPasswordLoginLimitRejectsDeniedUnavailableAndInvalid(t *testing.T) {
	repository := &limitRepositoryStub{}
	limits := NewLimitService(repository)
	if err := limits.PasswordLogin(context.Background(), "192.0.2.5", "user@example.com"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("denied login = %v", err)
	}
	repository.err = errors.New("database unavailable")
	if err := limits.PasswordLogin(context.Background(), "192.0.2.5", "user@example.com"); !errors.Is(err, ErrLimitUnavailable) {
		t.Fatalf("repository failure = %v", err)
	}
	for _, input := range []struct{ source, email string }{{"", "user@example.com"}, {"192.0.2.5", "invalid"}} {
		repository.loginSource = ""
		if err := limits.PasswordLogin(context.Background(), input.source, input.email); err == nil || repository.loginSource != "" {
			t.Fatalf("invalid login limit input reached repository: %v", err)
		}
	}
}
