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
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	identityhttp "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

type fakeClaimCodeService struct {
	input inbound.RequestClaimCodeInput
	calls int
	err   error
}

func (s *fakeClaimCodeService) RequestUnverifiedAccountClaimCode(_ context.Context, input inbound.RequestClaimCodeInput) error {
	s.calls++
	s.input = input
	return s.err
}

func claimCodeContext() context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.5"), Port: 1234}})
}

func TestClaimCodeHandlerIsPublicAndMapsFailures(t *testing.T) {
	service := &fakeClaimCodeService{}
	handler := identityhttp.NewIdentityHandler(&fakeSignupService{}, nil, service, nil, nil, nil)
	request := &identityv1.RequestUnverifiedAccountClaimCodeRequest{Email: "User@example.com"}
	response, err := handler.RequestUnverifiedAccountClaimCode(claimCodeContext(), request)
	if err != nil || !response.GetAccepted() || service.calls != 1 || service.input != (inbound.RequestClaimCodeInput{
		Email: "User@example.com", Source: "192.0.2.5",
	}) {
		t.Fatalf("response = %+v, input = %+v, calls = %d, error = %v", response, service.input, service.calls, err)
	}
	for _, test := range []struct {
		err  error
		want codes.Code
	}{
		{domain.ErrInvalidEmail, codes.InvalidArgument},
		{app.ErrRateLimited, codes.ResourceExhausted},
		{context.Canceled, codes.Canceled},
		{errors.New("database unavailable"), codes.Unavailable},
	} {
		service.err = test.err
		if _, err := handler.RequestUnverifiedAccountClaimCode(claimCodeContext(), request); status.Code(err) != test.want {
			t.Fatalf("service error %v mapped to %s", test.err, status.Code(err))
		}
	}
	if _, err := handler.RequestUnverifiedAccountClaimCode(claimCodeContext(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("nil request = %v", err)
	}
	if _, err := handler.RequestUnverifiedAccountClaimCode(context.Background(), request); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing source = %v", err)
	}
	withoutService := identityhttp.NewIdentityHandler(&fakeSignupService{}, nil, nil, nil, nil, nil)
	if _, err := withoutService.RequestUnverifiedAccountClaimCode(claimCodeContext(), request); status.Code(err) != codes.Unavailable {
		t.Fatalf("missing service = %v", err)
	}
}

func TestGeneratedClaimCodeRESTRoute(t *testing.T) {
	service := &fakeClaimCodeService{}
	mux := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerServer(context.Background(), mux,
		identityhttp.NewIdentityHandler(&fakeSignupService{}, nil, service, nil, nil, nil)); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/unverified-account-claim-codes", strings.NewReader(`{"email":"User@example.com"}`))
	request.RemoteAddr = "192.0.2.5:1234"
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	identityhttp.NewIdentityRequestHandler(mux, nil).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"accepted":true`) ||
		service.input.Email != "User@example.com" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("REST status = %d, body = %q, input = %+v", response.Code, response.Body.String(), service.input)
	}
}
