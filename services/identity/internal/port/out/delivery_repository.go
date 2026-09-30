package outbound

import "context"

type CurrentDelivery struct {
	Subject  string
	Email    string
	Material DeliveryMaterial
}

type DeliveryRepository interface {
	WithCurrentDelivery(ctx context.Context, challengeID, purpose string, send func(context.Context, CurrentDelivery) error) error
	// WithNextPasswordChangeNotice reports whether a due notice was found. A failed send defers the notice for a retry.
	WithNextPasswordChangeNotice(ctx context.Context, send func(ctx context.Context, email string) error) (bool, error)
	PurgeTerminal(ctx context.Context) error
}
