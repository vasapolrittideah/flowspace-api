package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type AccountRepository struct{ pool *pgxpool.Pool }

var _ outbound.AccountRepository = (*AccountRepository)(nil)

func NewAccountRepository(pool *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{pool: pool}
}

func (r *AccountRepository) WithinTransaction(ctx context.Context, fn func(outbound.AccountTransaction) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(&accountTransaction{queries: sqlc.New(tx), session: NewSessionRepository(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type accountTransaction struct {
	queries *sqlc.Queries
	session *SessionRepository
}

var _ outbound.AccountTransaction = (*accountTransaction)(nil)

func (t *accountTransaction) CreateAccount(ctx context.Context, subject, email, passwordHash string) error {
	local, domain, ok := strings.Cut(email, "@")
	if !ok {
		return errors.New("invalid normalized email")
	}
	_, err := t.queries.CreateAccount(ctx, sqlc.CreateAccountParams{
		Subject: subject, EmailLocal: local, EmailDomain: domain, PasswordHash: passwordHash,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "identity_accounts_active_email" {
		return outbound.ErrAccountExists
	}
	return err
}

func (t *accountTransaction) Create(ctx context.Context, subject string, hash []byte) (outbound.SessionRecord, error) {
	return t.session.Create(ctx, subject, hash)
}

func (t *accountTransaction) CreateChallenge(ctx context.Context, subject, email string, verifier [32]byte) (string, error) {
	local, domain, ok := strings.Cut(email, "@")
	if !ok {
		return "", errors.New("invalid normalized email")
	}
	challenge, err := t.queries.CreateChallenge(ctx, sqlc.CreateChallengeParams{
		AccountSubject: subject, Purpose: "verify-email", EmailLocal: local,
		EmailDomain: domain, CodeVerifier: verifier[:],
	})
	if err != nil {
		return "", err
	}
	return uuid.UUID(challenge.ID.Bytes).String(), nil
}

func (t *accountTransaction) StoreDelivery(ctx context.Context, challengeID string, material outbound.DeliveryMaterial) error {
	id, err := parseChallengeID(challengeID)
	if err != nil {
		return err
	}
	return t.queries.StoreChallengeDelivery(ctx, sqlc.StoreChallengeDeliveryParams{
		ChallengeID: id, KeyVersion: material.KeyVersion, Nonce: material.Nonce, Ciphertext: material.Ciphertext,
	})
}

func (t *accountTransaction) CreateOutboxEvent(ctx context.Context, challengeID string) error {
	id, err := parseChallengeID(challengeID)
	if err != nil {
		return err
	}
	_, err = t.queries.CreateOutboxEvent(ctx, id)
	return err
}

func parseChallengeID(value string) (pgtype.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}
