package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func (r *AccountRepository) WithinPasswordSessionTransaction(ctx context.Context, fn func(outbound.PasswordSessionTransaction) error) error {
	return r.withinTransaction(ctx, func(tx *accountTransaction) error { return fn(tx) })
}

func (r *AccountRepository) FindPasswordAccount(ctx context.Context, email string) (outbound.PasswordAccount, bool, error) {
	local, domain, ok := strings.Cut(email, "@")
	if !ok {
		return outbound.PasswordAccount{}, false, errors.New("invalid normalized email")
	}
	account, err := sqlc.New(r.pool).GetPasswordAccountByEmail(ctx, sqlc.GetPasswordAccountByEmailParams{EmailLocal: local, EmailDomain: domain})
	if errors.Is(err, pgx.ErrNoRows) {
		return outbound.PasswordAccount{}, false, nil
	}
	if err != nil {
		return outbound.PasswordAccount{}, false, err
	}
	return outbound.PasswordAccount{Subject: account.Subject, PasswordHash: account.PasswordHash, EmailVerified: account.EmailVerifiedAt.Valid}, true, nil
}

func (t *accountTransaction) LockPasswordAccount(ctx context.Context, subject string) (outbound.PasswordAccount, error) {
	account, err := t.queries.GetPasswordAccountForUpdate(ctx, subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return outbound.PasswordAccount{}, outbound.ErrUnauthenticated
	}
	if err != nil {
		return outbound.PasswordAccount{}, err
	}
	return outbound.PasswordAccount{Subject: account.Subject, PasswordHash: account.PasswordHash, EmailVerified: account.EmailVerifiedAt.Valid}, nil
}
