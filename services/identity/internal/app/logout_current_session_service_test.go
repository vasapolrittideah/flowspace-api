package app

import (
	"context"
	"errors"
	"testing"

	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type logoutRepositoryStub struct {
	subject, sessionID string
	err                error
	calls              int
}

func (r *logoutRepositoryStub) RevokeCurrent(_ context.Context, subject, sessionID string) error {
	r.subject, r.sessionID = subject, sessionID
	r.calls++
	return r.err
}

func (r *logoutRepositoryStub) RevokeAll(ctx context.Context, subject, sessionID string) error {
	return r.RevokeCurrent(ctx, subject, sessionID)
}

func TestLogoutCurrentSessionService(t *testing.T) {
	repository := &logoutRepositoryStub{}
	service := NewCurrentSessionLogoutService(repository)
	input := inbound.LogoutCurrentSessionInput{Subject: "subject-1", SessionID: "session-1"}
	if err := service.LogoutCurrentSession(t.Context(), input); err != nil || repository.subject != input.Subject || repository.sessionID != input.SessionID || repository.calls != 1 {
		t.Fatalf("logout = %v, repository = %+v", err, repository)
	}
	for _, invalid := range []inbound.LogoutCurrentSessionInput{{SessionID: input.SessionID}, {Subject: input.Subject}} {
		if err := service.LogoutCurrentSession(t.Context(), invalid); !errors.Is(err, outbound.ErrUnauthenticated) || repository.calls != 1 {
			t.Fatalf("invalid input = %+v, error = %v, calls = %d", invalid, err, repository.calls)
		}
	}
	for _, test := range []struct{ source, want error }{
		{outbound.ErrUnauthenticated, outbound.ErrUnauthenticated},
		{errors.New("database failed"), ErrCurrentLogoutUnavailable},
		{context.Canceled, context.Canceled},
	} {
		repository.err = test.source
		if err := service.LogoutCurrentSession(t.Context(), input); !errors.Is(err, test.want) {
			t.Fatalf("repository error = %v, result = %v", test.source, err)
		}
	}
	if err := NewCurrentSessionLogoutService(nil).LogoutCurrentSession(t.Context(), input); !errors.Is(err, ErrCurrentLogoutUnavailable) {
		t.Fatalf("missing repository = %v", err)
	}
}
