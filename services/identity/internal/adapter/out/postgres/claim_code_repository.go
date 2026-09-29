package postgres

import (
	"context"

	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func (r *AccountRepository) WithinClaimCodeTransaction(ctx context.Context, fn func(outbound.ClaimCodeTransaction) error) error {
	return r.withinTransaction(ctx, func(tx *accountTransaction) error { return fn(tx) })
}

func (t *accountTransaction) ReplaceClaimChallenge(ctx context.Context, subject string) error {
	return t.replaceChallenge(ctx, subject, "claim-account")
}

func (t *accountTransaction) CreateClaimChallenge(ctx context.Context, subject, email string, verifier [32]byte) (string, error) {
	return t.createChallenge(ctx, subject, email, "claim-account", verifier)
}
