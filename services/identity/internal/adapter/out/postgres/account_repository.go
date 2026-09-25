package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	return r.withinTransaction(ctx, func(tx *accountTransaction) error { return fn(tx) })
}

func (r *AccountRepository) WithinVerificationTransaction(ctx context.Context, fn func(outbound.VerificationTransaction) error) error {
	return r.withinTransaction(ctx, func(tx *accountTransaction) error { return fn(tx) })
}

func (r *AccountRepository) WithinClaimCodeTransaction(ctx context.Context, fn func(outbound.ClaimCodeTransaction) error) error {
	return r.withinTransaction(ctx, func(tx *accountTransaction) error { return fn(tx) })
}

func (r *AccountRepository) WithinAccountClaimTransaction(ctx context.Context, fn func(outbound.AccountClaimTransaction) error) error {
	return r.withinTransaction(ctx, func(tx *accountTransaction) error { return fn(tx) })
}

func (r *AccountRepository) withinTransaction(ctx context.Context, fn func(*accountTransaction) error) error {
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

var (
	_ outbound.AccountTransaction      = (*accountTransaction)(nil)
	_ outbound.VerificationTransaction = (*accountTransaction)(nil)
	_ outbound.ClaimCodeTransaction    = (*accountTransaction)(nil)
	_ outbound.AccountClaimTransaction = (*accountTransaction)(nil)
)

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

func (t *accountTransaction) GetActiveAccountForSession(ctx context.Context, subject, sessionID string) (outbound.AccountState, error) {
	id, err := parseUUID(sessionID)
	if err != nil {
		return outbound.AccountState{}, outbound.ErrUnauthenticated
	}
	account, err := t.queries.GetActiveAccountForSessionForUpdate(ctx, sqlc.GetActiveAccountForSessionForUpdateParams{
		Subject: subject, SessionID: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return outbound.AccountState{}, outbound.ErrUnauthenticated
	}
	if err != nil {
		return outbound.AccountState{}, err
	}
	return outbound.AccountState{Email: account.EmailLocal + "@" + account.EmailDomain, EmailVerified: account.EmailVerifiedAt.Valid}, nil
}

func (t *accountTransaction) CanIssueCode(ctx context.Context, subject string) (bool, error) {
	allowed, err := t.queries.CanIssueCode(ctx, subject)
	if err != nil {
		return false, err
	}
	return allowed.Valid && allowed.Bool, nil
}

func (t *accountTransaction) GetAccountForClaim(ctx context.Context, email string) (outbound.ClaimAccount, bool, error) {
	local, domain, ok := strings.Cut(email, "@")
	if !ok {
		return outbound.ClaimAccount{}, false, errors.New("invalid normalized email")
	}
	account, err := t.queries.GetActiveAccountByEmailForUpdate(ctx, sqlc.GetActiveAccountByEmailForUpdateParams{
		EmailLocal: local, EmailDomain: domain,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return outbound.ClaimAccount{}, false, nil
	}
	if err != nil {
		return outbound.ClaimAccount{}, false, err
	}
	return outbound.ClaimAccount{Subject: account.Subject, EmailVerified: account.EmailVerifiedAt.Valid}, true, nil
}

func (t *accountTransaction) ReplaceVerificationChallenge(ctx context.Context, subject string) error {
	return t.replaceChallenge(ctx, subject, "verify-email")
}

func (t *accountTransaction) ReplaceClaimChallenge(ctx context.Context, subject string) error {
	return t.replaceChallenge(ctx, subject, "claim-account")
}

func (t *accountTransaction) replaceChallenge(ctx context.Context, subject, purpose string) error {
	_, err := t.queries.ReplaceCurrentChallenge(ctx, sqlc.ReplaceCurrentChallengeParams{
		AccountSubject: subject, Purpose: purpose,
	})
	return err
}

func (t *accountTransaction) CreateChallenge(ctx context.Context, subject, email string, verifier [32]byte) (string, error) {
	return t.createChallenge(ctx, subject, email, "verify-email", verifier)
}

func (t *accountTransaction) CreateClaimChallenge(ctx context.Context, subject, email string, verifier [32]byte) (string, error) {
	return t.createChallenge(ctx, subject, email, "claim-account", verifier)
}

func (t *accountTransaction) createChallenge(ctx context.Context, subject, email, purpose string, verifier [32]byte) (string, error) {
	local, domain, ok := strings.Cut(email, "@")
	if !ok {
		return "", errors.New("invalid normalized email")
	}
	challenge, err := t.queries.CreateChallenge(ctx, sqlc.CreateChallengeParams{
		AccountSubject: subject, Purpose: purpose, EmailLocal: local,
		EmailDomain: domain, CodeVerifier: verifier[:],
	})
	if err != nil {
		return "", err
	}
	return uuid.UUID(challenge.ID.Bytes).String(), nil
}

func (t *accountTransaction) StoreDelivery(ctx context.Context, challengeID string, material outbound.DeliveryMaterial) error {
	id, err := parseUUID(challengeID)
	if err != nil {
		return err
	}
	return t.queries.StoreChallengeDelivery(ctx, sqlc.StoreChallengeDeliveryParams{
		ChallengeID: id, KeyVersion: material.KeyVersion, Nonce: material.Nonce, Ciphertext: material.Ciphertext,
	})
}

func (t *accountTransaction) CreateOutboxEvent(ctx context.Context, challengeID string) error {
	id, err := parseUUID(challengeID)
	if err != nil {
		return err
	}
	_, err = t.queries.CreateOutboxEvent(ctx, id)
	return err
}

func (t *accountTransaction) GetCurrentVerificationChallenge(ctx context.Context, subject string) (outbound.ChallengeState, bool, error) {
	return t.getCurrentChallenge(ctx, subject, "verify-email")
}

func (t *accountTransaction) IncrementChallengeWrongGuess(ctx context.Context, challengeID string) error {
	id, err := parseUUID(challengeID)
	if err != nil {
		return err
	}
	_, err = t.queries.IncrementChallengeWrongGuess(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func (t *accountTransaction) ConsumeChallenge(ctx context.Context, challengeID string) (bool, error) {
	id, err := parseUUID(challengeID)
	if err != nil {
		return false, err
	}
	changed, err := t.queries.ConsumeCurrentChallenge(ctx, id)
	return changed == 1, err
}

func (t *accountTransaction) MarkEmailVerified(ctx context.Context, subject string) (bool, error) {
	changed, err := t.queries.MarkEmailVerified(ctx, subject)
	return changed == 1, err
}

func parseUUID(value string) (pgtype.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}
