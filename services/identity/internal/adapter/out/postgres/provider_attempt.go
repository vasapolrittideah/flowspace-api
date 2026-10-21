package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

// providerAttemptReauthentication is the purpose of a provider attempt that
// re-authenticates a session.
const providerAttemptReauthentication = "reauthenticate"

// createProviderAttempt stores an attempt with the re-authentication purpose
// when it binds a session, and with the login purpose otherwise.
func createProviderAttempt(ctx context.Context, queries *sqlc.Queries, attempt outbound.ProviderAttempt) (time.Time, error) {
	params := sqlc.CreateProviderLoginAttemptParams{
		Provider:             attempt.Provider,
		AttemptTokenVerifier: attempt.AttemptTokenVerifier[:],
		StateVerifier:        attempt.StateVerifier[:],
		CodeVerifier:         attempt.CodeVerifier,
		Nonce:                pgtype.Text{String: attempt.Nonce, Valid: attempt.Nonce != ""},
		CallbackUrl:          attempt.CallbackURL,
		Purpose:              "login",
	}
	if attempt.SessionID != "" {
		session, err := parseUUID(attempt.SessionID)
		if err != nil || attempt.Subject == "" {
			return time.Time{}, outbound.ErrUnauthenticated
		}
		params.Purpose, params.AccountSubject, params.SessionID = providerAttemptReauthentication, pgtype.Text{String: attempt.Subject, Valid: true}, session
	}
	expiresAt, err := queries.CreateProviderLoginAttempt(ctx, params)
	return expiresAt.Time, err
}
