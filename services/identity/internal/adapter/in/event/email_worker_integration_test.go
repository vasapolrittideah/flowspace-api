//go:build integration

package event_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redpanda"
	"github.com/testcontainers/testcontainers-go/wait"
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
	identityevent "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/event"
	deliverycrypto "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	identityemail "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/email"
	outboxevent "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/event"
	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

// telemetryText joins the text that a log or span can carry. It leaves out
// timestamps, trace IDs, and numeric values, because their random digits can
// contain a six-digit code by chance.
func telemetryText(logs *observer.ObservedLogs, spans tracetest.SpanStubs) string {
	var text strings.Builder
	for _, entry := range logs.All() {
		text.WriteString(entry.Message + "\n")
		for _, field := range entry.Context {
			if field.Key == "trace_id" {
				continue
			}
			text.WriteString(field.Key + "=" + field.String + "\n")
			if field.Interface != nil {
				fmt.Fprintf(&text, "%v\n", field.Interface)
			}
		}
	}
	writeAttributes := func(attributes []attribute.KeyValue) {
		for _, kv := range attributes {
			text.WriteString(string(kv.Key) + "\n")
			if kv.Value.Type() == attribute.STRING || kv.Value.Type() == attribute.STRINGSLICE {
				text.WriteString(kv.Value.Emit() + "\n")
			}
		}
	}
	for _, span := range spans {
		text.WriteString(span.Name + "\n" + span.Status.Description + "\n")
		writeAttributes(span.Attributes)
		for _, event := range span.Events {
			text.WriteString(event.Name + "\n")
			writeAttributes(event.Attributes)
		}
		for _, link := range span.Links {
			writeAttributes(link.Attributes)
		}
	}
	return text.String()
}

func TestTelemetryTextSkipsRandomDigits(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	zap.New(core).Info("email_delivery_processed", zap.String("trace_id", "0123456789abcdef0123456789abcdef"),
		zap.Float64("record_age_seconds", 0.123456), zap.Error(errors.New("to leak@example.com")))
	spans := tracetest.SpanStubs{{
		Name:       "identity.email_delivery",
		Attributes: []attribute.KeyValue{attribute.Int("count", 123456), attribute.String("code", "654321")},
	}}
	text := telemetryText(logs, spans)
	if strings.Contains(text, "123456") {
		t.Fatal("telemetry text contains a trace ID or a numeric value")
	}
	if !strings.Contains(text, "leak@example.com") || !strings.Contains(text, "654321") {
		t.Fatal("telemetry text misses a log error or a span attribute")
	}
}

type crashAfterSendRepository struct{ outbound.DeliveryRepository }

type switchingSender struct{ target outbound.EmailSender }

func (s *switchingSender) Send(ctx context.Context, email, code, purpose string) error {
	return s.target.Send(ctx, email, code, purpose)
}

func (s *switchingSender) SendPasswordChangeNotice(ctx context.Context, email string) error {
	return s.target.SendPasswordChangeNotice(ctx, email)
}

func (r crashAfterSendRepository) WithCurrentDelivery(ctx context.Context, challengeID, purpose string, send func(context.Context, outbound.CurrentDelivery) error) error {
	return r.DeliveryRepository.WithCurrentDelivery(ctx, challengeID, purpose, func(ctx context.Context, delivery outbound.CurrentDelivery) error {
		if err := send(ctx, delivery); err != nil {
			return err
		}
		return errors.New("simulated crash after SMTP acceptance")
	})
}

