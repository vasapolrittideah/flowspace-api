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
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
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
	for _, field := range []string{`"subject":"subject"`, `"emailVerified":true`, `"accessToken":"access"`, `"accessTokenExpiresAt"`, `"refreshTokenExpiresAt"`, `"sessionExpiresAt"`} {
		if !strings.Contains(response.Body.String(), field) {
			t.Fatalf("REST login response lacks %s", field)
		}
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

type fakeRefreshSession struct {
	token string
	calls int
	err   error
}

func (s *fakeRefreshSession) RefreshSession(_ context.Context, token string) (inbound.RefreshSessionResult, error) {
	s.calls++
	s.token = token
	if s.err != nil {
		return inbound.RefreshSessionResult{}, s.err
	}
	now := time.Now()
	return inbound.RefreshSessionResult{
		AccessToken: "new-access", RefreshToken: "new-refresh",
		AccessTokenExpiresAt: now.Add(10 * time.Minute), RefreshTokenExpiresAt: now.Add(30 * 24 * time.Hour), SessionExpiresAt: now.Add(90 * 24 * time.Hour),
	}, nil
}

func TestRefreshSessionRESTAndRPC(t *testing.T) {
	service := &fakeRefreshSession{}
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil).WithRefreshSession(service)
	if _, err := handler.RefreshSession(context.Background(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("nil request = %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("idempotency-key", "retry"))
	if _, err := handler.RefreshSession(ctx, &identityv1.RefreshSessionRequest{RefreshToken: "old"}); status.Code(err) != codes.InvalidArgument || service.calls != 0 {
		t.Fatalf("idempotency = %v", err)
	}
	mux := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerServer(context.Background(), mux, handler); err != nil {
		t.Fatal(err)
	}
	rest := identityhttp.NewIdentityRequestHandler(mux, nil)
	request := func() *http.Request {
		r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/session-refreshes", strings.NewReader(`{"refreshToken":"old-secret"}`))
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = "192.0.2.1:1234"
		return r
	}
	response := httptest.NewRecorder()
	rest.ServeHTTP(response, request())
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || service.token != "old-secret" || !strings.Contains(response.Body.String(), `"refreshToken":"new-refresh"`) {
		t.Fatalf("REST success = %d, %s", response.Code, response.Body.String())
	}
	keyRequest := request()
	keyRequest.Header["Idempotency-Key"] = []string{""}
	response = httptest.NewRecorder()
	rest.ServeHTTP(response, keyRequest)
	if response.Code != http.StatusBadRequest || service.calls != 1 {
		t.Fatalf("REST idempotency = %d", response.Code)
	}
}

func TestRefreshSessionSafeFailures(t *testing.T) {
	service := &fakeRefreshSession{}
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil).WithRefreshSession(service)
	mux := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerServer(context.Background(), mux, handler); err != nil {
		t.Fatal(err)
	}
	rest := identityhttp.NewIdentityRequestHandler(mux, nil)
	for _, test := range []struct {
		err  error
		want codes.Code
	}{
		{app.ErrInvalidRefreshToken, codes.InvalidArgument},
		{app.ErrUnauthenticatedRefresh, codes.Unauthenticated},
		{app.ErrRefreshUnavailable, codes.Unavailable},
	} {
		service.err = test.err
		if _, err := handler.RefreshSession(context.Background(), &identityv1.RefreshSessionRequest{RefreshToken: "old-secret"}); status.Code(err) != test.want || strings.Contains(err.Error(), "old-secret") {
			t.Fatalf("RPC error = %v", err)
		}
		request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/session-refreshes", strings.NewReader(`{"refreshToken":"old-secret"}`))
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = "192.0.2.1:1234"
		response := httptest.NewRecorder()
		rest.ServeHTTP(response, request)
		if response.Code != runtime.HTTPStatusFromCode(test.want) || strings.Contains(response.Body.String(), "old-secret") {
			t.Fatalf("REST error = %d, %s", response.Code, response.Body.String())
		}
	}
}

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
