package http

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
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func TestReauthenticationRPCError(t *testing.T) {
	if err := ReauthenticationRPCError(nil); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		err  error
		code codes.Code
	}{
		{app.ErrReauthenticationRequired, codes.PermissionDenied},
		{outbound.ErrUnauthenticated, codes.Unauthenticated},
		{context.Canceled, codes.Canceled},
		{context.DeadlineExceeded, codes.DeadlineExceeded},
		{errors.New("private database detail"), codes.Unavailable},
	} {
		if got := ReauthenticationRPCError(test.err); status.Code(got) != test.code || got.Error() == test.err.Error() {
			t.Fatalf("RPC error = %v, want %v", got, test.code)
		}
	}
	private := "private database detail"
	if got := ReauthenticationRPCError(errors.New(private)); status.Convert(got).Message() != "re-authentication unavailable" || strings.Contains(got.Error(), private) || len(status.Convert(got).Details()) != 0 {
		t.Fatalf("private detail escaped: %v", got)
	}
	result := status.Convert(ReauthenticationRPCError(app.ErrReauthenticationRequired))
	if len(result.Details()) != 1 {
		t.Fatalf("details = %v", result.Details())
	}
	detail, ok := result.Details()[0].(*errdetails.ErrorInfo)
	if !ok || detail.GetReason() != "REAUTHENTICATION_REQUIRED" || detail.GetDomain() != "identity.flowspace" {
		t.Fatalf("error detail = %v", detail)
	}
}

type reauthenticationVerifier struct{}

func (reauthenticationVerifier) Verify(raw string) (outbound.AccessTokenIdentity, error) {
	if raw != "access-proof" {
		return outbound.AccessTokenIdentity{}, errors.New("private token")
	}
	return outbound.AccessTokenIdentity{Subject: "subject-proof", SessionID: "session-proof"}, nil
}

type passwordReauthenticationStub struct {
	input inbound.ReauthenticateWithPasswordInput
	err   error
	calls int
}

func (s *passwordReauthenticationStub) ReauthenticateWithPassword(ctx context.Context, input inbound.ReauthenticateWithPasswordInput) (inbound.ReauthenticateWithPasswordResult, error) {
	s.calls++
	s.input = input
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		return inbound.ReauthenticateWithPasswordResult{}, errors.New("deadline missing")
	}
	return inbound.ReauthenticateWithPasswordResult{ExpiresAt: time.Unix(1800000000, 0)}, s.err
}

func TestPasswordReauthenticationHandler(t *testing.T) {
	stub := &passwordReauthenticationStub{}
	handler := NewIdentityHandler(nil, nil, nil, nil, reauthenticationVerifier{}, nil).WithPasswordReauthentication(stub)
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 42}})
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer access-proof"))
	request := &identityv1.ReauthenticateWithPasswordRequest{Password: "private password"}
	response, err := handler.ReauthenticateWithPassword(ctx, request)
	if err != nil || !response.GetReauthenticationExpiresAt().AsTime().Equal(time.Unix(1800000000, 0)) || stub.input.Subject != "subject-proof" || stub.input.SessionID != "session-proof" || stub.input.Source != "192.0.2.1" || stub.input.Password != request.GetPassword() {
		t.Fatalf("handler result=%v, err=%v, input=%+v", response, err, stub.input)
	}
	for _, test := range []struct {
		err  error
		code codes.Code
	}{{app.ErrInvalidCredentials, codes.InvalidArgument}, {domain.ErrInvalidPassword, codes.InvalidArgument}, {app.ErrRateLimited, codes.ResourceExhausted}, {app.ErrLimitUnavailable, codes.Unavailable}, {outbound.ErrUnauthenticated, codes.Unauthenticated}, {context.Canceled, codes.Canceled}, {context.DeadlineExceeded, codes.DeadlineExceeded}, {errors.New("private database"), codes.Unavailable}} {
		stub.err = test.err
		if result, err := handler.ReauthenticateWithPassword(ctx, request); result != nil || status.Code(err) != test.code || strings.Contains(status.Convert(err).Message(), "private") {
			t.Fatalf("error=%v want=%v", err, test.code)
		}
	}
	if _, err := handler.ReauthenticateWithPassword(t.Context(), request); status.Code(err) != codes.Unauthenticated {
		t.Fatal(err)
	}
	if _, err := handler.ReauthenticateWithPassword(ctx, nil); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	if _, err := NewIdentityHandler(nil, nil, nil, nil, reauthenticationVerifier{}, nil).ReauthenticateWithPassword(ctx, request); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
}

