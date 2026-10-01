package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func (r *ProviderAttemptRepository) ConsumeProviderState(ctx context.Context, provider string, stateVerifier [32]byte) (outbound.ProviderAttemptProof, bool, error) {
	row, err := sqlc.New(r.pool).ConsumeProviderLoginState(ctx, sqlc.ConsumeProviderLoginStateParams{Provider: provider, StateVerifier: stateVerifier[:]})
	if errors.Is(err, pgx.ErrNoRows) {
		return outbound.ProviderAttemptProof{}, false, nil
	}
	if err != nil {
		return outbound.ProviderAttemptProof{}, false, err
	}
	return outbound.ProviderAttemptProof{
		ID: uuid.UUID(row.ID.Bytes).String(), CodeVerifier: row.CodeVerifier, Nonce: row.Nonce.String, CallbackURL: row.CallbackUrl,
	}, true, nil
}

func (r *ProviderAttemptRepository) RecordProviderResult(ctx context.Context, attemptID string, identity outbound.ProviderIdentity, handoffVerifier [32]byte) (bool, error) {
	id, err := parseUUID(attemptID)
	if err != nil {
		return false, err
	}
	changed, err := sqlc.New(r.pool).RecordProviderLoginResult(ctx, sqlc.RecordProviderLoginResultParams{
		ID:                    id,
		ProviderSubject:       pgtype.Text{String: identity.Subject, Valid: true},
		ProviderEmail:         pgtype.Text{String: identity.Email, Valid: identity.Email != ""},
		ProviderEmailVerified: pgtype.Bool{Bool: identity.EmailVerified, Valid: true},
		ProviderHostedDomain:  pgtype.Text{String: identity.HostedDomain, Valid: identity.HostedDomain != ""},
		HandoffCodeVerifier:   handoffVerifier[:],
	})
	return changed == 1, err
}

func (r *ProviderAttemptRepository) FailProviderAttempt(ctx context.Context, attemptID string) error {
	id, err := parseUUID(attemptID)
	if err != nil {
		return err
	}
	return sqlc.New(r.pool).FailProviderLoginAttempt(ctx, id)
}

func (r *ProviderAttemptRepository) RecordFailedProviderHandoff(ctx context.Context, attemptVerifier [32]byte) error {
	return sqlc.New(r.pool).RecordProviderHandoffFailure(ctx, attemptVerifier[:])
}
