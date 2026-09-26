package bootstrap

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"maps"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type testSessionCheckService struct{}

type sessionMetricMeter struct {
	metric.Meter
	counter *sessionMetricCounter
}

func (m sessionMetricMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return m.counter, nil
}

type sessionMetricCounter struct {
	metric.Int64Counter
	mu       sync.Mutex
	outcomes map[string]int64
	unsafe   bool
}

func (c *sessionMetricCounter) Add(_ context.Context, value int64, options ...metric.AddOption) {
	attributeSet := metric.NewAddConfig(options).Attributes()
	attributes := attributeSet.ToSlice()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(attributes) != 1 || attributes[0].Key != "outcome" {
		c.unsafe = true
		return
	}
	c.outcomes[attributes[0].Value.AsString()] += value
}

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

func TestPrivateSessionRPC(t *testing.T) {
	fixture := testSessionTLSFixture(t)
	counter := &sessionMetricCounter{outcomes: make(map[string]int64)}
	tlsConfig, err := newSessionTLSConfig(fixture.config)
	if err != nil {
		t.Fatal(err)
	}
	core, logs := observer.New(zap.InfoLevel)
	server, err := newSessionGRPCServer(tlsConfig, testSessionCheckService{}, sessionMetricMeter{counter: counter}, zap.New(core))
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
	client := func(name string) identityv1.IdentityServiceClient {
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
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	approved := client("approved")
	response, err := approved.CheckSession(ctx, &identityv1.CheckSessionRequest{Subject: "subject-1", SessionId: "session-1"})
	if err != nil || !response.GetEmailVerified() {
		t.Fatalf("approved caller = %+v, %v", response, err)
	}
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
		_, err := approved.CheckSession(ctx, &identityv1.CheckSessionRequest{Subject: test.subject, SessionId: "session-1"})
		if status.Code(err) != test.want {
			t.Fatalf("%s = %v", test.subject, err)
		}
	}
	for _, name := range []string{"missing", "wrong-service", "wrong-key"} {
		_, err := client(name).CheckSession(ctx, &identityv1.CheckSessionRequest{Subject: "subject-1", SessionId: "session-1"})
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("%s caller = %v", name, err)
		}
	}
	assertSessionObservability(t, logs, counter)
}

func assertSessionObservability(t *testing.T, logs *observer.ObservedLogs, counter *sessionMetricCounter) {
	t.Helper()
	if got := fmt.Sprint(logs.All()); strings.Contains(got, "subject-1") || strings.Contains(got, "session-1") ||
		logs.FilterMessage("identity_session_check").Len() != 3 {
		t.Fatalf("unsafe or missing session-check logs: %s", got)
	}
	counter.mu.Lock()
	defer counter.mu.Unlock()
	if counter.unsafe || !maps.Equal(counter.outcomes, map[string]int64{"OK": 1, "Unauthenticated": 1, "Unavailable": 1}) {
		t.Errorf("unsafe or missing session-check metrics: %v", counter.outcomes)
	}
}
