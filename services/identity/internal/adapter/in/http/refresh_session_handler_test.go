package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	identityhttp "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

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
