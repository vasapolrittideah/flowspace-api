package inbound

import "context"

type RequestClaimCodeInput struct {
	Email  string
	Source string
}

type ClaimCodeService interface {
	RequestUnverifiedAccountClaimCode(ctx context.Context, input RequestClaimCodeInput) error
}
