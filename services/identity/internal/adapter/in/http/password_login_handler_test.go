package http_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	identityhttp "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

type fakePasswordLogin struct {
	input inbound.CreatePasswordSessionInput
	calls int
	err   error
}

func (s *fakePasswordLogin) CreatePasswordSession(_ context.Context, input inbound.CreatePasswordSessionInput) (inbound.CreatePasswordSessionResult, error) {
	s.calls++
	s.input = input
	if s.err != nil {
		return inbound.CreatePasswordSessionResult{}, s.err
	}
	now := time.Now()
	return inbound.CreatePasswordSessionResult{Subject: "subject", EmailVerified: true, AccessToken: "access", RefreshToken: "refresh", AccessTokenExpiresAt: now.Add(time.Minute), RefreshTokenExpiresAt: now.Add(time.Hour), SessionExpiresAt: now.Add(24 * time.Hour)}, nil
}

func TestPasswordLoginRESTAndSafeFailures(t *testing.T) {
	service := &fakePasswordLogin{}
	mux := runtime.NewServeMux()
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil).WithPasswordLogin(service)
	if err := identityv1.RegisterIdentityServiceHandlerServer(context.Background(), mux, handler); err != nil {
		t.Fatal(err)
	}
	rest := identityhttp.NewIdentityRequestHandler(mux, nil)
	request := func() *http.Request {
		r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/password-sessions", strings.NewReader(`{"email":"User@example.com","password":"secret-password"}`))
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = "192.0.2.1:1234"
		return r
	}
	response := httptest.NewRecorder()
	rest.ServeHTTP(response, request())
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || service.input.Source != "192.0.2.1" || !strings.Contains(response.Body.String(), `"refreshToken":"refresh"`) {
		t.Fatalf("REST login status = %d", response.Code)
	}
	service.err = app.ErrInvalidCredentials
	response = httptest.NewRecorder()
	rest.ServeHTTP(response, request())
	if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), "secret-password") || strings.Contains(response.Body.String(), "User@example.com") {
		t.Fatalf("unsafe failure status = %d", response.Code)
	}
	keyRequest := request()
	keyRequest.Header["Idempotency-Key"] = []string{""}
	response = httptest.NewRecorder()
	rest.ServeHTTP(response, keyRequest)
	if response.Code != http.StatusBadRequest || service.calls != 2 {
		t.Fatalf("idempotency response = %d, calls = %d", response.Code, service.calls)
	}
}

func TestPasswordLoginRPCErrorMapping(t *testing.T) {
	service := &fakePasswordLogin{}
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil).WithPasswordLogin(service)
	if _, err := handler.CreatePasswordSession(context.Background(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing request error = %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("idempotency-key", "retry"))
	if _, err := handler.CreatePasswordSession(ctx, &identityv1.CreatePasswordSessionRequest{}); status.Code(err) != codes.InvalidArgument || service.calls != 0 {
		t.Fatalf("idempotency error = %v", err)
	}
	for _, test := range []struct {
		err  error
		want codes.Code
	}{
		{domain.ErrInvalidEmail, codes.InvalidArgument},
		{domain.ErrInvalidPassword, codes.InvalidArgument},
		{app.ErrRateLimited, codes.ResourceExhausted},
		{app.ErrLimitUnavailable, codes.Unavailable},
		{app.ErrInvalidCredentials, codes.Unauthenticated},
		{app.ErrLoginUnavailable, codes.Unavailable},
	} {
		service.err = test.err
		requestCtx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1234}})
		if _, err := handler.CreatePasswordSession(requestCtx, &identityv1.CreatePasswordSessionRequest{Email: "User@example.com", Password: "secret-password"}); status.Code(err) != test.want {
			t.Fatalf("error %v mapped to %s; want %s", test.err, status.Code(err), test.want)
		}
	}
	withoutService := identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil)
	requestCtx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1234}})
	if _, err := withoutService.CreatePasswordSession(requestCtx, &identityv1.CreatePasswordSessionRequest{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("missing service error = %v", err)
	}
}
