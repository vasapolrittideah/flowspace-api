package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	outbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/out"
)

func TestTokenVerifierRequiresLiveVerifiedSession(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	jwksState := new(atomic.Int32)
	server := httptest.NewServer(testJWKSHandler(jwksState, public))
	defer server.Close()
	issuer, audience := "urn:flowspace:identity:test", "flowspace-api"
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: jose.JSONWebKey{Key: private, KeyID: "key-1"}}, (&jose.SignerOptions{}).WithType("at+jwt"))
	if err != nil {
		t.Fatal(err)
	}
	accessToken, err := jwt.Signed(signer).Claims(jwt.Claims{
		Issuer: issuer, Subject: "subject-1", Audience: jwt.Audience{audience},
		IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)), Expiry: jwt.NewNumericDate(time.Now().Add(time.Minute)), ID: "token-1",
	}).Claims(struct {
		SessionID string `json:"sid"`
	}{SessionID: "session-1"}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	checks := 0
	verified := false
	verifier := &TokenVerifier{jwksURL: server.URL, issuer: issuer, audience: audience, client: server.Client(), check: func(_ context.Context, request *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
		checks++
		if request.GetSubject() != "subject-1" || request.GetSessionId() != "session-1" {
			t.Fatalf("check request = %+v", request)
		}
		return &identityv1.CheckSessionResponse{EmailVerified: verified}, nil
	}}
	if _, err := verifier.VerifyToken(t.Context(), accessToken); !errors.Is(err, outbound.ErrEmailUnverified) || checks != 1 {
		t.Fatalf("unverified: error = %v, checks = %d", err, checks)
	}
	verified = true
	if subject, err := verifier.VerifyToken(t.Context(), accessToken); err != nil || subject != "subject-1" || checks != 2 {
		t.Fatalf("verified: subject = %q, error = %v, checks = %d", subject, err, checks)
	}
	verifier.check = func(context.Context, *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
		return nil, status.Error(codes.Unauthenticated, "inactive")
	}
	if subject, err := verifier.VerifyToken(t.Context(), accessToken); subject != "" || !errors.Is(err, outbound.ErrUnauthenticated) {
		t.Fatalf("revoked: subject = %q, error = %v", subject, err)
	}
	verifier.check = func(context.Context, *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
		return nil, status.Error(codes.Unavailable, "offline")
	}
	if subject, err := verifier.VerifyToken(t.Context(), accessToken); subject != "" || !errors.Is(err, outbound.ErrIdentityUnavailable) {
		t.Fatalf("outage: subject = %q, error = %v", subject, err)
	}
	assertJWKSFailures(t, verifier, accessToken, jwksState)
}

func testJWKSHandler(state *atomic.Int32, public ed25519.PublicKey) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch state.Load() {
		case 1:
			http.Error(w, "offline", http.StatusServiceUnavailable)
		case 2:
			_, _ = w.Write([]byte("invalid JSON"))
		case 3:
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{})
		default:
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: public, KeyID: "key-1", Algorithm: "EdDSA", Use: "sig"}}})
		}
	})
}

func assertJWKSFailures(t *testing.T, verifier *TokenVerifier, accessToken string, jwksState *atomic.Int32) {
	t.Helper()
	for _, state := range []int32{1, 2, 3} {
		jwksState.Store(state)
		_, err := verifier.VerifyToken(t.Context(), accessToken)
		want := outbound.ErrIdentityUnavailable
		if state == 3 {
			want = outbound.ErrUnauthenticated
		}
		if !errors.Is(err, want) {
			t.Fatalf("JWKS state %d error = %v, want %v", state, err, want)
		}
	}
}

const (
	testSubject   = "subject-1"
	testSessionID = "session-1"
	parentTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	parentSpanID  = "00f067aa0ba902b7"
)

// stubSessionServer answers CheckSession after wait and sends the incoming
// metadata of each call to received. held is the time that the last call
// spent in the handler.
type stubSessionServer struct {
	identityv1.UnimplementedIdentityServiceServer
	received chan metadata.MD
	verified bool
	err      error
	wait     time.Duration
	held     time.Duration
}

