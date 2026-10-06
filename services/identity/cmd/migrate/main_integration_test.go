//go:build integration

package main

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/vasapolrittideah/flowspace-api/services/identity/db/migrations"
)

func startIdentityDatabase(ctx context.Context, t *testing.T) string {
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
	return dsn
}

func TestIdentityMigrationAppliesSchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	dsn := startIdentityDatabase(ctx, t)
	if err := run(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	connection, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	var table string
	if err := connection.QueryRowContext(ctx, "SELECT 'identity_accounts'::regclass::text").Scan(&table); err != nil || table != "identity_accounts" {
		t.Fatalf("migrated table = %q, %v", table, err)
	}
}

// claimWithoutTraceContext is the claim query of the relay before migration
// 00009.
const claimWithoutTraceContext = `WITH next_event AS (
    SELECT event.id
    FROM identity_outbox_events AS event
    JOIN identity_challenges AS challenge ON challenge.id = event.challenge_id
    WHERE event.published_at IS NULL
      AND event.next_attempt_at <= statement_timestamp()
      AND (event.claimed_until IS NULL OR event.claimed_until <= statement_timestamp())
    ORDER BY event.next_attempt_at, event.created_at
    LIMIT 1
    FOR UPDATE OF event SKIP LOCKED
)
UPDATE identity_outbox_events AS event
SET claim_owner = $1, claimed_until = statement_timestamp() + INTERVAL '30 seconds'
FROM next_event, identity_challenges AS challenge
WHERE event.id = next_event.id AND challenge.id = event.challenge_id
RETURNING event.id, event.challenge_id, challenge.purpose`

func traceContextColumns(ctx context.Context, t *testing.T, connection *sql.DB) int {
	t.Helper()
	var count int
	if err := connection.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'identity_outbox_events' AND column_name IN ('traceparent', 'tracestate')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestOutboxTraceContextMigrationKeepsExistingEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	connection, err := sql.Open("pgx", startIdentityDatabase(ctx, t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	goose.SetBaseFS(migrations.Files)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, connection, ".", 8); err != nil {
		t.Fatal(err)
	}
	var existing string
	if err := connection.QueryRowContext(ctx, `WITH account AS (
			INSERT INTO identity_accounts (subject, email_local, email_domain, password_hash)
			VALUES ('outbox-subject', 'outbox', 'example.com', '$argon2id$test') RETURNING subject
		), challenge AS (
			INSERT INTO identity_challenges (account_subject, purpose, email_local, email_domain, code_verifier, expires_at)
			SELECT subject, 'verify-email', 'outbox', 'example.com', decode(repeat('01', 32), 'hex'), statement_timestamp() + INTERVAL '10 minutes'
			FROM account RETURNING id
		)
		INSERT INTO identity_outbox_events (challenge_id) SELECT id FROM challenge RETURNING id::text`).Scan(&existing); err != nil {
		t.Fatal(err)
	}

	if err := goose.UpToContext(ctx, connection, ".", 9); err != nil {
		t.Fatal(err)
	}
	var traceparent, tracestate sql.NullString
	if err := connection.QueryRowContext(ctx, `SELECT traceparent, tracestate FROM identity_outbox_events WHERE id = $1`, existing).
		Scan(&traceparent, &tracestate); err != nil || traceparent.Valid || tracestate.Valid {
		t.Fatalf("existing event context = %v, %v, error = %v", traceparent, tracestate, err)
	}
	var claimed, challengeID, purpose string
	if err := connection.QueryRowContext(ctx, claimWithoutTraceContext, "older-relay").Scan(&claimed, &challengeID, &purpose); err != nil ||
		claimed != existing || purpose != "verify-email" {
		t.Fatalf("older relay claim = %q, %q, error = %v", claimed, purpose, err)
	}
	if _, err := connection.ExecContext(ctx, `WITH challenge AS (
			INSERT INTO identity_challenges (account_subject, purpose, email_local, email_domain, code_verifier, expires_at)
			VALUES ('outbox-subject', 'password-reset', 'outbox', 'example.com', decode(repeat('02', 32), 'hex'), statement_timestamp() + INTERVAL '10 minutes')
			RETURNING id
		)
		INSERT INTO identity_outbox_events (challenge_id) SELECT id FROM challenge`); err != nil {
		t.Fatalf("older insert = %v", err)
	}

	if err := goose.DownToContext(ctx, connection, ".", 8); err != nil {
		t.Fatal(err)
	}
	var events int
	if err := connection.QueryRowContext(ctx, `SELECT count(*) FROM identity_outbox_events`).Scan(&events); err != nil || events != 2 {
		t.Fatalf("events after rollback = %d, error = %v", events, err)
	}
	if got := traceContextColumns(ctx, t, connection); got != 0 {
		t.Fatalf("trace context columns after rollback = %d", got)
	}
	if err := goose.UpContext(ctx, connection, "."); err != nil {
		t.Fatal(err)
	}
	if got := traceContextColumns(ctx, t, connection); got != 2 {
		t.Fatalf("trace context columns after the second migration = %d", got)
	}
}
