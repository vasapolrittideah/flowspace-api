package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func (r *AccountRepository) WithinPasswordRecoveryRequestTransaction(ctx context.Context, fn func(outbound.PasswordRecoveryRequestTransaction) error) error {
	return r.withinTransaction(ctx, func(tx *accountTransaction) error { return fn(tx) })
}

func (t *accountTransaction) GetAccountForPasswordRecovery(ctx context.Context, email string) (outbound.PasswordRecoveryAccount, bool, error) {
	local, domain, ok := strings.Cut(email, "@")
	if !ok {
		return outbound.PasswordRecoveryAccount{}, false, errors.New("invalid normalized email")
	}
	account, err := t.queries.GetRecoveryAccountForUpdate(ctx, sqlc.GetRecoveryAccountForUpdateParams{EmailLocal: local, EmailDomain: domain})
	if errors.Is(err, pgx.ErrNoRows) {
		return outbound.PasswordRecoveryAccount{}, false, nil
	}
	if err != nil {
		return outbound.PasswordRecoveryAccount{}, false, err
	}
	return outbound.PasswordRecoveryAccount{Subject: account.Subject, EmailVerified: account.EmailVerifiedAt.Valid, HasPassword: account.PasswordHash != ""}, true, nil
}

func (t *accountTransaction) ReplacePasswordResetChallenge(ctx context.Context, subject string) error {
	if err := t.replaceChallenge(ctx, subject, "password-reset"); err != nil {
		return err
	}
	_, err := t.queries.DeleteReplacedChallengeDeliveries(ctx, sqlc.DeleteReplacedChallengeDeliveriesParams{
		AccountSubject: subject, Purpose: "password-reset",
	})
	return err
}

func (t *accountTransaction) CreatePasswordResetChallenge(ctx context.Context, subject, email string, verifier [32]byte) (string, error) {
	return t.createChallenge(ctx, subject, email, "password-reset", verifier)
}