func TestPasswordReauthenticationREST(t *testing.T) {
	stub := &passwordReauthenticationStub{}
	mux := runtime.NewServeMux()
	handler := NewIdentityHandler(nil, nil, nil, nil, reauthenticationVerifier{}, nil).WithPasswordReauthentication(stub)
	if err := identityv1.RegisterIdentityServiceHandlerServer(t.Context(), mux, handler); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		body, authorization string
		want                int
	}{{`{"password":"private password"}`, "Bearer access-proof", 200}, {`{"password":"private password"}`, "", 401}, {`{`, "Bearer access-proof", 400}} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/password-reauthentications", strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", test.authorization)
		request.RemoteAddr = "192.0.2.1:42"
		response := httptest.NewRecorder()
		NewIdentityRequestHandler(mux, nil).ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("REST=%d body=%s", response.Code, response.Body)
		}
		if strings.Contains(response.Body.String(), "private password") {
			t.Fatal("response exposed password")
		}
	}
}

type codeReauthenticationStub struct {
	request inbound.RequestReauthenticationCodeInput
	proof   inbound.ReauthenticateWithCodeInput
	err     error
}

func (s *codeReauthenticationStub) RequestReauthenticationCode(ctx context.Context, input inbound.RequestReauthenticationCodeInput) error {
	s.request = input
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		return errors.New("missing deadline")
	}
	return s.err
}

func (s *codeReauthenticationStub) ReauthenticateWithCode(ctx context.Context, input inbound.ReauthenticateWithCodeInput) (inbound.ReauthenticateWithCodeResult, error) {
	s.proof = input
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		return inbound.ReauthenticateWithCodeResult{}, errors.New("missing deadline")
	}
	return inbound.ReauthenticateWithCodeResult{ExpiresAt: time.Unix(1800000000, 0)}, s.err
}

func TestCodeReauthenticationHandler(t *testing.T) {
	stub := &codeReauthenticationStub{}
	handler := NewIdentityHandler(nil, nil, nil, nil, reauthenticationVerifier{}, nil).WithCodeReauthentication(stub)
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 42}})
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer access-proof"))
	requested, err := handler.RequestReauthenticationCode(ctx, &identityv1.RequestReauthenticationCodeRequest{})
	if err != nil || !requested.GetAccepted() || stub.request.Subject != "subject-proof" || stub.request.SessionID != "session-proof" || stub.request.Source != "192.0.2.1" {
		t.Fatalf("request=%v err=%v", requested, err)
	}
	proof, err := handler.ReauthenticateWithCode(ctx, &identityv1.ReauthenticateWithCodeRequest{Code: "012345"})
	if err != nil || !proof.GetReauthenticationExpiresAt().AsTime().Equal(time.Unix(1800000000, 0)) || stub.proof.Code != "012345" || stub.proof.Subject != "subject-proof" || stub.proof.SessionID != "session-proof" || stub.proof.Source != "192.0.2.1" {
		t.Fatalf("proof=%v err=%v", proof, err)
	}
}

func TestCodeReauthenticationHandlerErrors(t *testing.T) {
	stub := &codeReauthenticationStub{}
	handler := NewIdentityHandler(nil, nil, nil, nil, reauthenticationVerifier{}, nil).WithCodeReauthentication(stub)
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 42}})
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer access-proof"))
	for _, test := range []struct {
		err  error
		code codes.Code
	}{{app.ErrInvalidReauthenticationCode, codes.InvalidArgument}, {app.ErrReauthenticationEmailUnverified, codes.FailedPrecondition}, {app.ErrRateLimited, codes.ResourceExhausted}, {app.ErrLimitUnavailable, codes.Unavailable}, {outbound.ErrUnauthenticated, codes.Unauthenticated}, {context.Canceled, codes.Canceled}, {context.DeadlineExceeded, codes.DeadlineExceeded}, {errors.New("private code store"), codes.Unavailable}} {
		stub.err = test.err
		if _, err := handler.RequestReauthenticationCode(ctx, &identityv1.RequestReauthenticationCodeRequest{}); status.Code(err) != test.code || strings.Contains(status.Convert(err).Message(), "private") {
			t.Fatalf("request error=%v", err)
		}
		if _, err := handler.ReauthenticateWithCode(ctx, &identityv1.ReauthenticateWithCodeRequest{Code: "012345"}); status.Code(err) != test.code || strings.Contains(status.Convert(err).Message(), "private") {
			t.Fatalf("proof error=%v", err)
		}
	}
}

