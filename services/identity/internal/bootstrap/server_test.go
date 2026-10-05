package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/internal/requestid"
)

func TestSigningKeyPublicationBeforeAndAfterSwitch(t *testing.T) {
	oldPublic, oldPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	newPublic, newPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []struct {
		name, activeID, additional string
		active                     ed25519.PrivateKey
		want                       map[string]ed25519.PublicKey
	}{
		{
			"published", "old", "new:" + base64.RawURLEncoding.EncodeToString(newPublic), oldPrivate,
			map[string]ed25519.PublicKey{"old": oldPublic, "new": newPublic},
		},
		{
			"switched", "new", "old:" + base64.RawURLEncoding.EncodeToString(oldPublic), newPrivate,
			map[string]ed25519.PublicKey{"old": oldPublic, "new": newPublic},
		},
		{"retired", "new", "", newPrivate, map[string]ed25519.PublicKey{"new": newPublic}},
	} {
		t.Run(phase.name, func(t *testing.T) {
			keys, document, err := buildSigningKeys(phase.active, phase.activeID, phase.additional)
			if err != nil {
				t.Fatal(err)
			}
			var jwks jose.JSONWebKeySet
			if err := json.Unmarshal(document, &jwks); err != nil {
				t.Fatal(err)
			}
			if len(keys) != len(phase.want) || len(jwks.Keys) != len(phase.want) {
				t.Fatalf("published %d verifier keys and %d JWKS keys, want %d", len(keys), len(jwks.Keys), len(phase.want))
			}
			for _, key := range jwks.Keys {
				want, ok := phase.want[key.KeyID]
				published, valid := key.Key.(ed25519.PublicKey)
				if !ok || !valid || string(keys[key.KeyID]) != string(want) || string(published) != string(want) {
					t.Fatalf("wrong public key for %q", key.KeyID)
				}
			}
		})
	}
}

func TestPasswordLoginTelemetryOmitsCredentials(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	handler := observeRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}), zap.New(core), nil)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/v1/password-sessions?email=User@example.com", strings.NewReader("secret-password"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || logs.Len() != 1 {
		t.Fatalf("login log count = %d, status = %d", logs.Len(), response.Code)
	}
	fields := fmt.Sprint(logs.All()[0].ContextMap())
	if strings.Contains(fields, "User@example.com") || strings.Contains(fields, "secret-password") ||
		logs.All()[0].ContextMap()["operation"] != "POST /v1/password-sessions" {
		t.Fatal("login telemetry contains request data or misses its operation")
	}
}

func TestRefreshTelemetryOmitsToken(t *testing.T) {
	for _, outcome := range []int{http.StatusOK, http.StatusUnauthorized} {
		core, logs := observer.New(zap.InfoLevel)
		handler := observeRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(outcome)
		}), zap.New(core), nil)
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
			"/v1/session-refreshes?refresh_token=secret-refresh-token", strings.NewReader(`{"refreshToken":"secret-refresh-token"}`))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != outcome || logs.Len() != 1 {
			t.Fatalf("refresh log count = %d, status = %d", logs.Len(), response.Code)
		}
		fields := fmt.Sprint(logs.All()[0].ContextMap())
		if strings.Contains(fields, "secret-refresh-token") || logs.All()[0].ContextMap()["operation"] != "POST /v1/session-refreshes" {
			t.Fatal("refresh telemetry contains token or misses its operation")
		}
	}
}

func TestLogoutTelemetryOmitsToken(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	handler := observeRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), zap.New(core), nil)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/session-logouts?access_token=secret-access-token", nil)
	request.Header.Set("Authorization", "Bearer secret-access-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || logs.Len() != 1 {
		t.Fatalf("logout log count = %d, status = %d", logs.Len(), response.Code)
	}
	fields := fmt.Sprint(logs.All()[0].ContextMap())
	if strings.Contains(fields, "secret-access-token") || logs.All()[0].ContextMap()["operation"] != "POST /v1/session-logouts" {
		t.Fatal("logout telemetry contains token or misses its operation")
	}
}

