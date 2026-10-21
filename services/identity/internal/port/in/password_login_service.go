package inbound

import "context"

type CreatePasswordSessionInput struct {
	Email    string
	Password string
	Source   string
}

type PasswordLoginService interface {
	CreatePasswordSession(ctx context.Context, input CreatePasswordSessionInput) (CreatePasswordSessionResult, error)
}