func TestCodeReauthenticationHandlerAdmission(t *testing.T) {
	handler := NewIdentityHandler(nil, nil, nil, nil, reauthenticationVerifier{}, nil).WithCodeReauthentication(&codeReauthenticationStub{})
	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("authorization", "Bearer access-proof"))
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 42}})
	for _, call := range []func() error{
		func() error {
			_, err := handler.RequestReauthenticationCode(t.Context(), &identityv1.RequestReauthenticationCodeRequest{})
			return err
		},
		func() error {
			_, err := handler.ReauthenticateWithCode(t.Context(), &identityv1.ReauthenticateWithCodeRequest{Code: "012345"})
			return err
		},
	} {
		if err := call(); status.Code(err) != codes.Unauthenticated {
			t.Fatal(err)
		}
	}
	if _, err := handler.RequestReauthenticationCode(ctx, nil); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	if _, err := handler.ReauthenticateWithCode(ctx, nil); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	empty := NewIdentityHandler(nil, nil, nil, nil, reauthenticationVerifier{}, nil)
	if _, err := empty.RequestReauthenticationCode(ctx, &identityv1.RequestReauthenticationCodeRequest{}); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	if _, err := empty.ReauthenticateWithCode(ctx, &identityv1.ReauthenticateWithCodeRequest{Code: "012345"}); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
}

func TestCodeReauthenticationREST(t *testing.T) {
	stub := &codeReauthenticationStub{}
	mux := runtime.NewServeMux()
	handler := NewIdentityHandler(nil, nil, nil, nil, reauthenticationVerifier{}, nil).WithCodeReauthentication(stub)
	if err := identityv1.RegisterIdentityServiceHandlerServer(t.Context(), mux, handler); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/reauthentication-codes", "/v1/code-reauthentications"} {
		for _, auth := range []string{"Bearer access-proof", ""} {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(`{"code":"012345"}`))
			if path == "/v1/reauthentication-codes" {
				request = httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(`{}`))
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", auth)
			request.RemoteAddr = "192.0.2.1:42"
			response := httptest.NewRecorder()
			NewIdentityRequestHandler(mux, nil).ServeHTTP(response, request)
			want := http.StatusOK
			if auth == "" {
				want = http.StatusUnauthorized
			}
			if response.Code != want {
				t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body)
			}
		}
	}
}

type providerReauthenticationStub struct {
	start    inbound.StartProviderReauthenticationInput
	complete inbound.CompleteProviderReauthenticationInput
	err      error
	calls    int
}

func (s *providerReauthenticationStub) StartProviderReauthentication(ctx context.Context, input inbound.StartProviderReauthenticationInput) (inbound.StartProviderLoginResult, error) {
	s.calls++
	s.start = input
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		return inbound.StartProviderLoginResult{}, errors.New("missing deadline")
	}
	return inbound.StartProviderLoginResult{AuthorizationURL: "https://github.com/login/oauth/authorize?state=s", AttemptToken: "attempt", ExpiresAt: time.Unix(1800000000, 0)}, s.err
}

func (s *providerReauthenticationStub) CompleteProviderReauthentication(ctx context.Context, input inbound.CompleteProviderReauthenticationInput) (inbound.CompleteProviderReauthenticationResult, error) {
	s.calls++
	s.complete = input
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		return inbound.CompleteProviderReauthenticationResult{}, errors.New("missing deadline")
	}
	return inbound.CompleteProviderReauthenticationResult{ExpiresAt: time.Unix(1800000000, 0)}, s.err
}

