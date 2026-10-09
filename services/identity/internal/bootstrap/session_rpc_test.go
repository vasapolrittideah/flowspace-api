package bootstrap

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type testSessionCheckService struct{}

func (testSessionCheckService) CheckSession(_ context.Context, input inbound.CheckSessionInput) (bool, error) {
	switch input.Subject {
	case "subject-1":
		return input.SessionID == "session-1", nil
	case "inactive":
		return false, outbound.ErrUnauthenticated
	default:
		return false, errors.New("database failed")
	}
}

// startSessionRPC serves the session listener with mutual TLS and returns a
// client for each certificate name of the TLS fixture.
func startSessionRPC(t *testing.T, service inbound.SessionCheckService, meter metric.Meter, logger *zap.Logger,
) func(string) identityv1.IdentityServiceClient {
	t.Helper()
	fixture := testSessionTLSFixture(t)
	tlsConfig, err := newSessionTLSConfig(fixture.config)
	if err != nil {
		t.Fatal(err)
	}
	server, err := newSessionGRPCServer(tlsConfig, service, meter, logger)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		<-finished
	})
	return func(name string) identityv1.IdentityServiceClient {
		t.Helper()
		clientTLS := &tls.Config{RootCAs: fixture.roots, ServerName: sessionTestServerName, MinVersion: tls.VersionTLS13}
		if certificate, ok := fixture.clients[name]; ok {
			clientTLS.Certificates = []tls.Certificate{certificate}
		}
		connection, err := grpc.NewClient("passthrough:///"+listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = connection.Close() })
		return identityv1.NewIdentityServiceClient(connection)
	}
}

// checkSession calls CheckSession for subject and session-1 with the
// metadata pairs.
func checkSession(t *testing.T, client identityv1.IdentityServiceClient, subject string, pairs ...string,
) (*identityv1.CheckSessionResponse, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
	return client.CheckSession(ctx, &identityv1.CheckSessionRequest{Subject: subject, SessionId: "session-1"})
}

func TestPrivateSessionRPC(t *testing.T) {
	exporter := recordSpans(t)
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	core, logs := observer.New(zap.InfoLevel)
	client := startSessionRPC(t, testSessionCheckService{}, provider.Meter("test"), zap.New(core))
	correlation := []string{"traceparent", parentTraceparent, "x-request-id", "request-1"}
	approved := client("approved")
	response, err := checkSession(t, approved, "subject-1")
	if err != nil || !response.GetEmailVerified() {
		t.Fatalf("approved caller = %+v, %v", response, err)
	}
	ctx, cancel := context.WithTimeout(metadata.AppendToOutgoingContext(t.Context(), correlation...), 3*time.Second)
	defer cancel()
	if _, err := approved.CreateAccount(ctx, &identityv1.CreateAccountRequest{}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("private public method = %v", err)
	}
	for _, test := range []struct {
		subject string
		want    codes.Code
	}{
		{"inactive", codes.Unauthenticated},
		{"database", codes.Unavailable},
	} {
		if _, err := checkSession(t, approved, test.subject); status.Code(err) != test.want {
			t.Fatalf("%s = %v", test.subject, err)
		}
	}
	// Valid correlation metadata does not admit a caller without an approved
	// certificate.
	for _, name := range []string{"missing", "wrong-service", "wrong-key"} {
		if _, err := checkSession(t, client(name), "subject-1", correlation...); status.Code(err) != codes.Unavailable {
			t.Fatalf("%s caller = %v", name, err)
		}
	}
	assertSessionObservability(t, logs, reader)
	if spans := exporter.GetSpans(); len(spans) != 3 {
		t.Fatalf("recorded %d session spans, want 3", len(spans))
	}
}

// recordingSessionCheckService sends the span context of each call to
// spanContexts.
type recordingSessionCheckService struct {
	testSessionCheckService
	spanContexts chan trace.SpanContext
}

func (s recordingSessionCheckService) CheckSession(ctx context.Context, input inbound.CheckSessionInput) (bool, error) {
	s.spanContexts <- trace.SpanContextFromContext(ctx)
	return s.testSessionCheckService.CheckSession(ctx, input)
}

func onlySessionCheckLine(t *testing.T, logs *observer.ObservedLogs) map[string]any {
	t.Helper()
	lines := logs.TakeAll()
	if len(lines) != 1 || lines[0].Message != "identity_session_check" {
		t.Fatalf("session logs = %v", lines)
	}
	return lines[0].ContextMap()
}

// assertNoSessionData fails when the span or the logs contain a session
// value. The test permits only the rpc.method and rpc.grpc.status_code
// span attributes, so the span cannot hold token or client certificate data.
func assertNoSessionData(t *testing.T, span tracetest.SpanStub, logs map[string]any, values ...string) {
	t.Helper()
	keys := slices.Sorted(maps.Keys(spanAttributes(span)))
	if !slices.Equal(keys, []string{"rpc.grpc.status_code", "rpc.method"}) || len(span.Events) != 0 ||
		len(span.Links) != 0 || span.Status.Description != "" {
		t.Fatalf("span has more than the session attributes: %+v", span)
	}
	recorded := fmt.Sprint(span.Name, spanAttributes(span), logs)
	for _, value := range values {
		if strings.Contains(recorded, value) {
			t.Fatalf("telemetry contains %q: %s", value, recorded)
		}
	}
}