func TestEmailWorkerMailpitOutageCrashReplayAndStaleEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	spans := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spans))
	previousTracerProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(tracerProvider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousTracerProvider)
		_ = tracerProvider.Shutdown(context.Background())
	})
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
	const topic = "identity-mail-delivery-test"
	if _, err := kadm.NewClient(producer).CreateTopic(ctx, 1, 1, nil, topic); err != nil {
		t.Fatal(err)
	}
	publisher, err := outboxevent.NewPublisher(ctx, producer, registry, topic)
	if err != nil {
		t.Fatal(err)
	}
	mailpit, err := testcontainers.Run(ctx, "axllent/mailpit:v1.31.1",
		testcontainers.WithExposedPorts("1025/tcp", "8025/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/api/v1/info").WithPort("8025/tcp")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(mailpit) })
	host, err := mailpit.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	smtpPort, err := mailpit.MappedPort(ctx, "1025/tcp")
	if err != nil {
		t.Fatal(err)
	}
	httpPort, err := mailpit.MappedPort(ctx, "8025/tcp")
	if err != nil {
		t.Fatal(err)
	}
	sender, err := identityemail.NewMailpitSender(net.JoinHostPort(host, smtpPort.Port()), "no-reply@flowspace.local")
	if err != nil {
		t.Fatal(err)
	}
	deadSender, err := identityemail.NewMailpitSender("127.0.0.1:1", "no-reply@flowspace.local")
	if err != nil {
		t.Fatal(err)
	}
	mailURL := "http://" + net.JoinHostPort(host, httpPort.Port())
	protector, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{3}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	repository := identitypostgres.NewDeliveryRepository(pool)
	newWorker := func(repository outbound.DeliveryRepository, sender outbound.EmailSender) *identityevent.EmailWorker {
		t.Helper()
		worker, err := identityevent.NewEmailWorker(seed, topic, "identity-mail-delivery-group", repository, protector, sender, logger)
		if err != nil {
			t.Fatal(err)
		}
		return worker
	}
	createRequest := func(subject, code, purpose string) (outbound.OutboxEvent, pgtype.UUID) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		address := subject + "@example.com"
		if _, err := tx.Exec(ctx, `INSERT INTO identity_accounts (subject, email_local, email_domain, password_hash) VALUES ($1, $1, 'example.com', '$argon2id$test')`, subject); err != nil {
			t.Fatal(err)
		}
		if purpose == "password-reset" {
			if _, err := tx.Exec(ctx, `UPDATE identity_accounts SET email_verified_at=statement_timestamp() WHERE subject=$1`, subject); err != nil {
				t.Fatal(err)
			}
		}
		var challengeID, eventID pgtype.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO identity_challenges (account_subject, purpose, email_local, email_domain, code_verifier, expires_at)
			VALUES ($1, $2, $1, 'example.com', $3, statement_timestamp() + INTERVAL '10 minutes') RETURNING id`, subject, purpose, bytes.Repeat([]byte{1}, 32)).Scan(&challengeID); err != nil {
			t.Fatal(err)
		}
		material, err := protector.Protect(uuid.UUID(challengeID.Bytes).String(), purpose, subject, address, code)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO identity_challenge_deliveries (challenge_id, key_version, nonce, ciphertext) VALUES ($1, $2, $3, $4)`, challengeID, material.KeyVersion, material.Nonce, material.Ciphertext); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(ctx, `INSERT INTO identity_outbox_events (challenge_id) VALUES ($1) RETURNING id`, challengeID).Scan(&eventID); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return outbound.OutboxEvent{ID: uuid.UUID(eventID.Bytes).String(), ChallengeID: uuid.UUID(challengeID.Bytes).String(), Purpose: purpose}, challengeID
	}
	mailbox := func() struct {
		Total    int `json:"total"`
		Messages []struct {
			ID string `json:"ID"`
		} `json:"messages"`
	} {
		t.Helper()
		var result struct {
			Total    int `json:"total"`
			Messages []struct {
				ID string `json:"ID"`
			} `json:"messages"`
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, mailURL+"/api/v1/messages", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&result) != nil {
			t.Fatalf("Mailpit mailbox status = %d", response.StatusCode)
		}
		return result
	}
	materialCount := func(id pgtype.UUID) int {
		t.Helper()
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenge_deliveries WHERE challenge_id = $1`, id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	outage, outageID := createRequest("mail-outage", "123456", "password-reset")
	if err := publisher.Publish(ctx, outage); err != nil {
		t.Fatal(err)
	}
	mailSender := &switchingSender{target: deadSender}
	down := newWorker(repository, mailSender)
	if worked, err := down.RunOnce(ctx); !worked || !errors.Is(err, identityevent.ErrEmailDelivery) || materialCount(outageID) != 1 || mailbox().Total != 0 {
		t.Fatalf("mail outage: worked=%v error=%v", worked, err)
	}
	mailSender.target = sender
	recovered := down
	if worked, err := recovered.RunOnce(ctx); !worked || err != nil || materialCount(outageID) != 0 || mailbox().Total != 1 {
		t.Fatalf("mail recovery: worked=%v error=%v", worked, err)
	}
	if err := publisher.Publish(ctx, outage); err != nil {
		t.Fatal(err)
	}
	if worked, err := recovered.RunOnce(ctx); !worked || err != nil || mailbox().Total != 1 {
		t.Fatalf("duplicate event: worked=%v error=%v", worked, err)
	}
	recovered.Close()
	crash, crashID := createRequest("worker-crash", "654321", "verify-email")
	if err := publisher.Publish(ctx, crash); err != nil {
		t.Fatal(err)
	}
	crashed := newWorker(crashAfterSendRepository{repository}, sender)
	if worked, err := crashed.RunOnce(ctx); !worked || !errors.Is(err, identityevent.ErrEmailDelivery) || materialCount(crashID) != 1 || mailbox().Total != 2 {
		t.Fatalf("crash after SMTP: worked=%v error=%v", worked, err)
	}
	crashed.Close()
	replayed := newWorker(repository, sender)
	if worked, err := replayed.RunOnce(ctx); !worked || err != nil || materialCount(crashID) != 0 || mailbox().Total != 3 {
		t.Fatalf("crash replay: worked=%v error=%v", worked, err)
	}
	messages := mailbox()
	if len(messages.Messages) != 3 {
		t.Fatalf("Mailpit message count = %d, want 3", len(messages.Messages))
	}
	for _, message := range messages.Messages[:2] {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, mailURL+"/api/v1/message/"+message.ID+"/raw", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		raw, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || !bytes.Contains(raw, []byte("654321")) {
			t.Fatal("crash replay changed the emailed code")
		}
	}
	stale, staleID := createRequest("stale", "111111", "verify-email")
	if err := publisher.Publish(ctx, stale); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET replaced_at = statement_timestamp() WHERE id = $1`, staleID); err != nil {
		t.Fatal(err)
	}
	if worked, err := replayed.RunOnce(ctx); !worked || err != nil || materialCount(staleID) != 0 || mailbox().Total != 3 {
		t.Fatalf("stale event: worked=%v error=%v", worked, err)
	}
	var workerSpans int
	for _, span := range spans.GetSpans() {
		if span.Name == "identity.email_delivery" {
			workerSpans++
		}
	}
	if logs.Len() != 6 || workerSpans != 6 {
		t.Fatalf("delivery telemetry records: logs=%d traces=%d, want 6 each", logs.Len(), workerSpans)
	}
	recovery, recoveryID := createRequest("recovery-current", "222222", "password-reset")
	if err := publisher.Publish(ctx, recovery); err != nil {
		t.Fatal(err)
	}
	if worked, err := replayed.RunOnce(ctx); !worked || err != nil || materialCount(recoveryID) != 0 || mailbox().Total != 4 {
		t.Fatalf("current recovery event: worked=%v error=%v", worked, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, mailURL+"/api/v1/message/latest/raw", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK || !bytes.Contains(raw, []byte("Flowspace password reset code")) ||
		!bytes.Contains(raw, []byte("222222")) || !bytes.Contains(raw, []byte("expires in 10 minutes")) {
		t.Fatalf("recovery Mailpit message: status=%d error=%v", response.StatusCode, readErr)
	}
	staleRecovery, staleRecoveryID := createRequest("recovery-stale", "333333", "password-reset")
	if err := publisher.Publish(ctx, staleRecovery); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET replaced_at=statement_timestamp() WHERE id=$1`, staleRecoveryID); err != nil {
		t.Fatal(err)
	}
	if worked, err := replayed.RunOnce(ctx); !worked || err != nil || materialCount(staleRecoveryID) != 0 || mailbox().Total != 4 {
		t.Fatalf("stale recovery event: worked=%v error=%v", worked, err)
	}
	replayed.Close()
	observer, err := kgo.NewClient(kgo.SeedBrokers(seed), kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	var records []*kgo.Record
	for len(records) < 6 {
		records = append(records, observer.PollFetches(ctx).Records()...)
		if err := ctx.Err(); err != nil {
			t.Fatal(err)
		}
	}
	for _, record := range records {
		for _, secret := range [][]byte{
			[]byte("123456"), []byte("654321"), []byte("111111"), []byte("222222"), []byte("333333"),
			[]byte("mail-outage@example.com"), []byte("worker-crash@example.com"), []byte("stale@example.com"),
			[]byte("recovery-current@example.com"), []byte("recovery-stale@example.com"),
		} {
			if bytes.Contains(record.Value, secret) || bytes.Contains(record.Key, secret) {
				t.Fatal("broker record contains email delivery secrets")
			}
		}
	}
	workerSpans = 0
	for _, span := range spans.GetSpans() {
		if span.Name == "identity.email_delivery" {
			workerSpans++
		}
	}
	if logs.Len() != 8 || workerSpans != 8 {
		t.Fatalf("delivery telemetry records: logs=%d traces=%d, want 8 each", logs.Len(), workerSpans)
	}
	telemetry := telemetryText(logs, spans.GetSpans())
	for _, secret := range []string{
		"123456", "654321", "111111", "222222", "333333", "mail-outage@example.com",
		"worker-crash@example.com", "stale@example.com", "recovery-current@example.com", "recovery-stale@example.com",
	} {
		if strings.Contains(telemetry, secret) {
			t.Fatal("delivery telemetry contains email delivery secrets")
		}
	}
	providerOnly, providerOnlyID := createRequest("provider-only", "444444", "verify-email")
	if _, err := pool.Exec(ctx, `UPDATE identity_accounts SET password_hash = NULL WHERE subject = 'provider-only'`); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Publish(ctx, providerOnly); err != nil {
		t.Fatal(err)
	}
	delivered := mailbox().Total
	mailSender.target = deadSender
	providerWorker := newWorker(repository, mailSender)
	if worked, err := providerWorker.RunOnce(ctx); !worked || !errors.Is(err, identityevent.ErrEmailDelivery) ||
		materialCount(providerOnlyID) != 1 || mailbox().Total != delivered {
		t.Fatalf("provider-only mail outage: worked=%v error=%v", worked, err)
	}
	mailSender.target = sender
	if worked, err := providerWorker.RunOnce(ctx); !worked || err != nil || materialCount(providerOnlyID) != 0 || mailbox().Total != delivered+1 {
		t.Fatalf("provider-only mail recovery: worked=%v error=%v", worked, err)
	}
	providerWorker.Close()
	var passwordless, verified bool
	if err := pool.QueryRow(ctx, `SELECT password_hash IS NULL, email_verified_at IS NOT NULL FROM identity_accounts WHERE subject = 'provider-only'`).
		Scan(&passwordless, &verified); err != nil || !passwordless || verified {
		t.Fatalf("delivery retry changed the provider-only account: passwordless=%t verified=%t error=%v", passwordless, verified, err)
	}
	testTraceFromSignupToDelivery(ctx, t, pool, protector, publisher, newWorker, sender, logger, logs, spans)
	testPasswordChangeNotice(ctx, t, pool, newWorker, mailSender, sender, deadSender, mailURL, mailbox().Total)
}

// spansNamed returns the recorded spans with name, in the order they ended.
func spansNamed(spans *tracetest.InMemoryExporter, name string) []tracetest.SpanStub {
	var named []tracetest.SpanStub
	for _, span := range spans.GetSpans() {
		if span.Name == name {
			named = append(named, span)
		}
	}
	return named
}

// assertDeliveryContinuesRelay makes sure that the last delivery span is a
// consumer child of the last producer span of the relay, and that its
// email_delivery_processed line has its trace ID. It returns the relay span.
func assertDeliveryContinuesRelay(t *testing.T, logs *observer.ObservedLogs, spans *tracetest.InMemoryExporter) tracetest.SpanStub {
	t.Helper()
	published, delivered := spansNamed(spans, "identity.outbox_publish"), spansNamed(spans, "identity.email_delivery")
	relaySpan, deliverySpan := published[len(published)-1], delivered[len(delivered)-1]
	if relaySpan.SpanKind != trace.SpanKindProducer || deliverySpan.SpanKind != trace.SpanKindConsumer ||
		deliverySpan.Parent.SpanID() != relaySpan.SpanContext.SpanID() || deliverySpan.SpanContext.TraceID() != relaySpan.SpanContext.TraceID() {
		t.Fatalf("relay span = %+v, delivery span = %+v", relaySpan, deliverySpan)
	}
	processed := logs.FilterMessage("email_delivery_processed").All()
	fields := processed[len(processed)-1].ContextMap()
	if fields["trace_id"] != deliverySpan.SpanContext.TraceID().String() || fields["operation"] != "identity.email_delivery" {
		t.Fatalf("email_delivery_processed = %v", fields)
	}
	return relaySpan
}

// testTraceFromSignupToDelivery proves that a signup request, the outbox
// publish, and the email delivery share one trace, and that a signup outside
// a request span gets a new root trace in the relay.
func testTraceFromSignupToDelivery(ctx context.Context, t *testing.T, pool *pgxpool.Pool, protector outbound.DeliveryProtector,
	publisher *outboxevent.Publisher, newWorker func(outbound.DeliveryRepository, outbound.EmailSender) *identityevent.EmailWorker,
	sender outbound.EmailSender, logger *zap.Logger, logs *observer.ObservedLogs, spans *tracetest.InMemoryExporter,
) {
	t.Helper()
	// Earlier steps published their events without the relay.
	if _, err := pool.Exec(ctx, `UPDATE identity_outbox_events SET published_at = statement_timestamp() WHERE published_at IS NULL`); err != nil {
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
	relay := outboxevent.NewOutboxRelay(identitypostgres.NewOutboxRepository(pool), publisher.Publish, "trace-relay", logger)
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
		worker := newWorker(identitypostgres.NewDeliveryRepository(pool), sender)
		defer worker.Close()
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
	telemetry := telemetryText(logs, spans.GetSpans())
	for _, secret := range []string{"Traced-Signup@example.com", "Untraced-Signup@example.com", "correct horse battery staple"} {
		if strings.Contains(telemetry, secret) {
			t.Fatal("trace telemetry contains signup secrets")
		}
	}
}

func testPasswordChangeNotice(ctx context.Context, t *testing.T, pool *pgxpool.Pool,
	newWorker func(outbound.DeliveryRepository, outbound.EmailSender) *identityevent.EmailWorker,
	mailSender *switchingSender, sender, deadSender outbound.EmailSender, mailURL string, mailTotal int,
) {
	t.Helper()
	const subject, email, newPassword = "notice-owner", "Notice-Owner@example.com", "notice password 12345"
	compromised := func(context.Context, string) (bool, error) { return false, nil }
	queries := identitysqlc.New(pool)
	if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
		Subject: subject, EmailLocal: "Notice-Owner", EmailDomain: "example.com", PasswordHash: "$argon2id$old",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.MarkEmailVerified(ctx, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: bytes.Repeat([]byte{4}, 32)}); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{5}, 32)
	code, verifier, _, err := domain.NewChallenge(key, subject, email, domain.PurposePasswordReset, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
		AccountSubject: subject, Purpose: string(domain.PurposePasswordReset), EmailLocal: "Notice-Owner",
		EmailDomain: "example.com", CodeVerifier: verifier[:],
	}); err != nil {
		t.Fatal(err)
	}
	allow := func(context.Context, string) error { return nil }
	reset := app.NewPasswordResetService(identitypostgres.NewAccountRepository(pool), allow, allow, allow, compromised, key)
	input := inbound.ResetPasswordInput{Email: email, Code: code, NewPassword: newPassword, Source: "192.0.2.60"}
	wrong := input
	wrong.Code = "000000"
	if code == wrong.Code {
		wrong.Code = "000001"
	}
	mailSender.target = deadSender
	worker := newWorker(identitypostgres.NewDeliveryRepository(pool), mailSender)
	defer worker.Close()
	if err := reset.ResetPassword(ctx, wrong); !errors.Is(err, app.ErrInvalidPasswordResetCode) {
		t.Fatalf("failed reset = %v", err)
	}
	if found, err := worker.DeliverPasswordChangeNotice(ctx); found || err != nil {
		t.Fatalf("failed reset queued a notice: found=%v error=%v", found, err)
	}
	if err := reset.ResetPassword(ctx, input); err != nil {
		t.Fatalf("reset = %v", err)
	}
	var hash string
	committed := func() {
		t.Helper()
		var activeSessions int
		if err := pool.QueryRow(ctx, `SELECT password_hash, (SELECT count(*) FROM identity_sessions WHERE account_subject=$1 AND revoked_at IS NULL)
			FROM identity_accounts WHERE subject=$1`, subject).Scan(&hash, &activeSessions); err != nil || hash == "$argon2id$old" || activeSessions != 0 {
			t.Fatalf("reset state changed: new hash=%v active sessions=%d error=%v", hash != "$argon2id$old", activeSessions, err)
		}
	}
	committed()
	resetHash := hash
	if found, err := worker.DeliverPasswordChangeNotice(ctx); !found || !errors.Is(err, identityevent.ErrEmailDelivery) {
		t.Fatalf("notice outage: found=%v error=%v", found, err)
	}
	var attempts int
	var delivered bool
	if err := pool.QueryRow(ctx, `SELECT attempt_count, delivered_at IS NOT NULL FROM identity_password_change_notices WHERE account_subject=$1`, subject).Scan(&attempts, &delivered); err != nil || attempts != 1 || delivered {
		t.Fatalf("notice after outage: attempts=%d delivered=%v error=%v", attempts, delivered, err)
	}
	committed()
	if found, err := worker.DeliverPasswordChangeNotice(ctx); found || err != nil {
		t.Fatalf("deferred notice retried early: found=%v error=%v", found, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_password_change_notices SET next_attempt_at=statement_timestamp() WHERE account_subject=$1`, subject); err != nil {
		t.Fatal(err)
	}
	mailSender.target = sender
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if found, err := worker.DeliverPasswordChangeNotice(canceled); found || !errors.Is(err, identityevent.ErrEmailDelivery) {
		t.Fatalf("canceled notice delivery: found=%v error=%v", found, err)
	}
	for range 2 {
		if _, err := worker.DeliverPasswordChangeNotice(ctx); err != nil {
			t.Fatalf("notice retry = %v", err)
		}
	}
	committed()
	var retained int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_password_change_notices WHERE account_subject=$1`, subject).Scan(&retained); err != nil || retained != 0 {
		t.Fatalf("delivered notice kept its email address: rows=%d error=%v", retained, err)
	}
	if hash != resetHash {
		t.Fatal("notice delivery changed the password hash")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, mailURL+"/api/v1/message/latest/raw", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK || !bytes.Contains(raw, []byte("To: Notice-Owner@example.com")) ||
		!bytes.Contains(raw, []byte("Flowspace password changed")) {
		t.Fatalf("notice Mailpit message: status=%d error=%v", response.StatusCode, readErr)
	}
	for _, secret := range []string{code, newPassword, resetHash, "argon2"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("password-change notice contains a secret")
		}
	}
	var mailbox struct {
		Total int `json:"total"`
	}
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, mailURL+"/api/v1/messages", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if err := json.NewDecoder(response.Body).Decode(&mailbox); err != nil || mailbox.Total != mailTotal+1 {
		t.Fatalf("notice count = %d, want %d once: error=%v", mailbox.Total, mailTotal+1, err)
	}
}