func providerReauthenticationContext(t *testing.T) context.Context {
	t.Helper()
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 42}})
	return metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer access-proof"))
}

func TestProviderReauthenticationHandler(t *testing.T) {
	stub := &providerReauthenticationStub{}
	handler := NewIdentityHandler(nil, nil, nil, nil, reauthenticationVerifier{}, nil).WithProviderReauthentication(stub)
	ctx := providerReauthenticationContext(t)

	started, err := handler.StartProviderReauthentication(ctx, &identityv1.StartProviderReauthenticationRequest{Provider: identityv1.Provider_PROVIDER_GITHUB})
	if err != nil || started.GetAuthorizationUrl() != "https://github.com/login/oauth/authorize?state=s" || started.GetAttemptToken() != "attempt" ||
		!started.GetAttemptExpiresAt().AsTime().Equal(time.Unix(1800000000, 0)) ||
		stub.start != (inbound.StartProviderReauthenticationInput{Subject: "subject-proof", SessionID: "session-proof", Provider: "github", Source: "192.0.2.1"}) {
		t.Fatalf("start = %v, error = %v, input = %+v", started, err, stub.start)
	}
	completed, err := handler.CompleteProviderReauthentication(ctx, &identityv1.CompleteProviderReauthenticationRequest{AttemptToken: "attempt", HandoffCode: "handoff"})
	if err != nil || !completed.GetReauthenticationExpiresAt().AsTime().Equal(time.Unix(1800000000, 0)) ||
		stub.complete != (inbound.CompleteProviderReauthenticationInput{Subject: "subject-proof", SessionID: "session-proof", AttemptToken: "attempt", HandoffCode: "handoff", Source: "192.0.2.1"}) {
		t.Fatalf("complete = %v, error = %v, input = %+v", completed, err, stub.complete)
	}
}

func TestProviderReauthenticationHandlerErrors(t *testing.T) {
	stub := &providerReauthenticationStub{}
	handler := NewIdentityHandler(nil, nil, nil, nil, reauthenticationVerifier{}, nil).WithProviderReauthentication(stub)
	ctx := providerReauthenticationContext(t)
	for _, test := range []struct {
		err  error
		code codes.Code
	}{
		{domain.ErrInvalidProvider, codes.InvalidArgument},
		{domain.ErrInvalidAttemptToken, codes.InvalidArgument},
		{domain.ErrInvalidHandoffCode, codes.InvalidArgument},
		{app.ErrProviderHandoffRejected, codes.FailedPrecondition},
		{app.ErrProviderAccountUnavailable, codes.FailedPrecondition},
		{app.ErrRateLimited, codes.ResourceExhausted},
		{app.ErrLimitUnavailable, codes.Unavailable},
		{outbound.ErrUnauthenticated, codes.Unauthenticated},
		{context.Canceled, codes.Canceled},
		{errors.New("private provider store"), codes.Unavailable},
	} {
		stub.err = test.err
		if _, err := handler.StartProviderReauthentication(ctx, &identityv1.StartProviderReauthenticationRequest{Provider: identityv1.Provider_PROVIDER_GOOGLE}); status.Code(err) != test.code || strings.Contains(status.Convert(err).Message(), "private") {
			t.Fatalf("start error %v = %v, want %v", test.err, err, test.code)
		}
		if _, err := handler.CompleteProviderReauthentication(ctx, &identityv1.CompleteProviderReauthenticationRequest{AttemptToken: "attempt", HandoffCode: "handoff"}); status.Code(err) != test.code || strings.Contains(status.Convert(err).Message(), "private") {
			t.Fatalf("complete error %v = %v, want %v", test.err, err, test.code)
		}
	}
}