func (s *stubSessionServer) CheckSession(ctx context.Context, request *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
	started := time.Now()
	defer func() { s.held = time.Since(started) }()
	time.Sleep(s.wait)
	incoming, _ := metadata.FromIncomingContext(ctx)
	s.received <- incoming
	if request.GetSubject() != testSubject || request.GetSessionId() != testSessionID {
		return nil, status.Error(codes.InvalidArgument, "unexpected session")
	}
	return &identityv1.CheckSessionResponse{EmailVerified: s.verified}, s.err
}

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

// sessionVerifier returns a verifier whose session checks go to server
// through the connection that dialSession builds, and a valid access token.
func sessionVerifier(t *testing.T, server *stubSessionServer) (*TokenVerifier, string) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	identityv1.RegisterIdentityServiceServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	conn, err := dialSession("passthrough:///bufnet", insecure.NewCredentials(),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(testJWKSHandler(new(atomic.Int32), public))
	t.Cleanup(jwks.Close)
	issuer, audience := "urn:flowspace:identity:test", "flowspace-api"
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: jose.JSONWebKey{Key: private, KeyID: "key-1"}},
		(&jose.SignerOptions{}).WithType("at+jwt"))
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.Signed(signer).Claims(jwt.Claims{
		Issuer: issuer, Subject: testSubject, Audience: jwt.Audience{audience},
		IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)), Expiry: jwt.NewNumericDate(time.Now().Add(time.Minute)), ID: "token-1",
	}).Claims(struct {
		SessionID string `json:"sid"`
	}{SessionID: testSessionID}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	verifier := &TokenVerifier{
		jwksURL: jwks.URL, issuer: issuer, audience: audience, client: jwks.Client(),
		check: newSessionCheck(conn),
	}
	return verifier, token
}

// inWorkspaceRequest runs check in a Workspace server span under a remote
// parent with traceState and with the request ID metadata pairs. It returns
// the span context of the Workspace span.
func inWorkspaceRequest(t *testing.T, traceState string, pairs []string, check func(context.Context)) trace.SpanContext {
	t.Helper()
	traceID, _ := trace.TraceIDFromHex(parentTraceID)
	spanID, _ := trace.SpanIDFromHex(parentSpanID)
	state, err := trace.ParseTraceState(traceState)
	if err != nil {
		t.Fatal(err)
	}
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, TraceState: state, Remote: true,
	})
	ctx := metadata.NewIncomingContext(trace.ContextWithRemoteSpanContext(t.Context(), parent), metadata.Pairs(pairs...))
	ctx, span := otel.Tracer("test").Start(ctx, "GET /v1/workspaces/{workspace_id}", trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()
	check(ctx)
	return span.SpanContext()
}

func onlyClientSpan(t *testing.T, exporter *tracetest.InMemoryExporter) tracetest.SpanStub {
	t.Helper()
	var clients []tracetest.SpanStub
	for _, span := range exporter.GetSpans() {
		if span.SpanKind == trace.SpanKindClient {
			clients = append(clients, span)
		}
	}
	if len(clients) != 1 {
		t.Fatalf("recorded %d client spans, want 1", len(clients))
	}
	return clients[0]
}

// assertNoSessionData fails when the span holds more than the gRPC status
// code, so it cannot hold the subject, the session, the token, or client
// certificate data.
func assertNoSessionData(t *testing.T, span tracetest.SpanStub, token string) {
	t.Helper()
	if len(span.Attributes) != 1 || span.Attributes[0].Key != "rpc.grpc.status_code" || len(span.Events) != 0 ||
		len(span.Links) != 0 || span.Status.Description != "" {
		t.Fatalf("span has more than the status code: %+v", span)
	}
	for _, value := range []string{testSubject, testSessionID, token} {
		if strings.Contains(span.Name, value) {
			t.Fatalf("span name contains %q", value)
		}
	}
}

