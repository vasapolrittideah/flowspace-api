package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var ErrDeliveryMaterialGone = errors.New("delivery material unavailable")

type DeliveryRepository struct{ pool *pgxpool.Pool }

var _ outbound.DeliveryRepository = (*DeliveryRepository)(nil)

func NewDeliveryRepository(pool *pgxpool.Pool) *DeliveryRepository {
	return &DeliveryRepository{pool: pool}
}

func (r *DeliveryRepository) WithCurrentDelivery(ctx context.Context, challengeID, purpose string, send func(context.Context, outbound.CurrentDelivery) error) error {
	id, err := parseUUID(challengeID)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := sqlc.New(tx)
	account, err := queries.GetDeliveryAccountForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	challenge, err := queries.GetDeliveryChallengeForUpdate(ctx, id)
	if err != nil {
		return err
	}
	if challenge.Purpose != purpose {
		return tx.Commit(ctx)
	}
	if !isCurrentDelivery(account, challenge) {
		if _, err := queries.DeleteChallengeDelivery(ctx, id); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	material, err := queries.GetChallengeDelivery(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	if err := send(ctx, outbound.CurrentDelivery{
		Subject: account.Subject, Email: account.EmailLocal + "@" + account.EmailDomain,
		Material: outbound.DeliveryMaterial{KeyVersion: material.KeyVersion, Nonce: material.Nonce, Ciphertext: material.Ciphertext},
	}); err != nil {
		return err
	}
	changed, err := queries.DeleteChallengeDelivery(ctx, id)
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrDeliveryMaterialGone
	}
	return tx.Commit(ctx)
}

func isCurrentDelivery(account sqlc.GetDeliveryAccountForUpdateRow, challenge sqlc.GetDeliveryChallengeForUpdateRow) bool {
	emailEligible := !account.EmailVerifiedAt.Valid
	if challenge.Purpose == "password-reset" {
		emailEligible = account.EmailVerifiedAt.Valid
	}
	return !account.RetiredAt.Valid && emailEligible && !challenge.ReplacedAt.Valid &&
		!challenge.ConsumedAt.Valid && challenge.WrongGuesses < 5 && !challenge.Expired &&
		challenge.EmailLocal == account.EmailLocal && challenge.EmailDomain == account.EmailDomain
}

func (r *DeliveryRepository) WithNextPasswordChangeNotice(ctx context.Context, send func(context.Context, string) error) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := sqlc.New(tx)
	notice, err := queries.LockNextPasswordChangeNotice(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if sendErr := send(ctx, notice.EmailLocal+"@"+notice.EmailDomain); sendErr != nil {
		if _, err := queries.DeferPasswordChangeNotice(ctx, notice.ID); err != nil {
			return true, errors.Join(sendErr, err)
		}
		return true, errors.Join(sendErr, tx.Commit(ctx))
	}
	// Deleting the sent notice removes its email address. A crash before this commit resends the notice,
	// which changes no account or session state.
	if _, err := queries.DeletePasswordChangeNotice(ctx, notice.ID); err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}

func (r *DeliveryRepository) PurgeTerminal(ctx context.Context) error {
	_, err := sqlc.New(r.pool).PurgeTerminalDeliveries(ctx)
	return err
}