func TestProviderReauthenticationHandlerAdmission(t *testing.T) {
	stub := &providerReauthenticationStub{}
	handler := NewIdentityHandler(nil, nil, nil, nil, reauthenticationVerifier{}, nil).WithProviderReauthentication(stub)
	ctx := providerReauthenticationContext(t)
	start := &identityv1.StartProviderReauthenticationRequest{Provider: identityv1.Provider_PROVIDER_GOOGLE}
	complete := &identityv1.CompleteProviderReauthenticationRequest{AttemptToken: "attempt", HandoffCode: "handoff"}
	if _, err := handler.StartProviderReauthentication(t.Context(), start); status.Code(err) != codes.Unauthenticated {
		t.Fatal(err)
	}
	if _, err := handler.CompleteProviderReauthentication(t.Context(), complete); status.Code(err) != codes.Unauthenticated {
		t.Fatal(err)
	}
	for _, request := range []*identityv1.StartProviderReauthenticationRequest{nil, {}, {Provider: identityv1.Provider(99)}} {
		if _, err := handler.StartProviderReauthentication(ctx, request); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("start %v error = %v", request, err)
		}
	}
	retry := metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer access-proof", "idempotency-key", "retry"))
	if _, err := handler.StartProviderReauthentication(retry, start); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("idempotency error = %v", err)
	}
	if _, err := handler.CompleteProviderReauthentication(ctx, nil); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	if stub.calls != 0 {
		t.Fatalf("rejected requests reached the service %d times", stub.calls)
	}
	empty := NewIdentityHandler(nil, nil, nil, nil, reauthenticationVerifier{}, nil)
	if _, err := empty.StartProviderReauthentication(ctx, start); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	if _, err := empty.CompleteProviderReauthentication(ctx, complete); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
}

func TestProviderReauthenticationREST(t *testing.T) {
	stub := &providerReauthenticationStub{}
	mux := runtime.NewServeMux()
	handler := NewIdentityHandler(nil, nil, nil, nil, reauthenticationVerifier{}, nil).WithProviderReauthentication(stub)
	if err := identityv1.RegisterIdentityServiceHandlerServer(t.Context(), mux, handler); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path, body, authorization string
		idempotencyKey            bool
		want                      int
		fields                    []string
	}{
		{"/v1/provider-reauthentication-attempts", `{"provider":"PROVIDER_GITHUB"}`, "Bearer access-proof", false, http.StatusOK, []string{
			`"authorizationUrl":"https://github.com/login/oauth/authorize?state=s"`, `"attemptToken":"attempt"`, `"attemptExpiresAt":"2027-01-15T08:00:00Z"`,
		}},
		{"/v1/provider-reauthentication-attempts", `{"provider":"PROVIDER_GITHUB"}`, "", false, http.StatusUnauthorized, nil},
		{"/v1/provider-reauthentication-attempts", `{"provider":"PROVIDER_GITHUB"}`, "Bearer access-proof", true, http.StatusBadRequest, nil},
		{"/v1/provider-reauthentication-attempts", `{"provider":"PROVIDER_OKTA"}`, "Bearer access-proof", false, http.StatusBadRequest, nil},
		{"/v1/provider-reauthentications", `{"attemptToken":"attempt","handoffCode":"handoff"}`, "Bearer access-proof", false, http.StatusOK, []string{
			`"reauthenticationExpiresAt":"2027-01-15T08:00:00Z"`,
		}},
		{"/v1/provider-reauthentications", `{"attemptToken":"attempt","handoffCode":"handoff"}`, "", false, http.StatusUnauthorized, nil},
	} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, test.path, strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", test.authorization)
		if test.idempotencyKey {
			request.Header["Idempotency-Key"] = []string{""}
		}
		request.RemoteAddr = "192.0.2.1:42"
		response := httptest.NewRecorder()
		NewIdentityRequestHandler(mux, nil).ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("%s status = %d, want %d, body = %s", test.path, response.Code, test.want, response.Body)
		}
		for _, field := range test.fields {
			if !strings.Contains(response.Body.String(), field) {
				t.Fatalf("%s response lacks %s: %s", test.path, field, response.Body)
			}
		}
	}
	if stub.calls != 2 || stub.start != (inbound.StartProviderReauthenticationInput{Subject: "subject-proof", SessionID: "session-proof", Provider: "github", Source: "192.0.2.1"}) ||
		stub.complete != (inbound.CompleteProviderReauthenticationInput{Subject: "subject-proof", SessionID: "session-proof", AttemptToken: "attempt", HandoffCode: "handoff", Source: "192.0.2.1"}) {
		t.Fatalf("service calls = %d, start = %+v, complete = %+v", stub.calls, stub.start, stub.complete)
	}
}