func TestAllLogoutTelemetryOmitsToken(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/account-session-logouts?access_token=secret-access-token", nil)
	request.Header.Set("Authorization", "Bearer secret-access-token")
	operation := safeOperation(request, nil)
	if operation != "POST /v1/account-session-logouts" || strings.Contains(operation, "secret-access-token") {
		t.Fatal("all-session logout telemetry contains token or misses its operation")
	}
}

func TestSigningKeyPublicationRejectsInvalidAdditionalKey(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"current:AA", "current:" + base64.RawURLEncoding.EncodeToString(publicKey), "missing-colon"} {
		if _, _, err := buildSigningKeys(privateKey, "current", value); err == nil {
			t.Fatalf("accepted invalid additional key %q", value)
		}
	}
}

type stubIdentityHandler struct {
	identityv1.UnimplementedIdentityServiceServer
}

func (stubIdentityHandler) CreateAccount(context.Context, *identityv1.CreateAccountRequest) (*identityv1.CreateAccountResponse, error) {
	return &identityv1.CreateAccountResponse{Subject: "subject-1"}, nil
}

func (stubIdentityHandler) RequestPasswordResetCode(context.Context, *identityv1.RequestPasswordResetCodeRequest) (*identityv1.RequestPasswordResetCodeResponse, error) {
	return &identityv1.RequestPasswordResetCodeResponse{Accepted: true}, nil
}

func (stubIdentityHandler) ResetPassword(context.Context, *identityv1.ResetPasswordRequest) (*identityv1.ResetPasswordResponse, error) {
	return &identityv1.ResetPasswordResponse{PasswordChanged: true, SessionsRevoked: true}, nil
}

func (stubIdentityHandler) CheckSession(context.Context, *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
	return &identityv1.CheckSessionResponse{EmailVerified: true}, nil
}

func TestPublicHandlerOnlyServesApprovedRoutes(t *testing.T) {
	handler, err := newPublicHandler(t.Context(), stubIdentityHandler{}, http.NotFoundHandler(), zap.NewNop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/accounts", strings.NewReader(`{"email":"a@example.com","password":"password"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", "request-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("X-Request-ID") != "request-1" {
		t.Fatalf("approved route: status %d, request ID %q", response.Code, response.Header().Get("X-Request-ID"))
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/.well-known/jwks.json", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("public JWKS status = %d", response.Code)
	}
}

