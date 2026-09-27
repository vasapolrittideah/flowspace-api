package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	identityhttp "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type fakeAllLogout struct {
	input inbound.LogoutAllSessionsInput
	calls int
	err   error
}

func (s *fakeAllLogout) LogoutAllSessions(_ context.Context, input inbound.LogoutAllSessionsInput) error {
	s.calls++
	s.input = input
	return s.err
}

func TestLogoutAllSessionsRPC(t *testing.T) {
	service := &fakeAllLogout{}
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, nil, fakeAccessVerifier{}, nil).WithAllSessionLogout(service)
	response, err := handler.LogoutAllSessions(verificationContext(), &identityv1.LogoutAllSessionsRequest{})
	if err != nil || !response.GetRevoked() || service.calls != 1 || service.input != (inbound.LogoutAllSessionsInput{Subject: "token-subject", SessionID: "token-session"}) {
		t.Fatalf("logout = %+v, %v, service = %+v", response, err, service)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{"authorization": {"Bearer signed-token", "Bearer signed-token"}})
	if _, err := handler.LogoutAllSessions(ctx, &identityv1.LogoutAllSessionsRequest{}); status.Code(err) != codes.Unauthenticated || service.calls != 1 {
		t.Fatalf("duplicate bearer = %v, calls = %d", err, service.calls)
	}
	if _, err := handler.LogoutAllSessions(verificationContext(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("nil request = %v", err)
	}
	service.err = outbound.ErrUnauthenticated
	if _, err := handler.LogoutAllSessions(verificationContext(), &identityv1.LogoutAllSessionsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("inactive session = %v", err)
	}
	service.err = app.ErrAllLogoutUnavailable
	if _, err := handler.LogoutAllSessions(verificationContext(), &identityv1.LogoutAllSessionsRequest{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("database failure = %v", err)
	}
}

func TestLogoutAllSessionsREST(t *testing.T) {
	service := &fakeAllLogout{}
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, nil, fakeAccessVerifier{}, nil).WithAllSessionLogout(service)
	mux := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerServer(context.Background(), mux, handler); err != nil {
		t.Fatal(err)
	}
	rest := identityhttp.NewIdentityRequestHandler(mux, nil)
	request := func(body string) *http.Request {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/account-session-logouts", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer signed-token")
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = "192.0.2.1:1234"
		return r
	}
	response := httptest.NewRecorder()
	rest.ServeHTTP(response, request(`{}`))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), `"revoked":true`) || service.calls != 1 {
		t.Fatalf("REST success = %d, %s, calls = %d", response.Code, response.Body.String(), service.calls)
	}
	response = httptest.NewRecorder()
	rest.ServeHTTP(response, request(`{"subject":"other-subject"}`))
	if response.Code != http.StatusOK || service.calls != 2 || service.input.Subject != "token-subject" {
		t.Fatalf("client-chosen subject = %d, service = %+v", response.Code, service)
	}
}
