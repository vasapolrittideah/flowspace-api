package http_test

import (
	"context"
	"errors"
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

type fakeProviderLogin struct {
	input inbound.StartProviderLoginInput
	calls int
	err   error
}

func (s *fakeProviderLogin) StartProviderLogin(_ context.Context, input inbound.StartProviderLoginInput) (inbound.StartProviderLoginResult, error) {
	s.calls++
	s.input = input
	if s.err != nil {
		return inbound.StartProviderLoginResult{}, s.err
	}
	return inbound.StartProviderLoginResult{
		AuthorizationURL: "https://accounts.google.com/o/oauth2/v2/auth?state=state",
		AttemptToken:     "attempt-token",
		ExpiresAt:        time.Date(2026, 10, 1, 12, 10, 0, 0, time.UTC),
	}, nil
}

func (s *fakeProviderLogin) CompleteProviderCallback(context.Context, inbound.CompleteProviderCallbackInput) (string, error) {
	return "", errors.New("not used")
}

func TestStartProviderLoginREST(t *testing.T) {
	service := &fakeProviderLogin{}
	mux := runtime.NewServeMux()
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil).WithProviderLogin(service)
	if err := identityv1.RegisterIdentityServiceHandlerServer(context.Background(), mux, handler); err != nil {
		t.Fatal(err)
	}
	rest := identityhttp.NewIdentityRequestHandler(mux, nil)
	request := func(body string) *http.Request {
		r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/provider-login-attempts", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = "192.0.2.1:1234"
		return r
	}

	response := httptest.NewRecorder()
	rest.ServeHTTP(response, request(`{"provider":"PROVIDER_GITHUB"}`))
	body := response.Body.String()
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" ||
		service.input != (inbound.StartProviderLoginInput{Provider: "github", Source: "192.0.2.1"}) ||
		!strings.Contains(body, `"attemptToken":"attempt-token"`) || !strings.Contains(body, `"attemptExpiresAt":"2026-10-01T12:10:00Z"`) ||
		!strings.Contains(body, `"authorizationUrl":"https://accounts.google.com/o/oauth2/v2/auth?state=state"`) ||
		strings.Contains(body, "accessToken") || strings.Contains(body, "refreshToken") {
		t.Fatalf("start response = %d %s", response.Code, body)
	}

	response = httptest.NewRecorder()
	rest.ServeHTTP(response, request(`{"provider":"PROVIDER_OKTA"}`))
	if response.Code != http.StatusBadRequest || service.calls != 1 {
		t.Fatalf("unknown provider status = %d, calls = %d", response.Code, service.calls)
	}

	response = httptest.NewRecorder()
	rest.ServeHTTP(response, request(`{"provider":"PROVIDER_GOOGLE","redirectUri":"https://attacker.example","subject":"victim"}`))
	if response.Code != http.StatusOK || service.input != (inbound.StartProviderLoginInput{Provider: "google", Source: "192.0.2.1"}) {
		t.Fatalf("client-selected fields reached the service: %d %+v", response.Code, service.input)
	}
}

func TestStartProviderLoginRESTRejectsIdempotencyKey(t *testing.T) {
	service := &fakeProviderLogin{}
	mux := runtime.NewServeMux()
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil).WithProviderLogin(service)
	if err := identityv1.RegisterIdentityServiceHandlerServer(context.Background(), mux, handler); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/provider-login-attempts", strings.NewReader(`{"provider":"PROVIDER_GOOGLE"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header["Idempotency-Key"] = []string{""}
	request.RemoteAddr = "192.0.2.1:1234"
	response := httptest.NewRecorder()
	identityhttp.NewIdentityRequestHandler(mux, nil).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || service.calls != 0 {
		t.Fatalf("idempotency response = %d, calls = %d", response.Code, service.calls)
	}
}

func TestStartProviderLoginRPCErrorMapping(t *testing.T) {
	service := &fakeProviderLogin{}
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil).WithProviderLogin(service)
	if _, err := handler.StartProviderLogin(context.Background(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing request error = %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("idempotency-key", "retry"))
	if _, err := handler.StartProviderLogin(ctx, &identityv1.StartProviderLoginRequest{Provider: identityv1.Provider_PROVIDER_GOOGLE}); status.Code(err) != codes.InvalidArgument || service.calls != 0 {
		t.Fatalf("idempotency error = %v", err)
	}
	requestCtx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1234}})
	if _, err := handler.StartProviderLogin(requestCtx, &identityv1.StartProviderLoginRequest{}); status.Code(err) != codes.InvalidArgument || service.calls != 0 {
		t.Fatalf("unspecified provider error = %v", err)
	}
	for _, test := range []struct {
		err  error
		want codes.Code
	}{
		{domain.ErrInvalidProvider, codes.InvalidArgument},
		{app.ErrRateLimited, codes.ResourceExhausted},
		{app.ErrLimitUnavailable, codes.Unavailable},
		{app.ErrProviderLoginUnavailable, codes.Unavailable},
		{context.DeadlineExceeded, codes.DeadlineExceeded},
		{errors.New("database down"), codes.Unavailable},
	} {
		service.err = test.err
		_, err := handler.StartProviderLogin(requestCtx, &identityv1.StartProviderLoginRequest{Provider: identityv1.Provider_PROVIDER_GOOGLE})
		if status.Code(err) != test.want || strings.Contains(status.Convert(err).Message(), "database") {
			t.Fatalf("error %v mapped to %v; want %s", test.err, err, test.want)
		}
	}
	withoutService := identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil)
	if _, err := withoutService.StartProviderLogin(requestCtx, &identityv1.StartProviderLoginRequest{Provider: identityv1.Provider_PROVIDER_GOOGLE}); status.Code(err) != codes.Unavailable {
		t.Fatalf("missing service error = %v", err)
	}
}
