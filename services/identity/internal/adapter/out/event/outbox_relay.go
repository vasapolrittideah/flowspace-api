package event

import (
	"context"
	"errors"
	"time"

	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type OutboxRelay struct {
	repository outbound.OutboxRepository
	publish    func(context.Context, outbound.OutboxEvent) error
	owner      string
}

func NewOutboxRelay(repository outbound.OutboxRepository, publish func(context.Context, outbound.OutboxEvent) error, owner string) *OutboxRelay {
	return &OutboxRelay{repository: repository, publish: publish, owner: owner}
}

func (r *OutboxRelay) RunOnce(ctx context.Context) (bool, error) {
	request, ok, err := r.repository.Claim(ctx, r.owner)
	if err != nil || !ok {
		return false, err
	}
	if err := r.publish(ctx, request); err != nil {
		return true, errors.Join(err, r.repository.Release(ctx, request.ID, r.owner, time.Now().Add(time.Second)))
	}
	return true, r.repository.MarkPublished(ctx, request.ID, r.owner)
}
