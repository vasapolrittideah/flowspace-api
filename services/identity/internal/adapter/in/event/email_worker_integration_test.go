//go:build integration

package event_test

import (
	"bytes"
	"context"
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
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/vasapolrittideah/flowspace-api/services/identity/db/migrations"
	identityevent "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/event"
	deliverycrypto "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	identityemail "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/email"
	outboxevent "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/event"
	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type crashAfterSendRepository struct{ outbound.DeliveryRepository }

type switchingSender struct{ target outbound.EmailSender }

func (s *switchingSender) Send(ctx context.Context, email, code, purpose string) error {
	return s.target.Send(ctx, email, code, purpose)
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
	createRequest := func(subject, code string) (outbound.OutboxEvent, pgtype.UUID) {
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
		var challengeID, eventID pgtype.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO identity_challenges (account_subject, purpose, email_local, email_domain, code_verifier, expires_at)
			VALUES ($1, 'verify-email', $1, 'example.com', $2, statement_timestamp() + INTERVAL '10 minutes') RETURNING id`, subject, bytes.Repeat([]byte{1}, 32)).Scan(&challengeID); err != nil {
			t.Fatal(err)
		}
		material, err := protector.Protect(uuid.UUID(challengeID.Bytes).String(), "verify-email", subject, address, code)
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
		return outbound.OutboxEvent{ID: uuid.UUID(eventID.Bytes).String(), ChallengeID: uuid.UUID(challengeID.Bytes).String(), Purpose: "verify-email"}, challengeID
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
	outage, outageID := createRequest("mail-outage", "123456")
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
	crash, crashID := createRequest("worker-crash", "654321")
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
	stale, staleID := createRequest("stale", "111111")
	if err := publisher.Publish(ctx, stale); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET replaced_at = statement_timestamp() WHERE id = $1`, staleID); err != nil {
		t.Fatal(err)
	}
	if worked, err := replayed.RunOnce(ctx); !worked || err != nil || materialCount(staleID) != 0 || mailbox().Total != 3 {
		t.Fatalf("stale event: worked=%v error=%v", worked, err)
	}
	replayed.Close()
	observer, err := kgo.NewClient(kgo.SeedBrokers(seed), kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	var records []*kgo.Record
	for len(records) < 4 {
		records = append(records, observer.PollFetches(ctx).Records()...)
		if err := ctx.Err(); err != nil {
			t.Fatal(err)
		}
	}
	for _, record := range records {
		for _, secret := range [][]byte{[]byte("123456"), []byte("654321"), []byte("111111"), []byte("mail-outage@example.com"), []byte("worker-crash@example.com"), []byte("stale@example.com")} {
			if bytes.Contains(record.Value, secret) || bytes.Contains(record.Key, secret) {
				t.Fatal("broker record contains email delivery secrets")
			}
		}
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
	telemetry := fmt.Sprintf("%+v %+v", logs.All(), spans.GetSpans())
	for _, secret := range []string{"123456", "654321", "111111", "mail-outage@example.com", "worker-crash@example.com", "stale@example.com"} {
		if strings.Contains(telemetry, secret) {
			t.Fatal("delivery telemetry contains email delivery secrets")
		}
	}
}
