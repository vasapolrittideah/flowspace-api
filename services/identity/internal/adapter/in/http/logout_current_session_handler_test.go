package http_test

import (
	"context"
	"errors"
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

type fakeCurrentLogout struct {
	input inbound.LogoutCurrentSessionInput
	calls int
	err   error
}

func (s *fakeCurrentLogout) LogoutCurrentSession(_ context.Context, input inbound.LogoutCurrentSessionInput) error {
	s.calls++
	s.input = input
	return s.err
}

func TestLogoutCurrentSessionRPC(t *testing.T) {
	service := &fakeCurrentLogout{}
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, nil, fakeAccessVerifier{}, nil).WithCurrentSessionLogout(service)
	response, err := handler.LogoutCurrentSession(verificationContext(), &identityv1.LogoutCurrentSessionRequest{})
	if err != nil || !response.GetRevoked() || service.calls != 1 || service.input != (inbound.LogoutCurrentSessionInput{Subject: "token-subject", SessionID: "token-session"}) {
		t.Fatalf("logout = %+v, %v, service = %+v", response, err, service)
	}
	for _, test := range []struct {
		name   string
		values []string
	}{
		{"missing", nil},
		{"duplicate", []string{"Bearer signed-token", "Bearer signed-token"}},
		{"malformed", []string{"signed-token"}},
		{"invalid token", []string{"Bearer invalid-token"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{"authorization": test.values})
			response, err := handler.LogoutCurrentSession(ctx, &identityv1.LogoutCurrentSessionRequest{})
			if response != nil || status.Code(err) != codes.Unauthenticated || service.calls != 1 {
				t.Fatalf("invalid bearer = %+v, %v, calls = %d", response, err, service.calls)
			}
		})
	}
	if _, err := handler.LogoutCurrentSession(verificationContext(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("nil request = %v", err)
	}
	for _, test := range []struct {
		err  error
		want codes.Code
	}{
		{outbound.ErrUnauthenticated, codes.Unauthenticated},
		{app.ErrCurrentLogoutUnavailable, codes.Unavailable},
		{errors.New("secret database detail"), codes.Unavailable},
		{context.Canceled, codes.Canceled},
	} {
		service.err = test.err
		response, err := handler.LogoutCurrentSession(verificationContext(), &identityv1.LogoutCurrentSessionRequest{})
		if response != nil || status.Code(err) != test.want || strings.Contains(err.Error(), "secret database detail") {
			t.Fatalf("failure = %+v, %v", response, err)
		}
	}
}

func TestLogoutCurrentSessionREST(t *testing.T) {
	service := &fakeCurrentLogout{}
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, nil, fakeAccessVerifier{}, nil).WithCurrentSessionLogout(service)
	mux := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerServer(context.Background(), mux, handler); err != nil {
		t.Fatal(err)
	}
	rest := identityhttp.NewIdentityRequestHandler(mux, nil)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/session-logouts", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer signed-token")
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "192.0.2.1:1234"
	response := httptest.NewRecorder()
	rest.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), `"revoked":true`) || service.calls != 1 {
		t.Fatalf("REST success = %d, %s, calls = %d", response.Code, response.Body.String(), service.calls)
	}
}
