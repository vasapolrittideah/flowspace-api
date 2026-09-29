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
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	identityhttp "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

type fakeRequestPasswordResetCodeService struct {
	input inbound.RequestPasswordResetCodeInput
	err   error
}

func (f *fakeRequestPasswordResetCodeService) RequestPasswordResetCode(_ context.Context, input inbound.RequestPasswordResetCodeInput) error {
	f.input = input
	return f.err
}

func TestRequestPasswordResetCodeHandlerReturnsGenericResponse(t *testing.T) {
	service := &fakeRequestPasswordResetCodeService{}
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil).WithRequestPasswordResetCode(service)
	response, err := handler.RequestPasswordResetCode(claimCodeContext(), &identityv1.RequestPasswordResetCodeRequest{Email: "User@example.com"})
	if err != nil || !response.GetAccepted() || service.input.Email != "User@example.com" || service.input.Source == "" {
		t.Fatalf("response=%v error=%v input=%+v", response, err, service.input)
	}
	for _, test := range []struct {
		err  error
		want codes.Code
	}{
		{domain.ErrInvalidEmail, codes.InvalidArgument},
		{app.ErrRateLimited, codes.ResourceExhausted},
		{app.ErrLimitUnavailable, codes.Unavailable},
		{context.Canceled, codes.Canceled},
		{errors.New("database unavailable"), codes.Unavailable},
	} {
		service.err = test.err
		if _, err := handler.RequestPasswordResetCode(claimCodeContext(), &identityv1.RequestPasswordResetCodeRequest{Email: "User@example.com"}); status.Code(err) != test.want {
			t.Fatalf("error %v mapped to %s", test.err, status.Code(err))
		}
	}
	if _, err := handler.RequestPasswordResetCode(claimCodeContext(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("nil request=%v", err)
	}
	if _, err := handler.RequestPasswordResetCode(context.Background(), &identityv1.RequestPasswordResetCodeRequest{Email: "User@example.com"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing source=%v", err)
	}
	withoutService := identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil)
	if _, err := withoutService.RequestPasswordResetCode(claimCodeContext(), &identityv1.RequestPasswordResetCodeRequest{Email: "User@example.com"}); status.Code(err) != codes.Unavailable {
		t.Fatalf("missing service=%v", err)
	}
}

func TestRequestPasswordResetCodeRESTRouteIsPublic(t *testing.T) {
	service := &fakeRequestPasswordResetCodeService{}
	mux := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerServer(context.Background(), mux,
		identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil).WithRequestPasswordResetCode(service)); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/password-reset-codes", strings.NewReader(`{"email":"User@example.com"}`))
	request.RemoteAddr = "192.0.2.5:1234"
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	identityhttp.NewIdentityRequestHandler(mux, nil).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"accepted":true`) ||
		service.input.Email != "User@example.com" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d body=%q input=%+v", response.Code, response.Body.String(), service.input)
	}
}

type fakePasswordResetService struct {
	input inbound.ResetPasswordInput
	err   error
}

func (f *fakePasswordResetService) ResetPassword(_ context.Context, input inbound.ResetPasswordInput) error {
	f.input = input
	return f.err
}

func TestResetPasswordRESTRouteIsPublic(t *testing.T) {
	service := &fakePasswordResetService{}
	mux := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerServer(context.Background(), mux,
		identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil).WithPasswordReset(service)); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/password-resets", strings.NewReader(`{"email":"User@example.com","code":"012345","newPassword":"fresh password 123"}`))
	request.RemoteAddr = "192.0.2.5:1234"
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	identityhttp.NewIdentityRequestHandler(mux, nil).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"passwordChanged":true`) ||
		!strings.Contains(response.Body.String(), `"sessionsRevoked":true`) || service.input.Code != "012345" ||
		service.input.NewPassword != "fresh password 123" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d body=%q input=%+v", response.Code, response.Body.String(), service.input)
	}
}

func TestResetPasswordMapsSafeErrors(t *testing.T) {
	service := &fakePasswordResetService{}
	handler := identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil).WithPasswordReset(service)
	request := &identityv1.ResetPasswordRequest{Email: "User@example.com", Code: "012345", NewPassword: "fresh password 123"}
	for _, test := range []struct {
		err  error
		want codes.Code
	}{
		{app.ErrInvalidPasswordResetCode, codes.InvalidArgument},
		{domain.ErrInvalidPassword, codes.InvalidArgument},
		{domain.ErrInvalidEmail, codes.InvalidArgument},
		{app.ErrRateLimited, codes.ResourceExhausted},
		{context.Canceled, codes.Canceled},
		{errors.New("database unavailable"), codes.Unavailable},
	} {
		service.err = test.err
		if _, err := handler.ResetPassword(claimCodeContext(), request); status.Code(err) != test.want {
			t.Fatalf("error %v mapped to %s", test.err, status.Code(err))
		}
	}
	if _, err := handler.ResetPassword(claimCodeContext(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("nil request=%v", err)
	}
	if _, err := handler.ResetPassword(context.Background(), request); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing source=%v", err)
	}
	if _, err := identityhttp.NewIdentityHandler(nil, nil, nil, nil, nil, nil).ResetPassword(claimCodeContext(), request); status.Code(err) != codes.Unavailable {
		t.Fatalf("missing service=%v", err)
	}
}