func TestSessionCheckCreatesAChildClientSpanAndForwardsItsContext(t *testing.T) {
	exporter := recordSpans(t)
	server := &stubSessionServer{received: make(chan metadata.MD, 1), verified: true}
	verifier, token := sessionVerifier(t, server)
	workspaceSpan := inWorkspaceRequest(t, "vendor=value", []string{"x-request-id", "request-1"}, func(ctx context.Context) {
		if subject, err := verifier.VerifyToken(ctx, token); err != nil || subject != testSubject {
			t.Fatalf("subject = %q, error = %v", subject, err)
		}
	})

	client := onlyClientSpan(t, exporter)
	if client.Name != identityv1.IdentityService_CheckSession_FullMethodName ||
		client.Parent.SpanID() != workspaceSpan.SpanID() || client.SpanContext.TraceID().String() != parentTraceID {
		t.Fatalf("client span = %q, parent %v, trace %v", client.Name, client.Parent.SpanID(), client.SpanContext.TraceID())
	}
	sent := <-server.received
	wantTraceparent := fmt.Sprintf("00-%s-%s-01", parentTraceID, client.SpanContext.SpanID())
	if got := sent.Get("traceparent"); len(got) != 1 || got[0] != wantTraceparent {
		t.Fatalf("traceparent = %v, want %s", got, wantTraceparent)
	}
	if got := sent.Get("tracestate"); len(got) != 1 || got[0] != "vendor=value" {
		t.Fatalf("tracestate = %v", got)
	}
	if got := sent.Get("x-request-id"); len(got) != 1 || got[0] != "request-1" {
		t.Fatalf("x-request-id = %v", got)
	}
	assertNoSessionData(t, client, token)
}

func TestSessionCheckSendsOnlyTheContextThatExists(t *testing.T) {
	for _, test := range []struct {
		name  string
		pairs []string
	}{
		{"missing request ID", nil},
		{"invalid request ID", []string{"x-request-id", "has space"}},
		{"two request IDs", []string{"x-request-id", "first", "x-request-id", "second"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recordSpans(t)
			server := &stubSessionServer{received: make(chan metadata.MD, 1), verified: true}
			verifier, token := sessionVerifier(t, server)
			inWorkspaceRequest(t, "", test.pairs, func(ctx context.Context) {
				if subject, err := verifier.VerifyToken(ctx, token); err != nil || subject != testSubject {
					t.Fatalf("subject = %q, error = %v", subject, err)
				}
			})

			sent := <-server.received
			if len(sent.Get("traceparent")) != 1 || sent.Get("tracestate") != nil || sent.Get("x-request-id") != nil {
				t.Fatalf("sent metadata = %v", sent)
			}
		})
	}
}

func TestSessionCheckReplacesEarlierCorrelationMetadata(t *testing.T) {
	recordSpans(t)
	server := &stubSessionServer{received: make(chan metadata.MD, 1), verified: true}
	verifier, token := sessionVerifier(t, server)
	inWorkspaceRequest(t, "", []string{"x-request-id", "request-1"}, func(ctx context.Context) {
		ctx = metadata.AppendToOutgoingContext(ctx, "traceparent", "00-"+strings.Repeat("1", 32)+"-"+strings.Repeat("2", 16)+"-01",
			"tracestate", "stale=value", "x-request-id", "stale-request", "other", "kept")
		if subject, err := verifier.VerifyToken(ctx, token); err != nil || subject != testSubject {
			t.Fatalf("subject = %q, error = %v", subject, err)
		}
	})

	sent := <-server.received
	if got := sent.Get("traceparent"); len(got) != 1 || !strings.Contains(got[0], parentTraceID) {
		t.Fatalf("traceparent = %v", got)
	}
	if sent.Get("tracestate") != nil || len(sent.Get("x-request-id")) != 1 || sent.Get("x-request-id")[0] != "request-1" ||
		len(sent.Get("other")) != 1 || sent.Get("other")[0] != "kept" {
		t.Fatalf("sent metadata = %v", sent)
	}
}

