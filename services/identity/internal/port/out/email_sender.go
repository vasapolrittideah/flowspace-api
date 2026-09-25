package outbound

import "context"

type EmailSender interface {
	Send(ctx context.Context, email, code, purpose string) error
}
