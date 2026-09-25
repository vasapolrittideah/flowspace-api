package http_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	identityhttp "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type fakeVerificationService struct {
	input       inbound.RequestEmailVerificationCodeInput
	verifyInput inbound.VerifyEmailInput
	calls       int
	verifyCalls int
	err         error
}

func (s *fakeVerificationService) RequestEmailVerificationCode(_ context.Context, input inbound.RequestEmailVerificationCodeInput) error {
	s.calls++
	s.input = input
	return s.err
}

func (s *fakeVerificationService) VerifyEmail(_ context.Context, input inbound.VerifyEmailInput) error {
	s.verifyCalls++
	s.verifyInput = input
	return s.err
}

type fakeAccessVerifier struct{ err error }

func (v fakeAccessVerifier) Verify(raw string) (outbound.AccessTokenIdentity, error) {
	if raw != "signed-token" || v.err != nil {
		return outbound.AccessTokenIdentity{}, outbound.ErrUnauthenticated
	}
	return outbound.AccessTokenIdentity{Subject: "token-subject", SessionID: "token-session"}, nil
}

func verificationContext() context.Context {
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1234}})
	return metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer signed-token"))
}

func TestRequestVerificationCodeHandler(t *testing.T) {
	service := &fakeVerificationService{}
	handler := identityhttp.NewIdentityHandler(&fakeSignupService{}, service, nil, fakeAccessVerifier{}, nil)
	response, err := handler.RequestEmailVerificationCode(verificationContext(), &identityv1.RequestEmailVerificationCodeRequest{})
	if err != nil || !response.GetAccepted() || service.calls != 1 || service.input != (inbound.RequestEmailVerificationCodeInput{
		Subject: "token-subject", SessionID: "token-session", Source: "192.0.2.1",
	}) {
		t.Fatalf("response = %+v, input = %+v, calls = %d, error = %v", response, service.input, service.calls, err)
	}
	for _, test := range []struct {
		name   string
		values []string
	}{
		{"missing bearer", nil},
		{"duplicate bearer", []string{"Bearer signed-token", "Bearer signed-token"}},
		{"malformed bearer", []string{"signed-token"}},
		{"invalid token", []string{"Bearer wrong-token"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := service.calls
			ctx := metadata.NewIncomingContext(verificationContext(), metadata.MD{"authorization": test.values})
			response, err := handler.RequestEmailVerificationCode(ctx, &identityv1.RequestEmailVerificationCodeRequest{})
			if response != nil || status.Code(err) != codes.Unauthenticated || service.calls != before {
				t.Fatalf("response = %+v, code = %s, calls = %d", response, status.Code(err), service.calls)
			}
		})
	}
	for _, test := range []struct {
		err  error
		want codes.Code
	}{
		{app.ErrEmailAlreadyVerified, codes.FailedPrecondition},
		{app.ErrRateLimited, codes.ResourceExhausted},
		{outbound.ErrUnauthenticated, codes.Unauthenticated},
		{errors.New("database unavailable"), codes.Unavailable},
		{context.Canceled, codes.Canceled},
	} {
		service.err = test.err
		response, err := handler.RequestEmailVerificationCode(verificationContext(), &identityv1.RequestEmailVerificationCodeRequest{})
		if response != nil || status.Code(err) != test.want {
			t.Fatalf("service error %v mapped to %s", test.err, status.Code(err))
		}
	}
}

func TestGeneratedVerificationCodeRESTRoute(t *testing.T) {
	service := &fakeVerificationService{}
	mux := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerServer(context.Background(), mux,
		identityhttp.NewIdentityHandler(&fakeSignupService{}, service, nil, fakeAccessVerifier{}, nil)); err != nil {
		t.Fatal(err)
	}
	handler := identityhttp.NewIdentityRequestHandler(mux, nil)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/email-verification-codes", strings.NewReader(`{}`))
	request.RemoteAddr = "192.0.2.1:1234"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer signed-token")
	request.Header.Set("Idempotency-Key", "ignored")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"accepted":true`) || service.input.Subject != "token-subject" ||
		strings.Contains(response.Body.String(), "token") || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("REST status = %d, body = %q, input = %+v", response.Code, response.Body.String(), service.input)
	}
}

func TestVerifyEmailHandler(t *testing.T) {
	service := &fakeVerificationService{}
	handler := identityhttp.NewIdentityHandler(&fakeSignupService{}, service, nil, fakeAccessVerifier{}, nil)
	response, err := handler.VerifyEmail(verificationContext(), &identityv1.VerifyEmailRequest{Code: "012345"})
	if err != nil || !response.GetEmailVerified() || service.verifyCalls != 1 || service.verifyInput != (inbound.VerifyEmailInput{
		Subject: "token-subject", SessionID: "token-session", Source: "192.0.2.1", Code: "012345",
	}) {
		t.Fatalf("response = %+v, input = %+v, calls = %d, error = %v", response, service.verifyInput, service.verifyCalls, err)
	}
	for _, test := range []struct {
		err  error
		want codes.Code
	}{
		{app.ErrInvalidVerificationCode, codes.InvalidArgument},
		{app.ErrRateLimited, codes.ResourceExhausted},
		{outbound.ErrUnauthenticated, codes.Unauthenticated},
		{errors.New("database unavailable"), codes.Unavailable},
	} {
		service.err = test.err
		_, err := handler.VerifyEmail(verificationContext(), &identityv1.VerifyEmailRequest{Code: "012345"})
		if status.Code(err) != test.want {
			t.Fatalf("service error %v mapped to %s", test.err, status.Code(err))
		}
	}
	before := service.verifyCalls
	ctx := metadata.NewIncomingContext(verificationContext(), metadata.MD{"authorization": {"Bearer signed-token", "Bearer signed-token"}})
	if _, err := handler.VerifyEmail(ctx, &identityv1.VerifyEmailRequest{Code: "012345"}); status.Code(err) != codes.Unauthenticated || service.verifyCalls != before {
		t.Fatalf("duplicate authorization = %v, calls = %d", err, service.verifyCalls)
	}
	if _, err := handler.VerifyEmail(verificationContext(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("nil request = %v", err)
	}
	withoutPeer := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer signed-token"))
	if _, err := handler.VerifyEmail(withoutPeer, &identityv1.VerifyEmailRequest{Code: "012345"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing source = %v", err)
	}
	withoutService := identityhttp.NewIdentityHandler(&fakeSignupService{}, nil, nil, fakeAccessVerifier{}, nil)
	if _, err := withoutService.VerifyEmail(verificationContext(), &identityv1.VerifyEmailRequest{Code: "012345"}); status.Code(err) != codes.Unavailable {
		t.Fatalf("missing service = %v", err)
	}
}

func TestGeneratedVerifyEmailRESTRoute(t *testing.T) {
	service := &fakeVerificationService{}
	mux := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerServer(context.Background(), mux,
		identityhttp.NewIdentityHandler(&fakeSignupService{}, service, nil, fakeAccessVerifier{}, nil)); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/email-verifications", strings.NewReader(`{"code":"012345"}`))
	request.RemoteAddr = "192.0.2.1:1234"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer signed-token")
	response := httptest.NewRecorder()
	identityhttp.NewIdentityRequestHandler(mux, nil).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"emailVerified":true`) ||
		service.verifyInput.Code != "012345" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("REST status = %d, body = %q, input = %+v", response.Code, response.Body.String(), service.verifyInput)
	}
}
