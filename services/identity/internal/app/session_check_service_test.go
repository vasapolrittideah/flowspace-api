package app

import (
	"context"
	"errors"
	"testing"

	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type sessionCheckRepositoryStub struct {
	verified bool
	err      error
	calls    int
}

func (r *sessionCheckRepositoryStub) Check(_ context.Context, subject, sessionID string) (bool, error) {
	r.calls++
	if subject != "subject-1" || sessionID != "session-1" {
		return false, outbound.ErrUnauthenticated
	}
	return r.verified, r.err
}

func TestSessionCheckService(t *testing.T) {
	repository := &sessionCheckRepositoryStub{}
	service := NewSessionCheckService(repository)
	input := inbound.CheckSessionInput{Subject: "subject-1", SessionID: "session-1"}

	verified, err := service.CheckSession(t.Context(), input)
	if err != nil || verified || repository.calls != 1 {
		t.Fatalf("unverified session = %t, %v, calls = %d", verified, err, repository.calls)
	}
	repository.verified = true
	verified, err = service.CheckSession(t.Context(), input)
	if err != nil || !verified || repository.calls != 2 {
		t.Fatalf("current verification = %t, %v, calls = %d", verified, err, repository.calls)
	}

	for _, invalid := range []inbound.CheckSessionInput{{SessionID: "session-1"}, {Subject: "subject-1"}, {Subject: "wrong", SessionID: "session-1"}, {Subject: "subject-1", SessionID: "wrong"}} {
		verified, err := service.CheckSession(t.Context(), invalid)
		if verified || !errors.Is(err, outbound.ErrUnauthenticated) {
			t.Fatalf("invalid identifiers %+v = %t, %v", invalid, verified, err)
		}
	}
}

func TestSessionCheckServiceErrors(t *testing.T) {
	repository := &sessionCheckRepositoryStub{verified: true}
	service := NewSessionCheckService(repository)
	input := inbound.CheckSessionInput{Subject: "subject-1", SessionID: "session-1"}
	repository.err = outbound.ErrUnauthenticated
	verified, err := service.CheckSession(t.Context(), input)
	if verified || !errors.Is(err, outbound.ErrUnauthenticated) {
		t.Fatalf("inactive session = %t, %v", verified, err)
	}

	repository.err = errors.New("database failed")
	verified, err = service.CheckSession(t.Context(), input)
	if verified || !errors.Is(err, ErrSessionCheckUnavailable) {
		t.Fatalf("database failure = %t, %v", verified, err)
	}
	verified, err = NewSessionCheckService(nil).CheckSession(t.Context(), input)
	if verified || !errors.Is(err, ErrSessionCheckUnavailable) {
		t.Fatalf("missing repository = %t, %v", verified, err)
	}
}
