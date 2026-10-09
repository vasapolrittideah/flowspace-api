//go:build integration

package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redpanda"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/vasapolrittideah/flowspace-api/internal/postgrespool"
)

func TestWorkerReportsTheOutboxAgeAndTheConsumerLag(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
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
	if err := applyIdentityMigration(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	pool, err := postgrespool.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	broker, err := redpanda.Run(ctx, "docker.redpanda.com/redpandadata/redpanda:v25.2.4")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(broker) })
	seed, err := broker.KafkaSeedBroker(ctx)
	if err != nil {
		t.Fatal(err)
	}
	producer, err := kgo.NewClient(kgo.SeedBrokers(seed))
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	worker := &Worker{pool: pool, producer: producer, group: "lag-test"}

	if age, err := worker.outboxAge(ctx); err != nil || age != 0 {
		t.Fatalf("outbox age without events = %v, %v, want 0", age, err)
	}
	insertOutboxEvent(ctx, t, pool, "verify-email", 10*time.Minute, true)
	if age, err := worker.outboxAge(ctx); err != nil || age != 0 {
		t.Fatalf("outbox age with only a published event = %v, %v, want 0", age, err)
	}
	insertOutboxEvent(ctx, t, pool, "claim-account", 90*time.Second, false)
	insertOutboxEvent(ctx, t, pool, "password-reset", 10*time.Second, false)
	if age, err := worker.outboxAge(ctx); err != nil || age < 90 || age > 100 {
		t.Fatalf("outbox age with unpublished events = %v, %v, want the 90 second event", age, err)
	}

	const topic = "identity-lag-test"
	admin := kadm.NewClient(producer)
	if _, err := admin.CreateTopic(ctx, 1, 1, nil, topic); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if err := producer.ProduceSync(ctx, &kgo.Record{Topic: topic, Value: []byte("event")}).FirstErr(); err != nil {
			t.Fatal(err)
		}
	}
	offsets := kadm.Offsets{}
	offsets.Add(kadm.Offset{Topic: topic, Partition: 0, At: 2, LeaderEpoch: -1})
	if err := admin.CommitAllOffsets(ctx, worker.group, offsets); err != nil {
		t.Fatal(err)
	}
	if lag, err := worker.consumerLag(ctx); err != nil || lag != 3 {
		t.Fatalf("consumer lag = %d, %v, want 3", lag, err)
	}

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	core, logs := observer.New(zap.InfoLevel)
	if _, err := registerWorkerMetrics(provider.Meter("test"), zap.New(core), requestTimeout,
		worker.outboxAge, worker.consumerLag); err != nil {
		t.Fatal(err)
	}
	metrics := collectMetrics(t, reader)
	age, ok := metrics["identity.outbox.oldest_age"].Data.(metricdata.Gauge[float64])
	if !ok || len(age.DataPoints) != 1 || age.DataPoints[0].Value < 90 {
		t.Fatalf("outbox age metric = %+v, want the 90 second event", metrics["identity.outbox.oldest_age"])
	}
	lag, ok := metrics["identity.broker.consumer_lag"].Data.(metricdata.Gauge[int64])
	if !ok || len(lag.DataPoints) != 1 || lag.DataPoints[0].Value != 3 {
		t.Fatalf("consumer lag metric = %+v, want 3", metrics["identity.broker.consumer_lag"])
	}

	pool.Close()
	producer.Close()
	metrics = collectMetrics(t, reader)
	assertNoPoints(t, metrics, "identity.outbox.oldest_age")
	assertNoPoints(t, metrics, "identity.broker.consumer_lag")
	if logs.FilterMessage("identity_outbox_age_unavailable").Len() != 1 ||
		logs.FilterMessage("identity_broker_lag_unavailable").Len() != 1 {
		t.Fatalf("logs = %v, want 1 unavailable line for each measurement", logs.All())
	}
}

// insertOutboxEvent adds an outbox event for a new challenge with purpose,
// created age ago, and published when published is true.
func insertOutboxEvent(ctx context.Context, t *testing.T, pool *postgrespool.Pool, purpose string, age time.Duration, published bool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `WITH account AS (
			INSERT INTO identity_accounts (subject, email_local, email_domain, password_hash)
			VALUES ($1, $1, 'example.com', '$argon2id$test') RETURNING subject
		), challenge AS (
			INSERT INTO identity_challenges (account_subject, purpose, email_local, email_domain, code_verifier, expires_at)
			SELECT subject, $1, $1, 'example.com', decode(repeat('01', 32), 'hex'), statement_timestamp() + INTERVAL '10 minutes'
			FROM account RETURNING id
		)
		INSERT INTO identity_outbox_events (challenge_id, created_at, published_at)
		SELECT id, statement_timestamp() - $2::interval, CASE WHEN $3 THEN statement_timestamp() END FROM challenge`,
		purpose, age.String(), published); err != nil {
		t.Fatal(err)
	}
}
