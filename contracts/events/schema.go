package events

import _ "embed"

//go:embed flowspace/identity/v1/email_delivery_requested.proto
var EmailDeliveryRequestedSchema string
