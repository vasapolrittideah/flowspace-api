package outbound

import "context"

type CurrentDelivery struct {
	Subject  string
	Email    string
	Material DeliveryMaterial
}

type DeliveryRepository interface {
	WithCurrentDelivery(ctx context.Context, challengeID, purpose string, send func(context.Context, CurrentDelivery) error) error
	PurgeTerminal(ctx context.Context) error
}
