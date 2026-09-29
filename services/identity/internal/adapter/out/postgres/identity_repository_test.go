//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/vasapolrittideah/flowspace-api/services/identity/db/migrations"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type failingTokenSigner struct{}

func (failingTokenSigner) Sign(outbound.AccessTokenClaims) (string, error) {
	return "", errors.New("signing failed")
}

type failingDeliveryProtector struct{}

func (failingDeliveryProtector) Protect(_, _, _, _, _ string) (outbound.DeliveryMaterial, error) {
	return outbound.DeliveryMaterial{}, errors.New("encryption failed")
}

func TestIdentityRepository(t *testing.T) {
	ctx := context.Background()
	container, err := postgrescontainer.Run(ctx, "postgres:18-alpine",
		postgrescontainer.WithDatabase("identity"),
		postgrescontainer.WithUsername("identity"),
		postgrescontainer.WithPassword("identity"),
		postgrescontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Error(err)
		}
	})
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
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
	t.Run("refresh rotation and replay", func(t *testing.T) { testRefreshSessionRepository(t, pool) })
	t.Run("current session logout", func(t *testing.T) { testCurrentSessionLogout(t, pool) })
	t.Run("all session logout", func(t *testing.T) { testAllSessionLogout(t, pool) })

	testSessionCheckRepository(t, ctx, pool, dsn)
	testActiveEmailUniqueness(t, ctx, pool)
	testConcurrentSignup(t, ctx, pool)
	testSignupRollback(t, ctx, pool)
	testSessionIssuanceRollback(t, ctx, pool)
	testClaimRetirement(t, ctx, pool)
	testSignupChallengeRecords(t, ctx, pool)
	testSignupTransaction(t, ctx, pool)
	testSignupSigningFailure(t, ctx, pool)
	testVerificationCodeResend(t, ctx, pool)
	testVerifyEmail(t, ctx, pool)
	testClaimCode(t, ctx, pool)
	testRequestPasswordResetCode(t, ctx, pool)

	t.Run("account claims replace one unverified identity", func(t *testing.T) {
		testAccountClaimRepository(t, pool)
	})
	t.Run("password login commits one session and hides missing accounts", func(t *testing.T) {
		testPasswordLoginRepository(t, pool)
	})
}
