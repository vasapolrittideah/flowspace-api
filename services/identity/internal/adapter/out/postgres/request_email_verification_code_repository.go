package postgres

import (
	"context"

	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func (r *AccountRepository) WithinVerificationCodeIssueTransaction(ctx context.Context, fn func(outbound.VerificationCodeIssueTransaction) error) error {
	return r.withinTransaction(ctx, func(tx *accountTransaction) error { return fn(tx) })
}

func (t *accountTransaction) ReplaceVerificationChallenge(ctx context.Context, subject string) error {
	return t.replaceChallenge(ctx, subject, "verify-email")
}
