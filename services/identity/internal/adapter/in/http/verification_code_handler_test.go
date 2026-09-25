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
	input inbound.RequestEmailVerificationCodeInput
	calls int
	err   error
}

func (s *fakeVerificationService) RequestEmailVerificationCode(_ context.Context, input inbound.RequestEmailVerificationCodeInput) error {
	s.calls++
	s.input = input
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
	handler := identityhttp.NewIdentityHandler(&fakeSignupService{}, service, fakeAccessVerifier{}, nil)
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
		identityhttp.NewIdentityHandler(&fakeSignupService{}, service, fakeAccessVerifier{}, nil)); err != nil {
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
