package http_test

import (
	"context"
	"errors"
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
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

type fakeAccountClaimService struct {
	input inbound.ClaimAccountInput
	calls int
	err   error
}

func (s *fakeAccountClaimService) ClaimUnverifiedAccount(_ context.Context, input inbound.ClaimAccountInput) (inbound.ClaimAccountResult, error) {
	s.calls++
	s.input = input
	if s.err != nil {
		return inbound.ClaimAccountResult{}, s.err
	}
	return inbound.ClaimAccountResult{Subject: "new-subject", AccessToken: "access", RefreshToken: "refresh", AccessTokenExpiresAt: time.Now().Add(time.Minute)}, nil
}

func TestClaimAccountHandlerReturnsNewVerifiedSession(t *testing.T) {
	service := &fakeAccountClaimService{}
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, service, nil, nil)
	request := &identityv1.ClaimUnverifiedAccountRequest{Email: "User@example.com", Code: "012345", NewPassword: "correct horse battery staple"}
	response, err := handler.ClaimUnverifiedAccount(claimCodeContext(), request)
	if err != nil || response.GetSubject() != "new-subject" || !response.GetEmailVerified() || response.GetAccessToken() != "access" || response.GetRefreshToken() != "refresh" || response.GetAccessTokenExpiresAt() == nil ||
		service.input != (inbound.ClaimAccountInput{Email: request.GetEmail(), Code: request.GetCode(), NewPassword: request.GetNewPassword(), Source: "192.0.2.5"}) {
		t.Fatalf("response = %+v, input = %+v, error = %v", response, service.input, err)
	}
	ctx := metadata.NewIncomingContext(claimCodeContext(), metadata.Pairs("idempotency-key", "retry"))
	if response, err := handler.ClaimUnverifiedAccount(ctx, request); response != nil || status.Code(err) != codes.InvalidArgument || service.calls != 1 {
		t.Fatalf("idempotency response = %+v, calls = %d, error = %v", response, service.calls, err)
	}
	service.err = app.ErrInvalidClaimCode
	if response, err := handler.ClaimUnverifiedAccount(claimCodeContext(), request); response != nil || status.Code(err) != codes.InvalidArgument || status.Convert(err).Message() != "invalid claim code" {
		t.Fatalf("invalid claim response = %+v, error = %v", response, err)
	}
	service.err = domain.ErrInvalidPassword
	if _, err := handler.ClaimUnverifiedAccount(claimCodeContext(), request); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("invalid password = %v", err)
	}
}

func TestClaimAccountRESTRejectsIdempotencyKey(t *testing.T) {
	service := &fakeAccountClaimService{}
	mux := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerServer(context.Background(), mux,
		identityhttp.NewIdentityHandler(nil, nil, nil, service, nil, nil)); err != nil {
		t.Fatal(err)
	}
	handler := identityhttp.NewIdentityRequestHandler(mux, nil)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/unverified-account-claims",
		strings.NewReader(`{"email":"User@example.com","code":"012345","newPassword":"correct horse battery staple"}`))
	request.RemoteAddr = "192.0.2.5:1234"
	request.Header.Set("Content-Type", "application/json")
	request.Header["Idempotency-Key"] = []string{""}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || service.calls != 0 {
		t.Fatalf("idempotency status = %d, calls = %d", response.Code, service.calls)
	}
	request.Header.Del("Idempotency-Key")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"emailVerified":true`) || service.calls != 1 {
		t.Fatalf("REST status = %d, body = %q, calls = %d", response.Code, response.Body.String(), service.calls)
	}
}

func TestClaimAccountHandlerMapsSafeErrors(t *testing.T) {
	service := &fakeAccountClaimService{}
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, service, nil, nil)
	request := &identityv1.ClaimUnverifiedAccountRequest{Email: "User@example.com", Code: "012345", NewPassword: "correct horse battery staple"}
	for _, test := range []struct {
		err  error
		want codes.Code
	}{
		{domain.ErrInvalidEmail, codes.InvalidArgument},
		{domain.ErrInvalidPassword, codes.InvalidArgument},
		{app.ErrInvalidClaimCode, codes.InvalidArgument},
		{app.ErrRateLimited, codes.ResourceExhausted},
		{context.Canceled, codes.Canceled},
		{errors.New("database unavailable"), codes.Unavailable},
	} {
		service.err = test.err
		if _, err := handler.ClaimUnverifiedAccount(claimCodeContext(), request); status.Code(err) != test.want {
			t.Fatalf("error %v mapped to %s, want %s", test.err, status.Code(err), test.want)
		}
	}
	if _, err := handler.ClaimUnverifiedAccount(claimCodeContext(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("nil request = %v", err)
	}
	if _, err := handler.ClaimUnverifiedAccount(context.Background(), request); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing source = %v", err)
	}
	withoutService := identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil)
	if _, err := withoutService.ClaimUnverifiedAccount(claimCodeContext(), request); status.Code(err) != codes.Unavailable {
		t.Fatalf("missing service = %v", err)
	}
}