func TestSessionCheckTracingKeepsTheVerificationResult(t *testing.T) {
	for _, test := range []struct {
		name      string
		verified  bool
		err       error
		want      error
		code      codes.Code
		wantError bool
	}{
		{"unverified", false, nil, outbound.ErrEmailUnverified, codes.OK, false},
		{"revoked", false, status.Error(codes.Unauthenticated, "inactive"), outbound.ErrUnauthenticated, codes.Unauthenticated, false},
		{"outage", false, status.Error(codes.Unavailable, "offline"), outbound.ErrIdentityUnavailable, codes.Unavailable, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			exporter := recordSpans(t)
			server := &stubSessionServer{received: make(chan metadata.MD, 1), verified: test.verified, err: test.err}
			verifier, token := sessionVerifier(t, server)
			inWorkspaceRequest(t, "", []string{"x-request-id", "request-1"}, func(ctx context.Context) {
				if subject, err := verifier.VerifyToken(ctx, token); subject != "" || !errors.Is(err, test.want) {
					t.Fatalf("subject = %q, error = %v, want %v", subject, err, test.want)
				}
			})

			client := onlyClientSpan(t, exporter)
			if got := client.Attributes[0].Value.AsInt64(); got != int64(test.code) {
				t.Fatalf("rpc.grpc.status_code = %d, want %d", got, test.code)
			}
			if got := client.Status.Code == otelcodes.Error; got != test.wantError {
				t.Fatalf("span status = %v", client.Status)
			}
			assertNoSessionData(t, client, token)
		})
	}
}

// recordMetrics installs a global meter provider with a manual reader. Tests
// that call it must not run in parallel.
func recordMetrics(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	return reader
}

// onlyClientDuration returns the only rpc.client.call.duration point, which
// must hold 1 call.
func onlyClientDuration(t *testing.T, reader *sdkmetric.ManualReader) metricdata.HistogramDataPoint[float64] {
	t.Helper()
	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatal(err)
	}
	var points []metricdata.HistogramDataPoint[float64]
	for _, scope := range collected.ScopeMetrics {
		for _, measured := range scope.Metrics {
			histogram, ok := measured.Data.(metricdata.Histogram[float64])
			if measured.Name != "rpc.client.call.duration" || measured.Unit != "s" || !ok {
				t.Fatalf("unexpected metric %q with unit %q", measured.Name, measured.Unit)
			}
			points = append(points, histogram.DataPoints...)
		}
	}
	if len(points) != 1 || points[0].Count != 1 {
		t.Fatalf("rpc.client.call.duration points = %+v, want one call", points)
	}
	return points[0]
}

func TestSessionCheckRecordsClientDuration(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		code codes.Code
	}{
		{"verified", nil, codes.OK},
		{"revoked", status.Error(codes.Unauthenticated, "inactive"), codes.Unauthenticated},
		{"outage", status.Error(codes.Unavailable, "offline"), codes.Unavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := recordMetrics(t)
			server := &stubSessionServer{received: make(chan metadata.MD, 1), verified: true, err: test.err, wait: 20 * time.Millisecond}
			verifier, token := sessionVerifier(t, server)
			started := time.Now()
			_, _ = verifier.VerifyToken(t.Context(), token)
			elapsed := time.Since(started).Seconds()

			// The duration covers the time that the call spent in Identity.
			point := onlyClientDuration(t, reader)
			if held := server.held.Seconds(); point.Sum < held || point.Sum > elapsed {
				t.Fatalf("duration = %gs, want at least %gs and at most %gs", point.Sum, held, elapsed)
			}
			// The exact attribute names and types rule out the subject, the
			// session, the token, and client certificate data.
			method, hasMethod := point.Attributes.Value("rpc.method")
			code, hasCode := point.Attributes.Value("rpc.grpc.status_code")
			if point.Attributes.Len() != 2 || !hasMethod || method.Type() != attribute.STRING || !hasCode || code.Type() != attribute.INT64 ||
				method.AsString() != strings.TrimPrefix(identityv1.IdentityService_CheckSession_FullMethodName, "/") || code.AsInt64() != int64(test.code) {
				t.Fatalf("duration attributes = %v, want status %d", point.Attributes.ToSlice(), test.code)
			}
		})
	}
}