func TestPublicHandlerServesTypedRPC(t *testing.T) {
	handler, err := newPublicHandler(t.Context(), stubIdentityHandler{}, http.NotFoundHandler(), zap.NewNop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := proto.Marshal(&identityv1.CreateAccountRequest{Email: "a@example.com", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	const payloadLength = 25
	if len(payload) != payloadLength {
		t.Fatalf("unexpected test payload size: %d", len(payload))
	}
	frame := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(frame[1:5], payloadLength)
	copy(frame[5:], payload)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/flowspace.identity.v1.IdentityService/CreateAccount", strings.NewReader(string(frame)))
	request.ProtoMajor = 2
	request.Header.Set("Content-Type", "application/grpc")
	request.Header.Set("TE", "trailers")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Result().Trailer.Get("Grpc-Status") != "0" {
		t.Fatalf("typed RPC status = %d, trailer = %v", response.Code, response.Result().Trailer)
	}
}

func TestPublicHandlerServesPasswordRecoveryTypedRPC(t *testing.T) {
	handler, err := newPublicHandler(t.Context(), stubIdentityHandler{}, http.NotFoundHandler(), zap.NewNop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method  string
		message proto.Message
		length  byte
	}{
		{"RequestPasswordResetCode", &identityv1.RequestPasswordResetCodeRequest{Email: "a@example.com"}, 15},
		{"ResetPassword", &identityv1.ResetPasswordRequest{Email: "a@example.com", Code: "012345", NewPassword: "fresh password 123"}, 43},
	} {
		method := test.method
		payload, err := proto.Marshal(test.message)
		if err != nil {
			t.Fatal(err)
		}
		if len(payload) != int(test.length) {
			t.Fatalf("unexpected %s test payload size: %d", method, len(payload))
		}
		frame := append([]byte{0, 0, 0, 0, test.length}, payload...)
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
			"/flowspace.identity.v1.IdentityService/"+method, strings.NewReader(string(frame)))
		request.ProtoMajor = 2
		request.Header.Set("Content-Type", "application/grpc")
		request.Header.Set("TE", "trailers")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Result().Trailer.Get("Grpc-Status") != "0" {
			t.Fatalf("%s typed RPC status = %d, trailer = %v", method, response.Code, response.Result().Trailer)
		}
	}
}

func TestPasswordRecoveryTelemetryOmitsSecrets(t *testing.T) {
	for _, test := range []struct{ path, body string }{
		{"/v1/password-reset-codes", `{"email":"User@example.com"}`},
		{"/v1/password-resets", `{"email":"User@example.com","code":"012345","newPassword":"secret-new-password"}`},
	} {
		path, body := test.path, test.body
		for _, outcome := range []int{http.StatusOK, http.StatusBadRequest, http.StatusTooManyRequests} {
			core, logs := observer.New(zap.InfoLevel)
			handler := observeRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(outcome)
			}), zap.New(core), nil)
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
				path+"?email=User@example.com&code=012345", strings.NewReader(body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != outcome || logs.Len() != 1 {
				t.Fatalf("%s log count = %d, status = %d", path, logs.Len(), response.Code)
			}
			fields := fmt.Sprint(logs.All()[0].ContextMap())
			if strings.Contains(fields, "User@example.com") || strings.Contains(fields, "012345") ||
				strings.Contains(fields, "secret-new-password") || logs.All()[0].ContextMap()["operation"] != "POST "+path {
				t.Fatalf("%s telemetry contains request data or misses its operation: %s", path, fields)
			}
		}
	}
}

