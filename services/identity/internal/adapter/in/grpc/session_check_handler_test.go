package grpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	identitygrpc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/grpc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type sessionCheckServiceStub struct {
	input    inbound.CheckSessionInput
	verified bool
	err      error
}

func (s *sessionCheckServiceStub) CheckSession(_ context.Context, input inbound.CheckSessionInput) (bool, error) {
	s.input = input
	return s.verified, s.err
}

func TestSessionCheckHandler(t *testing.T) {
	service := &sessionCheckServiceStub{verified: true}
	handler := identitygrpc.NewSessionCheckHandler(service)
	request := &identityv1.CheckSessionRequest{Subject: "subject-1", SessionId: "session-1"}
	response, err := handler.CheckSession(t.Context(), request)
	if err != nil || !response.GetEmailVerified() || service.input != (inbound.CheckSessionInput{Subject: "subject-1", SessionID: "session-1"}) {
		t.Fatalf("active session = %+v, input = %+v, %v", response, service.input, err)
	}
	service.verified = false
	response, err = handler.CheckSession(t.Context(), request)
	if err != nil || response.GetEmailVerified() {
		t.Fatalf("unverified session = %+v, %v", response, err)
	}
	for _, test := range []struct {
		name string
		err  error
		want codes.Code
	}{
		{"inactive", outbound.ErrUnauthenticated, codes.Unauthenticated},
		{"unavailable", app.ErrSessionCheckUnavailable, codes.Unavailable},
		{"canceled", context.Canceled, codes.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			service.err = test.err
			response, err := handler.CheckSession(t.Context(), request)
			if response != nil || status.Code(err) != test.want {
				t.Fatalf("response = %+v, code = %s", response, status.Code(err))
			}
		})
	}
	service.err = nil
	for _, invalid := range []*identityv1.CheckSessionRequest{nil, {}, {Subject: "subject-1"}, {SessionId: "session-1"}} {
		response, err := handler.CheckSession(t.Context(), invalid)
		if response != nil || status.Code(err) != codes.Unauthenticated {
			t.Fatalf("invalid request %+v = %+v, %v", invalid, response, err)
		}
	}
	response, err = identitygrpc.NewSessionCheckHandler(nil).CheckSession(t.Context(), request)
	if response != nil || status.Code(err) != codes.Unavailable {
		t.Fatalf("missing service = %+v, %v", response, err)
	}
}
