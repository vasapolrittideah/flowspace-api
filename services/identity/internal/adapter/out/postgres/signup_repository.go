package postgres

import (
	"context"

	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func (r *AccountRepository) WithinTransaction(ctx context.Context, fn func(outbound.AccountTransaction) error) error {
	return r.withinTransaction(ctx, func(tx *accountTransaction) error { return fn(tx) })
}