func TestProviderLoginTelemetryOmitsSecrets(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	handler := observeRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"authorizationUrl":"https://accounts.google.com/o/oauth2/v2/auth?state=secret-state","attemptToken":"secret-attempt"}`))
	}), zap.New(core), nil)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/v1/provider-login-attempts?state=secret-state", strings.NewReader(`{"provider":"PROVIDER_GOOGLE"}`))
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if logs.Len() != 1 {
		t.Fatalf("provider login log count = %d", logs.Len())
	}
	fields := fmt.Sprint(logs.All()[0].ContextMap())
	if strings.Contains(fields, "secret-") || logs.All()[0].ContextMap()["operation"] != "POST /v1/provider-login-attempts" {
		t.Fatalf("provider login telemetry contains secrets or misses its operation: %s", fields)
	}
}

func TestProviderSessionTelemetryOmitsSecrets(t *testing.T) {
	for _, outcome := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusTooManyRequests} {
		core, logs := observer.New(zap.InfoLevel)
		handler := observeRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(outcome)
			_, _ = w.Write([]byte(`{"accessToken":"secret-access","refreshToken":"secret-refresh"}`))
		}), zap.New(core), nil)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodPost,
			"/v1/provider-sessions?handoffCode=secret-code", strings.NewReader(`{"attemptToken":"secret-attempt","handoffCode":"secret-code"}`)))
		if logs.Len() != 1 {
			t.Fatalf("status %d log count = %d", outcome, logs.Len())
		}
		entry := logs.All()[0].ContextMap()
		if fields := fmt.Sprint(entry); strings.Contains(fields, "secret-") || entry["operation"] != "POST /v1/provider-sessions" {
			t.Fatalf("status %d telemetry contains secrets or misses its operation: %s", outcome, fields)
		}
	}
}

func TestPublicHandlerRoutesProviderCallbacks(t *testing.T) {
	var provider string
	callback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider = r.PathValue("provider")
		w.WriteHeader(http.StatusTeapot)
	})
	handler, err := newPublicHandler(t.Context(), stubIdentityHandler{}, callback, zap.NewNop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/provider-login-callbacks/google?state=s&code=c", nil))
	if response.Code != http.StatusTeapot || provider != "google" {
		t.Fatalf("callback route status = %d, provider = %q", response.Code, provider)
	}
	provider = ""
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/provider-login-callbacks/google", nil))
	if response.Code != http.StatusNotFound || provider != "" {
		t.Fatalf("callback POST status = %d", response.Code)
	}
}

func TestProviderCallbackTelemetryOmitsSecrets(t *testing.T) {
	for _, path := range []string{"/v1/provider-login-callbacks/google", "/v1/provider-login-callbacks/github", "/v1/provider-login-callbacks/secret-provider"} {
		core, logs := observer.New(zap.InfoLevel)
		handler := observeRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("secret-handoff"))
		}), zap.New(core), nil)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			path+"?state=secret-state&code=secret-code", nil))
		fields := fmt.Sprint(logs.All()[0].ContextMap())
		if logs.Len() != 1 || strings.Contains(fields, "secret-") || logs.All()[0].ContextMap()["operation"] != "GET /v1/provider-login-callbacks/{provider}" {
			t.Fatalf("callback telemetry contains secrets or misses its operation: %s", fields)
		}
	}
}

func TestPublicHandlerRejectsCheckSession(t *testing.T) {
	handler, err := newPublicHandler(t.Context(), stubIdentityHandler{}, http.NotFoundHandler(), zap.NewNop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := proto.Marshal(&identityv1.CheckSessionRequest{Subject: "subject-1", SessionId: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	const payloadLength = 22
	if len(payload) != payloadLength {
		t.Fatalf("unexpected test payload size: %d", len(payload))
	}
	frame := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(frame[1:5], payloadLength)
	copy(frame[5:], payload)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/flowspace.identity.v1.IdentityService/CheckSession", strings.NewReader(string(frame)))
	request.ProtoMajor = 2
	request.Header.Set("Content-Type", "application/grpc")
	request.Header.Set("TE", "trailers")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := response.Result().Trailer.Get("Grpc-Status"); got != "12" {
		t.Fatalf("public CheckSession gRPC status = %q, want 12", got)
	}
}

func TestPublicRequestIdentifiersAndProxyConfiguration(t *testing.T) {
	if requestid.Valid("bad value") || requestid.Valid(strings.Repeat("a", 129)) || !requestid.Valid("safe-1") {
		t.Fatal("request ID validation failed")
	}
	if _, err := loadTrustedProxies("0.0.0.0/0"); err == nil {
		t.Fatal("accepted wildcard trusted proxy")
	}
	if prefixes, err := loadTrustedProxies("127.0.0.1/32"); err != nil || len(prefixes) != 1 {
		t.Fatalf("trusted proxies = %v, %v", prefixes, err)
	}
}

const (
	parentTraceID     = "4bf92f3577b34da6a3ce929d0e0e4736"
	parentSpanID      = "00f067aa0ba902b7"
	parentTraceparent = "00-" + parentTraceID + "-" + parentSpanID + "-01"
)

// recordSpans installs a global tracer provider that keeps ended spans in
// memory. Tests that call it must not run in parallel.
func recordSpans(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	return exporter
}

func onlySpan(t *testing.T, exporter *tracetest.InMemoryExporter) tracetest.SpanStub {
	t.Helper()
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	return spans[0]
}

func spanAttributes(span tracetest.SpanStub) map[string]string {
	attributes := map[string]string{}
	for _, item := range span.Attributes {
		attributes[string(item.Key)] = item.Value.String()
	}
	return attributes
}

func grpcRequest(t *testing.T, method string, message proto.Message) *http.Request {
	t.Helper()
	payload, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	length := len(payload)
	if length < 0 || length > math.MaxUint32 {
		t.Fatalf("payload has %d bytes", length)
		return nil
	}
	frame := make([]byte, 5+length)
	binary.BigEndian.PutUint32(frame[1:5], uint32(length))
	copy(frame[5:], payload)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, method, strings.NewReader(string(frame)))
	request.ProtoMajor = 2
	request.Header.Set("Content-Type", "application/grpc")
	request.Header.Set("TE", "trailers")
	return request
}

func restRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

type failingIdentityHandler struct {
	stubIdentityHandler
	err error
}

func (h failingIdentityHandler) CreateAccount(context.Context, *identityv1.CreateAccountRequest) (*identityv1.CreateAccountResponse, error) {
	return nil, h.err
}

func publicHandler(t *testing.T, handler identityv1.IdentityServiceServer, logger *zap.Logger) http.Handler {
	t.Helper()
	public, err := newPublicHandler(t.Context(), handler, http.NotFoundHandler(), logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	return public
}

// assertRPCLineMatches makes sure that the one identity_rpc line has a
// duration and the correlation and completion fields of the identity_request
// line.
func assertRPCLineMatches(t *testing.T, logs *observer.ObservedLogs, request map[string]any) {
	t.Helper()
	rpcLines := logs.FilterMessage("identity_rpc").All()
	if len(rpcLines) != 1 {
		t.Fatalf("identity_rpc lines = %d, want 1", len(rpcLines))
	}
	rpc := rpcLines[0].ContextMap()
	if _, ok := rpc["duration"].(time.Duration); !ok {
		t.Fatalf("identity_rpc duration = %#v", rpc["duration"])
	}
	for _, key := range []string{"request_id", "trace_id", "operation", "outcome", "status"} {
		if rpc[key] != request[key] {
			t.Fatalf("identity_rpc %s = %#v, want %#v", key, rpc[key], request[key])
		}
	}
}

func TestPublicHandlerCreatesOneBoundedServerSpan(t *testing.T) {
	createAccount := identityv1.IdentityService_CreateAccount_FullMethodName
	tests := []struct {
		name      string
		request   func(*testing.T) *http.Request
		wantName  string
		wantAttrs map[string]string
		wantLog   map[string]any
		// wantRPCLine means that the gRPC interceptor also writes an
		// identity_rpc completion line with the same fields.
		wantRPCLine bool
	}{
		{
			name: "known REST route",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				t.Helper()
				return restRequest(t, http.MethodPost, "/v1/accounts", `{}`)
			},
			wantName: "POST /v1/accounts",
			wantAttrs: map[string]string{
				"http.request.method": "POST", "http.route": "/v1/accounts", "http.response.status_code": "200",
			},
			wantLog: map[string]any{"operation": "POST /v1/accounts", "status": int64(200), "outcome": "success"},
		},
		{
			name: "provider callback",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return restRequest(t, http.MethodGet, "/v1/provider-login-callbacks/secret-provider?code=secret-code", "")
			},
			wantName: "GET /v1/provider-login-callbacks/{provider}",
			wantAttrs: map[string]string{
				"http.request.method": "GET", "http.route": "/v1/provider-login-callbacks/{provider}", "http.response.status_code": "404",
			},
			wantLog: map[string]any{"operation": "GET /v1/provider-login-callbacks/{provider}", "status": int64(404), "outcome": "failure"},
		},
		{
			name: "unknown path",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				t.Helper()
				return restRequest(t, http.MethodGet, "/v1/accounts/subject-1/secret-path", "")
			},
			wantName:  "GET",
			wantAttrs: map[string]string{"http.request.method": "GET", "http.response.status_code": "404"},
			wantLog:   map[string]any{"operation": "unknown", "status": int64(404), "outcome": "failure"},
		},
		{
			name: "client-defined method",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				t.Helper()
				return restRequest(t, "SECRET-METHOD", "/v1/accounts", "")
			},
			wantName:  "_OTHER /v1/accounts",
			wantAttrs: map[string]string{"http.request.method": "_OTHER", "http.route": "/v1/accounts", "http.response.status_code": "501"},
			wantLog:   map[string]any{"operation": "_OTHER /v1/accounts", "status": int64(501), "outcome": "failure"},
		},
		{
			name: "known gRPC method",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				t.Helper()
				return grpcRequest(t, createAccount, &identityv1.CreateAccountRequest{})
			},
			wantName: createAccount,
			wantAttrs: map[string]string{
				"rpc.method": strings.TrimPrefix(createAccount, "/"), "rpc.grpc.status_code": "0",
			},
			wantLog:     map[string]any{"operation": createAccount, "status": "OK", "outcome": "success"},
			wantRPCLine: true,
		},
		{
			name: "unknown gRPC method",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				t.Helper()
				return grpcRequest(t, "/flowspace.identity.v1.IdentityService/SecretMethod", &identityv1.CreateAccountRequest{})
			},
			wantName:  "POST",
			wantAttrs: map[string]string{"rpc.grpc.status_code": "12"},
			wantLog:   map[string]any{"operation": "unknown", "status": "Unimplemented", "outcome": "failure"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exporter := recordSpans(t)
			core, logs := observer.New(zap.InfoLevel)
			request := test.request(t)
			request.Header.Set("X-Request-ID", "request-1")
			publicHandler(t, stubIdentityHandler{}, zap.New(core)).ServeHTTP(httptest.NewRecorder(), request)

			span := onlySpan(t, exporter)
			if span.Name != test.wantName || span.SpanKind != trace.SpanKindServer {
				t.Fatalf("span = %q kind %v, want %q server", span.Name, span.SpanKind, test.wantName)
			}
			if got := spanAttributes(span); fmt.Sprint(got) != fmt.Sprint(test.wantAttrs) {
				t.Fatalf("attributes = %v, want %v", got, test.wantAttrs)
			}
			entries := logs.FilterMessage("identity_request").All()
			if len(entries) != 1 {
				t.Fatalf("identity_request lines = %d, want 1", len(entries))
			}
			fields := entries[0].ContextMap()
			if fields["request_id"] != "request-1" || fields["trace_id"] != span.SpanContext.TraceID().String() {
				t.Fatalf("request line = %v, span trace ID %s", fields, span.SpanContext.TraceID())
			}
			if _, ok := fields["duration"].(time.Duration); !ok {
				t.Fatalf("duration = %#v", fields["duration"])
			}
			for key, want := range test.wantLog {
				if fields[key] != want {
					t.Fatalf("%s = %#v, want %#v", key, fields[key], want)
				}
			}
			if test.wantRPCLine {
				assertRPCLineMatches(t, logs, fields)
			}
		})
	}
}

func TestPublicHandlerContinuesIncomingTraceparent(t *testing.T) {
	createAccount := identityv1.IdentityService_CreateAccount_FullMethodName
	for name, request := range map[string]func(*testing.T) *http.Request{
		"REST": func(t *testing.T) *http.Request {
			t.Helper()
			t.Helper()
			return restRequest(t, http.MethodPost, "/v1/accounts", `{}`)
		},
		"gRPC": func(t *testing.T) *http.Request {
			t.Helper()
			return grpcRequest(t, createAccount, &identityv1.CreateAccountRequest{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			exporter := recordSpans(t)
			core, logs := observer.New(zap.InfoLevel)
			incoming := request(t)
			incoming.Header.Set("Traceparent", parentTraceparent)
			publicHandler(t, stubIdentityHandler{}, zap.New(core)).ServeHTTP(httptest.NewRecorder(), incoming)

			span := onlySpan(t, exporter)
			if span.Parent.TraceID().String() != parentTraceID || span.Parent.SpanID().String() != parentSpanID || !span.Parent.IsRemote() {
				t.Fatalf("parent = %v", span.Parent)
			}
			if span.SpanContext.TraceID().String() != parentTraceID || span.SpanContext.SpanID().String() == parentSpanID {
				t.Fatalf("span context = %v", span.SpanContext)
			}
			for _, entry := range logs.All() {
				if entry.ContextMap()["trace_id"] != parentTraceID {
					t.Fatalf("%s trace_id = %v", entry.Message, entry.ContextMap()["trace_id"])
				}
			}
		})
	}
}

func TestPublicHandlerStartsARootWithoutAValidTraceparent(t *testing.T) {
	for _, traceparent := range []string{"", "not-a-traceparent", "00-00000000000000000000000000000000-" + parentSpanID + "-01"} {
		exporter := recordSpans(t)
		incoming := restRequest(t, http.MethodPost, "/v1/accounts", `{}`)
		if traceparent != "" {
			incoming.Header.Set("Traceparent", traceparent)
		}
		publicHandler(t, stubIdentityHandler{}, zap.NewNop()).ServeHTTP(httptest.NewRecorder(), incoming)

		span := onlySpan(t, exporter)
		if span.Parent.IsValid() || !span.SpanContext.IsValid() {
			t.Fatalf("traceparent %q: parent %v, span %v", traceparent, span.Parent, span.SpanContext)
		}
	}
}

func TestPublicHandlerRecordsGRPCFailures(t *testing.T) {
	createAccount := identityv1.IdentityService_CreateAccount_FullMethodName
	for _, test := range []struct {
		code      codes.Code
		wantError bool
	}{
		{codes.InvalidArgument, false},
		{codes.Unauthenticated, false},
		{codes.Internal, true},
		{codes.Unavailable, true},
		{codes.DeadlineExceeded, true},
		{codes.Unknown, true},
	} {
		exporter := recordSpans(t)
		core, logs := observer.New(zap.InfoLevel)
		handler := failingIdentityHandler{err: status.Error(test.code, "secret-detail")}
		incoming := grpcRequest(t, createAccount, &identityv1.CreateAccountRequest{})
		incoming.Header.Set("X-Request-ID", "request-1")
		publicHandler(t, handler, zap.New(core)).ServeHTTP(httptest.NewRecorder(), incoming)

		span := onlySpan(t, exporter)
		if got := span.Status.Code == otelcodes.Error; got != test.wantError || span.Status.Description != "" {
			t.Fatalf("%v: span status = %v", test.code, span.Status)
		}
		if got := spanAttributes(span)["rpc.grpc.status_code"]; got != strconv.Itoa(int(test.code)) {
			t.Fatalf("%v: rpc.grpc.status_code = %s", test.code, got)
		}
		rpcLines := logs.FilterMessage("identity_rpc").All()
		if len(rpcLines) != 1 {
			t.Fatalf("%v: identity_rpc lines = %d", test.code, len(rpcLines))
		}
		fields := rpcLines[0].ContextMap()
		if fields["status"] != test.code.String() || fields["outcome"] != "failure" || fields["operation"] != createAccount ||
			fields["request_id"] != "request-1" || fields["trace_id"] != span.SpanContext.TraceID().String() {
			t.Fatalf("%v: identity_rpc = %v", test.code, fields)
		}
		if _, ok := fields["duration"].(time.Duration); !ok {
			t.Fatalf("%v: duration = %#v", test.code, fields["duration"])
		}
	}
}

func TestObservedRequestsMarkHTTPServerErrors(t *testing.T) {
	for status, wantError := range map[int]bool{http.StatusBadRequest: false, http.StatusServiceUnavailable: true} {
		exporter := recordSpans(t)
		core, logs := observer.New(zap.InfoLevel)
		handler := observeRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}), zap.New(core), nil)
		handler.ServeHTTP(httptest.NewRecorder(), restRequest(t, http.MethodPost, "/v1/accounts", `{}`))

		span := onlySpan(t, exporter)
		if got := span.Status.Code == otelcodes.Error; got != wantError {
			t.Fatalf("status %d: error status = %v", status, got)
		}
		if got := logs.All()[0].ContextMap(); got["status"] != int64(status) || got["outcome"] != "failure" {
			t.Fatalf("status %d: request line = %v", status, got)
		}
	}
}

const responseCookie = "secret-cookie-name=secret-cookie-value"

// cookieIdentityHandler sets a Set-Cookie response header on CreateAccount.
type cookieIdentityHandler struct {
	stubIdentityHandler
}

func (h cookieIdentityHandler) CreateAccount(ctx context.Context, request *identityv1.CreateAccountRequest) (*identityv1.CreateAccountResponse, error) {
	if err := grpc.SetHeader(ctx, metadata.Pairs("set-cookie", responseCookie)); err != nil {
		return nil, err
	}
	return h.stubIdentityHandler.CreateAccount(ctx, request)
}

func TestPublicHandlerSpansOmitRequestData(t *testing.T) {
	createAccount := identityv1.IdentityService_CreateAccount_FullMethodName
	// cookieHeader names the response header that carries responseCookie.
	// The REST gateway prefixes the gRPC header metadata with Grpc-Metadata-.
	for name, test := range map[string]struct {
		request      func(*testing.T) *http.Request
		cookieHeader string
	}{
		"REST": {
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return restRequest(t, http.MethodPost, "/v1/accounts?email=secret-user@example.com&code=secret-code",
					`{"email":"secret-user@example.com","password":"secret-password"}`)
			},
			cookieHeader: "Grpc-Metadata-Set-Cookie",
		},
		"gRPC": {
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return grpcRequest(t, createAccount, &identityv1.CreateAccountRequest{Email: "secret-user@example.com", Password: "secret-password"})
			},
			cookieHeader: "Set-Cookie",
		},
		"provider callback": {
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return restRequest(t, http.MethodGet, "/v1/provider-login-callbacks/google?code=secret-code&state=secret-state", "")
			},
			cookieHeader: "Set-Cookie",
		},
	} {
		t.Run(name, func(t *testing.T) {
			exporter := recordSpans(t)
			core, logs := observer.New(zap.InfoLevel)
			callback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Add("Set-Cookie", responseCookie)
				w.WriteHeader(http.StatusFound)
			})
			public, err := newPublicHandler(t.Context(), cookieIdentityHandler{}, callback, zap.New(core), nil)
			if err != nil {
				t.Fatal(err)
			}
			incoming := test.request(t)
			incoming.Header.Set("Authorization", "Bearer secret-access-token")
			incoming.Header.Set("Cookie", "session=secret-cookie")
			response := httptest.NewRecorder()
			public.ServeHTTP(response, incoming)
			if !slices.Contains(response.Header().Values(test.cookieHeader), responseCookie) {
				t.Fatalf("response headers = %v, want %s: %s", response.Header(), test.cookieHeader, responseCookie)
			}

			span := onlySpan(t, exporter)
			recorded := []any{span.Name, span.Attributes, span.Status, span.Events}
			for _, entry := range logs.All() {
				recorded = append(recorded, entry.Message, entry.ContextMap())
			}
			telemetry := fmt.Sprint(recorded...)
			if logs.Len() == 0 || strings.Contains(telemetry, "secret-") || strings.Contains(telemetry, "subject-1") {
				t.Fatalf("telemetry contains request data: %s", telemetry)
			}
		})
	}
}
