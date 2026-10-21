package postgres

import "errors"

var (
	ErrDeliveryMaterialGone = errors.New("delivery material unavailable")
	ErrOutboxClaimLost      = errors.New("outbox claim expired")
)
