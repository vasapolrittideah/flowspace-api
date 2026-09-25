//go:build integration

package event_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
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
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
	"google.golang.org/protobuf/proto"

	contractevents "github.com/vasapolrittideah/flowspace-api/contracts/events"
	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/services/identity/db/migrations"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/event"
	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type failMarkRepository struct {
	outbound.OutboxRepository
	marked *bool
}

func (r failMarkRepository) MarkPublished(context.Context, string, string) error {
	*r.marked = true
	return errors.New("simulated crash before mark")
}

func TestOutboxRelayBrokerRecoveryAndReplay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
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

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO identity_accounts (subject, email_local, email_domain, password_hash)
		VALUES ('relay-subject', 'Secret', 'example.com', '$argon2id$test')`); err != nil {
		t.Fatal(err)
	}
	var challengeID, eventID pgtype.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO identity_challenges
		(account_subject, purpose, email_local, email_domain, code_verifier, expires_at)
		VALUES ('relay-subject', 'verify-email', 'Secret', 'example.com', $1, statement_timestamp() + INTERVAL '10 minutes')
		RETURNING id`, bytes.Repeat([]byte{3}, 32)).Scan(&challengeID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO identity_outbox_events (challenge_id) VALUES ($1) RETURNING id`, challengeID).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	repository := identitypostgres.NewOutboxRepository(pool)

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
	client, err := kgo.NewClient(kgo.SeedBrokers(seed), kgo.RecordDeliveryTimeout(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	const topic = "identity-email-delivery-test"
	if _, err := kadm.NewClient(client).CreateTopic(ctx, 1, 1, nil, topic); err != nil {
		t.Fatal(err)
	}
	publisher, err := event.NewPublisher(ctx, client, registry, topic)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := registry.SchemaByVersion(ctx, topic+"-value", -1)
	if err != nil || registered.Type != sr.TypeProtobuf || strings.TrimSpace(registered.Schema.Schema) != strings.TrimSpace(contractevents.EmailDeliveryRequestedSchema) {
		t.Fatalf("registered event schema = %+v, %v", registered, err)
	}
	deadClient, err := kgo.NewClient(kgo.SeedBrokers("127.0.0.1:1"), kgo.RecordDeliveryTimeout(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(deadClient.Close)
	deadPublisher, err := event.NewPublisher(ctx, deadClient, registry, topic)
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := event.NewOutboxRelay(repository, deadPublisher.Publish, "relay-down").RunOnce(ctx); !worked || err == nil {
		t.Fatalf("broker outage: worked=%v err=%v", worked, err)
	}
	var pending int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_outbox_events WHERE id = $1 AND published_at IS NULL`, eventID).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("pending delivery requests = %d, %v", pending, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_outbox_events SET next_attempt_at = statement_timestamp() - INTERVAL '1 second' WHERE id = $1`, eventID); err != nil {
		t.Fatal(err)
	}
	marked := false
	if worked, err := event.NewOutboxRelay(failMarkRepository{repository, &marked}, publisher.Publish, "relay-crashed").RunOnce(ctx); !worked || err == nil || !marked {
		t.Fatalf("lost mark: worked=%v err=%v", worked, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_outbox_events SET claimed_until = statement_timestamp() - INTERVAL '1 second' WHERE id = $1`, eventID); err != nil {
		t.Fatal(err)
	}
	if worked, err := event.NewOutboxRelay(repository, publisher.Publish, "relay-restarted").RunOnce(ctx); !worked || err != nil {
		t.Fatalf("relay restart: worked=%v err=%v", worked, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_outbox_events WHERE id = $1 AND published_at IS NOT NULL`, eventID).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("published delivery requests = %d, %v", pending, err)
	}

	consumer, err := kgo.NewClient(kgo.SeedBrokers(seed), kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(consumer.Close)
	readCtx, stopReading := context.WithTimeout(ctx, 10*time.Second)
	defer stopReading()
	var records []*kgo.Record
	for len(records) < 2 && readCtx.Err() == nil {
		records = append(records, consumer.PollFetches(readCtx).Records()...)
	}
	if len(records) != 2 {
		t.Fatalf("broker records = %d; want 2", len(records))
	}
	for _, record := range records {
		if string(record.Key) != uuid.UUID(eventID.Bytes).String() || bytes.Contains(record.Value, []byte("Secret@example.com")) {
			t.Fatal("broker record contains an unexpected key or email")
		}
		header := &sr.ConfluentHeader{}
		schemaID, encoded, err := header.DecodeID(record.Value)
		if err != nil || schemaID != registered.ID {
			t.Fatalf("record schema ID = %d, %v; want %d", schemaID, err, registered.ID)
		}
		_, encoded, err = header.DecodeIndex(encoded, 1)
		if err != nil {
			t.Fatal(err)
		}
		var message identityv1.EmailDeliveryRequested
		if err := proto.Unmarshal(encoded, &message); err != nil {
			t.Fatal(err)
		}
		if message.EventId != uuid.UUID(eventID.Bytes).String() || message.ChallengeId != uuid.UUID(challengeID.Bytes).String() || message.Purpose != "verify-email" {
			t.Fatalf("broker identifiers = %+v", &message)
		}
	}
}
