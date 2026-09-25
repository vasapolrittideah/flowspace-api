package event_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	identityevent "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/event"
	deliverycrypto "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type deliveryRepository struct {
	delivery outbound.CurrentDelivery
	called   int
	purged   int
	purgeErr error
}

func (r *deliveryRepository) WithCurrentDelivery(ctx context.Context, _, _ string, send func(context.Context, outbound.CurrentDelivery) error) error {
	r.called++
	return send(ctx, r.delivery)
}

func (r *deliveryRepository) PurgeTerminal(context.Context) error {
	r.purged++
	return r.purgeErr
}

type capturingSender struct {
	called int
	code   string
}

func (s *capturingSender) Send(_ context.Context, _, code, _ string) error {
	s.called++
	s.code = code
	return nil
}

func TestEmailWorkerReadsOnlyOpaqueEventAndChecksRecipient(t *testing.T) {
	worker, repository, sender, record := newEmailWorkerFixture(t)
	defer worker.Close()
	if err := worker.HandleRecord(context.Background(), record); err != nil || sender.called != 1 || sender.code != "123456" {
		t.Fatalf("current delivery: sends=%d, error=%v", sender.called, err)
	}
	repository.delivery.Email = "Other@example.com"
	err := worker.HandleRecord(context.Background(), record)
	if err == nil || sender.called != 1 || strings.Contains(err.Error(), "Recipient@example.com") || strings.Contains(err.Error(), "123456") {
		t.Fatalf("changed recipient: sends=%d, error=%v", sender.called, err)
	}
	if err := worker.HandleRecord(context.Background(), &kgo.Record{Key: record.Key, Value: []byte("invalid")}); err == nil || repository.called != 2 {
		t.Fatalf("invalid broker record reached repository: calls=%d, error=%v", repository.called, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := worker.Run(canceled); !errors.Is(err, context.Canceled) || repository.purged != 1 {
		t.Fatalf("startup cleanup = %d, error = %v", repository.purged, err)
	}
}

func TestEmailWorkerRejectsMalformedBrokerRecords(t *testing.T) {
	worker, repository, sender, valid := newEmailWorkerFixture(t)
	defer worker.Close()
	brokenID := &identityv1.EmailDeliveryRequested{
		EventId: "not-a-uuid", ChallengeId: "6f2a1f3e-77e4-40f8-9798-54a4522730ae", Purpose: "verify-email",
	}
	brokenPayload, err := proto.Marshal(brokenID)
	if err != nil {
		t.Fatal(err)
	}
	tests := []*kgo.Record{
		nil,
		{Key: valid.Key, Value: []byte{0, 0, 0, 0, 1, 0x80}},
		{Key: valid.Key, Value: append(append([]byte(nil), valid.Value[:6]...), 0xff)},
		{Key: valid.Key, Value: append(append([]byte(nil), valid.Value[:6]...), brokenPayload...)},
		{Key: []byte("wrong-key"), Value: valid.Value},
	}
	for _, record := range tests {
		if err := worker.HandleRecord(context.Background(), record); !errors.Is(err, identityevent.ErrInvalidDeliveryEvent) {
			t.Fatalf("malformed event error = %v", err)
		}
	}
	if repository.called != 0 || sender.called != 0 {
		t.Fatalf("malformed event reached delivery: repository=%d sender=%d", repository.called, sender.called)
	}
}

func TestEmailWorkerStopsWhenStartupCleanupFails(t *testing.T) {
	worker, repository, _, _ := newEmailWorkerFixture(t)
	defer worker.Close()
	repository.purgeErr = errors.New("database unavailable with private detail")
	if err := worker.Run(context.Background()); !errors.Is(err, identityevent.ErrEmailDelivery) || strings.Contains(err.Error(), "private detail") {
		t.Fatalf("startup cleanup error = %v", err)
	}
}

func newEmailWorkerFixture(t *testing.T) (*identityevent.EmailWorker, *deliveryRepository, *capturingSender, *kgo.Record) {
	t.Helper()
	const eventID = "24cf7dd3-34a9-4f7d-9d51-d2a888b7ed72"
	const challengeID = "6f2a1f3e-77e4-40f8-9798-54a4522730ae"
	protector, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{1}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	material, err := protector.Protect(challengeID, "verify-email", "subject", "Recipient@example.com", "123456")
	if err != nil {
		t.Fatal(err)
	}
	repository := &deliveryRepository{delivery: outbound.CurrentDelivery{
		Subject: "subject", Email: "Recipient@example.com", Material: material,
	}}
	sender := &capturingSender{}
	worker, err := identityevent.NewEmailWorker("127.0.0.1:1", "identity-email", "identity-mail-test", repository, protector, sender, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := proto.Marshal(&identityv1.EmailDeliveryRequested{
		EventId: eventID, ChallengeId: challengeID, Purpose: "verify-email",
	})
	if err != nil {
		t.Fatal(err)
	}
	header, err := (&sr.ConfluentHeader{}).AppendEncode(nil, 1, []int{0})
	if err != nil {
		t.Fatal(err)
	}
	record := &kgo.Record{Key: []byte(eventID), Value: append(header, payload...)}
	return worker, repository, sender, record
}
