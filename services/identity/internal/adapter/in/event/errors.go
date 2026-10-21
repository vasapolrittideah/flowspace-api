package event

import "errors"

var (
	ErrInvalidDeliveryEvent = errors.New("invalid delivery event")
	ErrEmailDelivery        = errors.New("email delivery failed")
	ErrDeliveryBroker       = errors.New("delivery broker failed")
)
