package event_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/event"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type relayRepository struct {
	request   outbound.OutboxEvent
	pending   bool
	published bool
	released  bool
}

func (r *relayRepository) Claim(context.Context, string) (outbound.OutboxEvent, bool, error) {
	return r.request, r.pending, nil
}

func (r *relayRepository) MarkPublished(context.Context, string, string) error {
	r.published = true
	r.pending = false
	return nil
}

func (r *relayRepository) Release(_ context.Context, _, _ string, next time.Time) error {
	r.released = !next.IsZero()
	return nil
}

func TestOutboxRelayRetriesTheSameEventAfterPublishFailure(t *testing.T) {
	request := outbound.OutboxEvent{ID: "event-1", ChallengeID: "challenge-1", Purpose: "verify-email"}
	repository := &relayRepository{request: request, pending: true}
	var published []outbound.OutboxEvent
	relay := event.NewOutboxRelay(repository, func(_ context.Context, got outbound.OutboxEvent) error {
		published = append(published, got)
		if len(published) == 1 {
			return errors.New("broker unavailable")
		}
		return nil
	}, "relay-1")

	if worked, err := relay.RunOnce(context.Background()); !worked || err == nil || !repository.released || repository.published {
		t.Fatalf("failed publish: worked=%v err=%v released=%v published=%v", worked, err, repository.released, repository.published)
	}
	if worked, err := relay.RunOnce(context.Background()); !worked || err != nil || !repository.published {
		t.Fatalf("retried publish: worked=%v err=%v published=%v", worked, err, repository.published)
	}
	if len(published) != 2 || published[0] != request || published[1] != request {
		t.Fatalf("published requests = %+v", published)
	}
	if worked, err := relay.RunOnce(context.Background()); worked || err != nil {
		t.Fatalf("empty outbox: worked=%v err=%v", worked, err)
	}
}