// assertContinuedSessionSpan makes sure that span is the successful
// CheckSession server span under parentTraceparent with the
// vendor=value trace state.
func assertContinuedSessionSpan(t *testing.T, span tracetest.SpanStub) {
	t.Helper()
	checkSessionMethod := identityv1.IdentityService_CheckSession_FullMethodName
	if span.Name != checkSessionMethod || span.SpanKind != trace.SpanKindServer {
		t.Fatalf("span = %q, kind %v", span.Name, span.SpanKind)
	}
	if !span.Parent.IsRemote() || span.Parent.TraceID().String() != parentTraceID || span.Parent.SpanID().String() != parentSpanID ||
		span.SpanContext.TraceState().String() != "vendor=value" {
		t.Fatalf("parent = %+v, trace state %q", span.Parent, span.SpanContext.TraceState().String())
	}
	if got := spanAttributes(span); got["rpc.method"] != strings.TrimPrefix(checkSessionMethod, "/") || got["rpc.grpc.status_code"] != "0" ||
		span.Status.Code != otelcodes.Unset {
		t.Fatalf("attributes = %v, status = %v", got, span.Status)
	}
}

func TestSessionRPCContinuesIncomingTrace(t *testing.T) {
	exporter := recordSpans(t)
	core, logs := observer.New(zap.InfoLevel)
	service := recordingSessionCheckService{spanContexts: make(chan trace.SpanContext, 1)}
	client := startSessionRPC(t, service, noop.NewMeterProvider().Meter("test"), zap.New(core))("approved")

	response, err := checkSession(t, client, "subject-1",
		"traceparent", parentTraceparent, "tracestate", "vendor=value", "x-request-id", "request-1")
	if err != nil || !response.GetEmailVerified() {
		t.Fatalf("session check = %+v, %v", response, err)
	}

	span := onlySpan(t, exporter)
	assertContinuedSessionSpan(t, span)
	if seen := <-service.spanContexts; seen.SpanID() != span.SpanContext.SpanID() {
		t.Fatalf("service span = %v, want %v", seen.SpanID(), span.SpanContext.SpanID())
	}
	checkSessionMethod := identityv1.IdentityService_CheckSession_FullMethodName
	fields := onlySessionCheckLine(t, logs)
	if fields["request_id"] != "request-1" || fields["trace_id"] != parentTraceID || fields["operation"] != checkSessionMethod ||
		fields["status"] != "OK" || fields["outcome"] != "success" || fields["duration"] == nil {
		t.Fatalf("identity_session_check = %v", fields)
	}
	assertNoSessionData(t, span, fields, "subject-1", "session-1")
}

func TestSessionRPCRecordsGRPCStatus(t *testing.T) {
	for _, test := range []struct {
		subject   string
		code      codes.Code
		outcome   string
		wantError bool
	}{
		{"subject-1", codes.OK, "success", false},
		{"inactive", codes.Unauthenticated, "failure", false},
		{"database", codes.Unavailable, "failure", true},
	} {
		t.Run(test.code.String(), func(t *testing.T) {
			exporter := recordSpans(t)
			core, logs := observer.New(zap.InfoLevel)
			client := startSessionRPC(t, testSessionCheckService{}, noop.NewMeterProvider().Meter("test"), zap.New(core))("approved")

			if _, err := checkSession(t, client, test.subject, "x-request-id", "request-1"); status.Code(err) != test.code {
				t.Fatalf("session check = %v, want %v", err, test.code)
			}

			span := onlySpan(t, exporter)
			if got := spanAttributes(span)["rpc.grpc.status_code"]; got != strconv.Itoa(int(test.code)) {
				t.Fatalf("rpc.grpc.status_code = %s", got)
			}
			if got := span.Status.Code == otelcodes.Error; got != test.wantError {
				t.Fatalf("span status = %v", span.Status)
			}
			fields := onlySessionCheckLine(t, logs)
			if fields["status"] != test.code.String() || fields["outcome"] != test.outcome ||
				fields["trace_id"] != span.SpanContext.TraceID().String() {
				t.Fatalf("identity_session_check = %v", fields)
			}
			assertNoSessionData(t, span, fields, test.subject, "session-1")
		})
	}
}

