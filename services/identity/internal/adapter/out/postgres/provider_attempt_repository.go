package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type ProviderAttemptRepository struct{ pool *pgxpool.Pool }

var _ outbound.ProviderAttemptRepository = (*ProviderAttemptRepository)(nil)

func NewProviderAttemptRepository(pool *pgxpool.Pool) *ProviderAttemptRepository {
	return &ProviderAttemptRepository{pool: pool}
}

func (r *ProviderAttemptRepository) CreateProviderAttempt(ctx context.Context, attempt outbound.ProviderAttempt) (time.Time, error) {
	expiresAt, err := sqlc.New(r.pool).CreateProviderLoginAttempt(ctx, sqlc.CreateProviderLoginAttemptParams{
		Provider:             attempt.Provider,
		AttemptTokenVerifier: attempt.AttemptTokenVerifier[:],
		StateVerifier:        attempt.StateVerifier[:],
		CodeVerifier:         attempt.CodeVerifier,
		Nonce:                pgtype.Text{String: attempt.Nonce, Valid: attempt.Nonce != ""},
		CallbackUrl:          attempt.CallbackURL,
	})
	return expiresAt.Time, err
}
