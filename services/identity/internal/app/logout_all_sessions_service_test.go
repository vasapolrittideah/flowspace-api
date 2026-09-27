package app

import (
	"errors"
	"testing"

	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func TestLogoutAllSessionsService(t *testing.T) {
	repository := &logoutRepositoryStub{}
	service := NewLogoutAllSessionsService(repository)
	input := inbound.LogoutAllSessionsInput{Subject: "account-subject", SessionID: "current-session"}
	if err := service.LogoutAllSessions(t.Context(), input); err != nil || repository.calls != 1 || repository.subject != input.Subject || repository.sessionID != input.SessionID {
		t.Fatalf("all-session logout = %v, repository = %+v", err, repository)
	}
	if err := service.LogoutAllSessions(t.Context(), inbound.LogoutAllSessionsInput{Subject: input.Subject}); !errors.Is(err, outbound.ErrUnauthenticated) || repository.calls != 1 {
		t.Fatalf("missing session = %v, calls = %d", err, repository.calls)
	}
	repository.err = outbound.ErrUnauthenticated
	if err := service.LogoutAllSessions(t.Context(), input); !errors.Is(err, outbound.ErrUnauthenticated) {
		t.Fatalf("inactive session = %v", err)
	}
	repository.err = errors.New("database failed")
	if err := service.LogoutAllSessions(t.Context(), input); !errors.Is(err, ErrAllLogoutUnavailable) {
		t.Fatalf("database failure = %v", err)
	}
	if err := NewLogoutAllSessionsService(nil).LogoutAllSessions(t.Context(), input); !errors.Is(err, ErrAllLogoutUnavailable) {
		t.Fatalf("missing repository = %v", err)
	}
}
