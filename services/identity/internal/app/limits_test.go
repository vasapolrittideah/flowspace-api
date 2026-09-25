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

type limitStoreStub struct {
	requests []recordedLimit
	allowed  bool
	err      error
}

func (s *limitStoreStub) Record(_ context.Context, scope, key, action string, maximum, dailyMaximum int, window, interval time.Duration) (bool, error) {
	s.requests = append(s.requests, recordedLimit{scope, key, action, maximum, dailyMaximum, window, interval})
	return s.allowed, s.err
}

func TestLimitsApplySharedPolicies(t *testing.T) {
	for _, test := range []struct {
		name string
		call func(*Limits) error
		want recordedLimit
	}{
		{"signup", func(l *Limits) error { return l.Signup(context.Background(), "192.0.2.1") }, recordedLimit{"source", "192.0.2.1", "signup", 10, 0, time.Hour, 0}},
		{"code request", func(l *Limits) error { return l.CodeRequest(context.Background(), "192.0.2.1") }, recordedLimit{"source", "192.0.2.1", "code-request", 60, 0, time.Hour, 0}},
		{"wrong code", func(l *Limits) error { return l.WrongCode(context.Background(), "192.0.2.1") }, recordedLimit{"source", "192.0.2.1", "code-guess", 100, 0, time.Hour, 0}},
		{"code issue", func(l *Limits) error { return l.CodeIssue(context.Background(), "subject") }, recordedLimit{"account", "subject", "code-request", 5, 0, time.Hour, time.Minute}},
		{"account guesses", func(l *Limits) error { return l.AccountWrongCode(context.Background(), "subject") }, recordedLimit{"account", "subject", "code-guess", 10, 20, time.Hour, 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &limitStoreStub{allowed: true}
			if err := test.call(NewLimits(store)); err != nil {
				t.Fatal(err)
			}
			if len(store.requests) == 0 || store.requests[0] != test.want {
				t.Fatalf("first request = %+v, want %+v", store.requests, test.want)
			}
		})
	}
}

func TestLimitsRejectDeniedAndUnavailable(t *testing.T) {
	store := &limitStoreStub{}
	limits := NewLimits(store)
	if err := limits.Signup(context.Background(), "192.0.2.1"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("denied signup = %v", err)
	}
	store.err = errors.New("database unavailable")
	if err := limits.CodeRequest(context.Background(), "192.0.2.1"); !errors.Is(err, ErrLimitUnavailable) {
		t.Fatalf("store failure = %v", err)
	}
	if err := limits.Signup(context.Background(), ""); !errors.Is(err, ErrInvalidLimitKey) {
		t.Fatalf("missing source = %v", err)
	}
}
