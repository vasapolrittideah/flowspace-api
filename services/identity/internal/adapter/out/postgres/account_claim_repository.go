package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func (r *AccountRepository) FindAccountForClaim(ctx context.Context, email string) (outbound.ClaimAccount, bool, error) {
	local, domain, ok := strings.Cut(email, "@")
	if !ok {
		return outbound.ClaimAccount{}, false, errors.New("invalid normalized email")
	}
	account, err := sqlc.New(r.pool).GetActiveAccountByEmail(ctx, sqlc.GetActiveAccountByEmailParams{
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

func (t *accountTransaction) GetCurrentClaimChallenge(ctx context.Context, subject string) (outbound.ChallengeState, bool, error) {
	return t.getCurrentChallenge(ctx, subject, "claim-account")
}

func (t *accountTransaction) getCurrentChallenge(ctx context.Context, subject, purpose string) (outbound.ChallengeState, bool, error) {
	challenge, err := t.queries.GetCurrentChallengeForUpdate(ctx, sqlc.GetCurrentChallengeForUpdateParams{
		AccountSubject: subject, Purpose: purpose,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return outbound.ChallengeState{}, false, nil
	}
	if err != nil {
		return outbound.ChallengeState{}, false, err
	}
	var verifier [32]byte
	if len(challenge.CodeVerifier) != len(verifier) {
		return outbound.ChallengeState{}, false, errors.New("invalid challenge verifier")
	}
	copy(verifier[:], challenge.CodeVerifier)
	return outbound.ChallengeState{
		ID: uuid.UUID(challenge.ID.Bytes).String(), Email: challenge.EmailLocal + "@" + challenge.EmailDomain,
		Verifier: verifier, WrongGuesses: challenge.WrongGuesses, ExpiresAt: challenge.ExpiresAt.Time,
	}, true, nil
}

func (t *accountTransaction) RetireAndRevokeAccount(ctx context.Context, subject string) (bool, error) {
	if _, err := t.queries.RevokeAccountSessions(ctx, subject); err != nil {
		return false, err
	}
	if _, err := t.queries.RevokeAccountChallenges(ctx, subject); err != nil {
		return false, err
	}
	if _, err := t.queries.DeleteAccountChallengeDeliveries(ctx, subject); err != nil {
		return false, err
	}
	changed, err := t.queries.RetireUnverifiedAccount(ctx, subject)
	return changed == 1, err
}
