package inbound

import (
	"context"
	"time"
)

type (
	RequestReauthenticationCodeInput struct{ Subject, SessionID, Source string }
	ReauthenticateWithCodeInput      struct{ Subject, SessionID, Source, Code string }
	ReauthenticateWithCodeResult     struct{ ExpiresAt time.Time }
	CodeReauthenticationService      interface {
		RequestReauthenticationCode(ctx context.Context, input RequestReauthenticationCodeInput) error
		ReauthenticateWithCode(ctx context.Context, input ReauthenticateWithCodeInput) (ReauthenticateWithCodeResult, error)
	}
)
