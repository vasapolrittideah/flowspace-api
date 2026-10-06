//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redpanda"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/vasapolrittideah/flowspace-api/services/identity/db/migrations"
	inboundevent "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/event"
	deliverycrypto "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	outboundevent "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/event"
	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

// recordingEmailSender keeps the address and the code of each sent email.
type recordingEmailSender struct{ sent []string }

func (s *recordingEmailSender) Send(_ context.Context, email, code, _ string) error {
	s.sent = append(s.sent, email, code)
	return nil
}

func (s *recordingEmailSender) SendPasswordChangeNotice(context.Context, string) error { return nil }

// startDeliveryStack starts PostgreSQL with the Identity schema and Redpanda
// with one delivery topic, and returns the pool, the broker seed, the
// publisher, and the topic.
func startDeliveryStack(ctx context.Context, t *testing.T) (*pgxpool.Pool, string, *outboundevent.Publisher, string) {
	t.Helper()
	database, err := postgrescontainer.Run(ctx, "postgres:18-alpine",
		postgrescontainer.WithDatabase("identity"), postgrescontainer.WithUsername("identity"),
		postgrescontainer.WithPassword("identity"), postgrescontainer.BasicWaitStrategies())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(database) })
	dsn, err := database.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.Files)
	if err := goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	broker, err := redpanda.Run(ctx, "docker.redpanda.com/redpandadata/redpanda:v25.2.4")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(broker) })
	seed, err := broker.KafkaSeedBroker(ctx)
	if err != nil {
		t.Fatal(err)
	}
	registryURL, err := broker.SchemaRegistryAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := sr.NewClient(sr.URLs(registryURL))
	if err != nil {
		t.Fatal(err)
	}
	producer, err := kgo.NewClient(kgo.SeedBrokers(seed), kgo.RecordDeliveryTimeout(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(producer.Close)
	const topic = "identity-signup-delivery-test"
	if _, err := kadm.NewClient(producer).CreateTopic(ctx, 1, 1, nil, topic); err != nil {
		t.Fatal(err)
	}
	publisher, err := outboundevent.NewPublisher(ctx, producer, registry, topic)
	if err != nil {
		t.Fatal(err)
	}
	return pool, seed, publisher, topic
}

// lastSpanNamed returns the last recorded span with name.
func lastSpanNamed(t *testing.T, spans *tracetest.InMemoryExporter, name string) tracetest.SpanStub {
	t.Helper()
	var last *tracetest.SpanStub
	for _, span := range spans.GetSpans() {
		if span.Name == name {
			last = &span
		}
	}
	if last == nil {
		t.Fatalf("no %s span", name)
	}
	return *last
}

// assertDeliveryContinuesRelay makes sure that the last delivery span is a
// consumer child of the last producer span of the relay, and that the last
// email_delivery_processed line has its trace ID. It returns the relay span.
func assertDeliveryContinuesRelay(t *testing.T, logs *observer.ObservedLogs, spans *tracetest.InMemoryExporter) tracetest.SpanStub {
	t.Helper()
	relaySpan := lastSpanNamed(t, spans, "identity.outbox_publish")
	deliverySpan := lastSpanNamed(t, spans, "identity.email_delivery")
	if relaySpan.SpanKind != trace.SpanKindProducer || deliverySpan.SpanKind != trace.SpanKindConsumer ||
		deliverySpan.Parent.SpanID() != relaySpan.SpanContext.SpanID() || deliverySpan.SpanContext.TraceID() != relaySpan.SpanContext.TraceID() {
		t.Fatalf("relay span = %+v, delivery span = %+v", relaySpan, deliverySpan)
	}
	processed := logs.FilterMessage("email_delivery_processed").All()
	if len(processed) == 0 {
		t.Fatal("no email_delivery_processed line")
	}
	fields := processed[len(processed)-1].ContextMap()
	if fields["trace_id"] != deliverySpan.SpanContext.TraceID().String() || fields["operation"] != "identity.email_delivery" {
		t.Fatalf("email_delivery_processed = %v", fields)
	}
	return relaySpan
}

// assertNoSecrets fails when a log line or a span holds one of secrets. The
// text leaves out trace IDs and numbers, because their random digits can
// contain a six-digit code by chance.
func assertNoSecrets(t *testing.T, logs *observer.ObservedLogs, spans *tracetest.InMemoryExporter, secrets []string) {
	t.Helper()
	var telemetry strings.Builder
	for _, entry := range logs.All() {
		telemetry.WriteString(entry.Message + "\n")
		for _, field := range entry.Context {
			if field.Key == "trace_id" {
				continue
			}
			telemetry.WriteString(field.Key + "=" + field.String + "\n")
			if field.Interface != nil {
				fmt.Fprintf(&telemetry, "%v\n", field.Interface)
			}
		}
	}
	writeAttributes := func(attributes []attribute.KeyValue) {
		for _, kv := range attributes {
			telemetry.WriteString(string(kv.Key) + "\n")
			if kv.Value.Type() == attribute.STRING || kv.Value.Type() == attribute.STRINGSLICE {
				telemetry.WriteString(kv.Value.String() + "\n")
			}
		}
	}
	for _, span := range spans.GetSpans() {
		telemetry.WriteString(span.Name + "\n" + span.Status.Description + "\n")
		writeAttributes(span.Attributes)
		for _, event := range span.Events {
			telemetry.WriteString(event.Name + "\n")
			writeAttributes(event.Attributes)
		}
		for _, link := range span.Links {
			writeAttributes(link.Attributes)
		}
	}
	for _, secret := range secrets {
		if strings.Contains(telemetry.String(), secret) {
			t.Fatalf("telemetry contains a signup secret: %s", telemetry.String())
		}
	}
}

// TestSignupEmailDeliveryTrace proves that a signup request, the outbox
// publish, and the email delivery share one trace, and that a signup outside
// a request span gets a new root trace in the relay.
func TestSignupEmailDeliveryTrace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	spans := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spans))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	pool, seed, publisher, topic := startDeliveryStack(ctx, t)
	protector, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{3}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	_, signingKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := token.NewSigner(signingKey, "trace-test", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	signup := app.NewSignupService(identitypostgres.NewAccountRepository(pool), signer, protector,
		func(context.Context, string) error { return nil }, func(context.Context, string) (bool, error) { return false, nil },
		bytes.Repeat([]byte{2}, 32))
	relay := outboundevent.NewOutboxRelay(identitypostgres.NewOutboxRepository(pool), publisher.Publish, "trace-relay", logger)
	sender := &recordingEmailSender{}
	worker, err := inboundevent.NewEmailWorker(seed, topic, "identity-signup-delivery-group",
		identitypostgres.NewDeliveryRepository(pool), protector, sender, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	deliver := func(signupCtx context.Context, email string) tracetest.SpanStub {
		t.Helper()
		if _, err := signup.CreateAccount(signupCtx, inbound.CreateAccountInput{
			Email: email, Password: "correct horse battery staple", Source: "192.0.2.10",
		}); err != nil {
			t.Fatal(err)
		}
		if worked, err := relay.RunOnce(ctx); !worked || err != nil {
			t.Fatalf("relay = %v, %v", worked, err)
		}
		if worked, err := worker.RunOnce(ctx); !worked || err != nil {
			t.Fatalf("worker = %v, %v", worked, err)
		}
		return assertDeliveryContinuesRelay(t, logs, spans)
	}

	requestCtx, request := otel.Tracer("test").Start(ctx, "POST /v1/accounts", trace.WithSpanKind(trace.SpanKindServer))
	relaySpan := deliver(requestCtx, "Traced-Signup@example.com")
	request.End()
	if relaySpan.Parent.SpanID() != request.SpanContext().SpanID() || relaySpan.SpanContext.TraceID() != request.SpanContext().TraceID() {
		t.Fatalf("relay parent = %v, request = %v", relaySpan.Parent, request.SpanContext())
	}
	rootSpan := deliver(ctx, "Untraced-Signup@example.com")
	if rootSpan.Parent.IsValid() || rootSpan.SpanContext.TraceID() == request.SpanContext().TraceID() {
		t.Fatalf("relay span without stored context = %+v", rootSpan)
	}

	if len(sender.sent) != 4 {
		t.Fatalf("sent emails = %d, want 2", len(sender.sent)/2)
	}
	// sender.sent holds both addresses and both codes.
	assertNoSecrets(t, logs, spans, append([]string{"correct horse battery staple"}, sender.sent...))
}