func TestSessionRPCValidatesForwardedRequestID(t *testing.T) {
	recordSpans(t)
	core, logs := observer.New(zap.InfoLevel)
	client := startSessionRPC(t, testSessionCheckService{}, noop.NewMeterProvider().Meter("test"), zap.New(core))("approved")
	for _, test := range []struct {
		name  string
		pairs []string
		want  string
	}{
		{"one character", []string{"x-request-id", "a"}, "a"},
		{"punctuation", []string{"x-request-id", "Request.ID_3-"}, "Request.ID_3-"},
		{"128 bytes", []string{"x-request-id", strings.Repeat("r", 128)}, strings.Repeat("r", 128)},
		{"missing", nil, ""},
		{"empty", []string{"x-request-id", ""}, ""},
		{"129 bytes", []string{"x-request-id", strings.Repeat("r", 129)}, ""},
		{"space", []string{"x-request-id", "has space"}, ""},
		{"slash", []string{"x-request-id", "has/slash"}, ""},
		{"two values", []string{"x-request-id", "first", "x-request-id", "second"}, ""},
	} {
		pairs := append([]string{"traceparent", parentTraceparent}, test.pairs...)
		response, err := checkSession(t, client, "subject-1", pairs...)
		if err != nil || !response.GetEmailVerified() {
			t.Fatalf("%s: session check = %+v, %v", test.name, response, err)
		}
		fields := onlySessionCheckLine(t, logs)
		if fields["request_id"] != test.want || fields["trace_id"] != parentTraceID {
			t.Fatalf("%s: identity_session_check = %v", test.name, fields)
		}
		for index := 1; test.want == "" && index < len(test.pairs); index += 2 {
			if value := test.pairs[index]; value != "" && strings.Contains(fmt.Sprint(fields), value) {
				t.Fatalf("%s: line contains the rejected request ID: %v", test.name, fields)
			}
		}
	}
}

func TestSessionRPCStartsRootWithoutValidTraceparent(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	for _, traceparent := range []string{"", "not-a-traceparent", "00-00000000000000000000000000000000-" + parentSpanID + "-01"} {
		exporter := recordSpans(t)
		client := startSessionRPC(t, testSessionCheckService{}, noop.NewMeterProvider().Meter("test"), zap.New(core))("approved")
		var pairs []string
		if traceparent != "" {
			pairs = []string{"traceparent", traceparent}
		}
		response, err := checkSession(t, client, "subject-1", pairs...)
		if err != nil || !response.GetEmailVerified() {
			t.Fatalf("traceparent %q: session check = %+v, %v", traceparent, response, err)
		}
		span := onlySpan(t, exporter)
		if span.Parent.IsValid() || !span.SpanContext.IsValid() {
			t.Fatalf("traceparent %q: parent %v, span %v", traceparent, span.Parent, span.SpanContext)
		}
		if fields := onlySessionCheckLine(t, logs); fields["trace_id"] != span.SpanContext.TraceID().String() {
			t.Fatalf("traceparent %q: identity_session_check = %v", traceparent, fields)
		}
	}
}

func assertSessionObservability(t *testing.T, logs *observer.ObservedLogs, reader *sdkmetric.ManualReader) {
	t.Helper()
	if got := fmt.Sprint(logs.All()); strings.Contains(got, "subject-1") || strings.Contains(got, "session-1") ||
		logs.FilterMessage("identity_session_check").Len() != 3 {
		t.Fatalf("unsafe or missing session-check logs: %s", got)
	}
	// Each status has one call, so the sum of its duration equals the
	// duration of its log line.
	want := map[string]float64{}
	for _, line := range logs.FilterMessage("identity_session_check").All() {
		fields := line.ContextMap()
		elapsed, _ := fields["duration"].(time.Duration)
		want[fmt.Sprint(fields["status"])] = elapsed.Seconds()
	}
	got := sessionDurations(t, reader)
	if len(want) != 3 || !maps.Equal(got, want) || slices.Contains(slices.Collect(maps.Values(got)), 0) {
		t.Errorf("rpc.server.call.duration seconds by status = %v, want %v", got, want)
	}
}

// sessionDurations returns the rpc.server.call.duration sum by gRPC status
// name, and fails when a status has more than one call. It permits only the
// rpc.method string and the rpc.grpc.status_code integer, so no subject,
// session, token, or client certificate data can reach a duration. It fails
// when another metric, such as the identity.session_checks counter, exists.
func sessionDurations(t *testing.T, reader *sdkmetric.ManualReader) map[string]float64 {
	t.Helper()
	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatal(err)
	}
	sums := map[string]float64{}
	for _, scope := range collected.ScopeMetrics {
		for _, measured := range scope.Metrics {
			if measured.Name != "rpc.server.call.duration" || measured.Unit != "s" {
				t.Fatalf("unexpected metric %q with unit %q", measured.Name, measured.Unit)
			}
			histogram, ok := measured.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("rpc.server.call.duration data = %T", measured.Data)
			}
			for _, point := range histogram.DataPoints {
				method, hasMethod := point.Attributes.Value("rpc.method")
				code, hasCode := point.Attributes.Value("rpc.grpc.status_code")
				if point.Attributes.Len() != 2 || !hasMethod || method.Type() != attribute.STRING || !hasCode || code.Type() != attribute.INT64 ||
					method.AsString() != strings.TrimPrefix(identityv1.IdentityService_CheckSession_FullMethodName, "/") || point.Count != 1 {
					t.Fatalf("duration point = %v with %d calls", point.Attributes.ToSlice(), point.Count)
				}
				sums[codes.Code(code.AsInt64()).String()] = point.Sum
			}
		}
	}
	return sums
}
