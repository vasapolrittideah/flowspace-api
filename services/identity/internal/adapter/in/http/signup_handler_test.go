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
	"google.golang.org/genproto/googleapis/rpc/errdetails"
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

type fakeSignupService struct {
	called int
	source string
	err    error
}

func (s *fakeSignupService) CreateAccount(_ context.Context, input inbound.CreateAccountInput) (inbound.CreateAccountResult, error) {
	s.called++
	s.source = input.Source
	if s.err != nil {
		return inbound.CreateAccountResult{}, s.err
	}
	return inbound.CreateAccountResult{Subject: "subject", AccessToken: "access", RefreshToken: "refresh", AccessTokenExpiresAt: time.Now().Add(time.Minute)}, nil
}

func TestSignupHandler(t *testing.T) {
	service := &fakeSignupService{}
	handler := identityhttp.NewIdentityHandler(service, nil, nil, nil, nil)
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1234}})
	request := &identityv1.CreateAccountRequest{Email: "User@example.com", Password: "correct horse battery staple"}
	response, err := handler.CreateAccount(ctx, request)
	if err != nil || response.EmailVerified == nil || response.GetEmailVerified() || response.GetSubject() != "subject" || response.GetRefreshToken() != "refresh" || service.source != "192.0.2.1" {
		t.Fatalf("response = %+v, source = %q, error = %v", response, service.source, err)
	}
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("idempotency-key", "retry"))
	if response, err := handler.CreateAccount(ctx, request); response != nil || status.Code(err) != codes.InvalidArgument || service.called != 1 {
		t.Fatalf("idempotency response = %+v, error = %v", response, err)
	}
	service.err = outbound.ErrAccountExists
	ctx = peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1234}})
	if response, err := handler.CreateAccount(ctx, request); response != nil || status.Code(err) != codes.AlreadyExists {
		t.Fatalf("duplicate response = %+v, error = %v", response, err)
	}
}

func TestSignupHandlerRejectsInvalidRequestsAndMapsFailures(t *testing.T) {
	service := &fakeSignupService{}
	handler := identityhttp.NewIdentityHandler(service, nil, nil, nil, nil)
	request := &identityv1.CreateAccountRequest{Email: "User@example.com", Password: "correct horse battery staple"}
	peerContext := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1234}})
	withPeer := func() context.Context { return peerContext }
	withoutPeer := context.Background
	withForwarded := func() context.Context {
		return metadata.NewIncomingContext(peerContext, metadata.Pairs("forwarded", "for=192.0.2.2"))
	}
	for _, test := range []struct {
		name       string
		ctx        func() context.Context
		request    *identityv1.CreateAccountRequest
		serviceErr error
		want       codes.Code
	}{
		{"missing request", withPeer, nil, nil, codes.InvalidArgument},
		{"missing peer", withoutPeer, request, nil, codes.InvalidArgument},
		{"forbidden forwarded header", withForwarded, request, nil, codes.InvalidArgument},
		{"bad email", withPeer, request, domain.ErrInvalidEmail, codes.InvalidArgument},
		{"rate limited", withPeer, request, app.ErrRateLimited, codes.ResourceExhausted},
		{"canceled", withPeer, request, context.Canceled, codes.Canceled},
		{"unavailable", withPeer, request, errors.New("database down"), codes.Unavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			service.err = test.serviceErr
			response, err := handler.CreateAccount(test.ctx(), test.request)
			if response != nil || status.Code(err) != test.want {
				t.Fatalf("response = %+v, code = %s; want %s", response, status.Code(err), test.want)
			}
			if test.name == "bad email" {
				details := status.Convert(err).Details()
				if len(details) != 1 {
					t.Fatalf("field details = %v", details)
				}
				badRequest, ok := details[0].(*errdetails.BadRequest)
				if !ok || len(badRequest.GetFieldViolations()) != 1 || badRequest.GetFieldViolations()[0].GetField() != "email" {
					t.Fatalf("field details = %v", details)
				}
			}
		})
	}
}

func TestSignupRequestHandlerRejectsIdempotencyKey(t *testing.T) {
	called := false
	handler := identityhttp.NewIdentityRequestHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }), nil)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/accounts", nil)
	request.Header["Idempotency-Key"] = []string{""}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || called || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("response status = %d, called = %t", response.Code, called)
	}
}

func TestSignupRequestHandlerValidatesSource(t *testing.T) {
	called := false
	handler := identityhttp.NewIdentityRequestHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusAccepted)
	}), nil)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/accounts", nil)
	request.RemoteAddr = "not-an-address"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || called {
		t.Fatalf("invalid source status = %d, called = %t", response.Code, called)
	}
	request.RemoteAddr = "192.0.2.1:1234"
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !called {
		t.Fatalf("valid source status = %d, called = %t", response.Code, called)
	}
}

func TestGeneratedSignupRESTRoute(t *testing.T) {
	service := &fakeSignupService{}
	mux := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerServer(context.Background(), mux, identityhttp.NewIdentityHandler(service, nil, nil, nil, nil)); err != nil {
		t.Fatal(err)
	}
	handler := identityhttp.NewIdentityRequestHandler(mux, nil)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/accounts",
		strings.NewReader(`{"email":"User@example.com","password":"correct horse battery staple"}`))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "192.0.2.1:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || service.source != "192.0.2.1" || !strings.Contains(response.Body.String(), `"emailVerified":false`) {
		t.Fatalf("generated REST response = %d %s, source = %q", response.Code, response.Body.String(), service.source)
	}
}
