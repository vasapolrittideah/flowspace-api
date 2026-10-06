package event_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/protobuf/proto"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	identityevent "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/event"
	deliverycrypto "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type deliveryRepository struct {
	delivery outbound.CurrentDelivery
	called   int
	purged   int
	purgeErr error
	notices  []string
	deferred int
}

func (r *deliveryRepository) WithNextPasswordChangeNotice(ctx context.Context, send func(context.Context, string) error) (bool, error) {
	if len(r.notices) == 0 {
		return false, nil
	}
	if err := send(ctx, r.notices[0]); err != nil {
		r.deferred++
		return true, err
	}
	r.notices = r.notices[1:]
	return true, nil
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
	called    int
	code      string
	purpose   string
	notices   []string
	noticeErr error
}

func (s *capturingSender) SendPasswordChangeNotice(_ context.Context, email string) error {
	if s.noticeErr != nil {
		return s.noticeErr
	}
	s.notices = append(s.notices, email)
	return nil
}

func (s *capturingSender) Send(_ context.Context, _, code, purpose string) error {
	s.called++
	s.code = code
	s.purpose = purpose
	return nil
}

func TestEmailWorkerAcceptsPasswordResetEvent(t *testing.T) {
	worker, repository, sender, record := newEmailWorkerFixtureForPurpose(t, string(domain.PurposePasswordReset))
	defer worker.Close()
	if err := worker.HandleRecord(context.Background(), record); err != nil || repository.called != 1 ||
		sender.called != 1 || sender.code != "123456" || sender.purpose != string(domain.PurposePasswordReset) {
		t.Fatalf("recovery delivery: repository=%d sender=%d purpose=%q error=%v", repository.called, sender.called, sender.purpose, err)
	}
	for _, secret := range [][]byte{[]byte("Recipient@example.com"), []byte("123456")} {
		if bytes.Contains(record.Key, secret) || bytes.Contains(record.Value, secret) {
			t.Fatal("recovery broker record contains delivery secrets")
		}
	}
}

func TestEmailWorkerDeliversPasswordChangeNotice(t *testing.T) {
	worker, repository, sender, _ := newEmailWorkerFixture(t)
	defer worker.Close()
	repository.notices = []string{"Recipient@example.com"}
	sender.noticeErr = errors.New("smtp refused Recipient@example.com")
	found, err := worker.DeliverPasswordChangeNotice(context.Background())
	if !found || !errors.Is(err, identityevent.ErrEmailDelivery) || strings.Contains(err.Error(), "Recipient@example.com") ||
		repository.deferred != 1 || len(repository.notices) != 1 {
		t.Fatalf("failed notice: found=%v deferred=%d error=%v", found, repository.deferred, err)
	}
	sender.noticeErr = nil
	if found, err := worker.DeliverPasswordChangeNotice(context.Background()); !found || err != nil ||
		len(sender.notices) != 1 || sender.notices[0] != "Recipient@example.com" || len(repository.notices) != 0 {
		t.Fatalf("retried notice: found=%v sent=%v error=%v", found, sender.notices, err)
	}
	if found, err := worker.DeliverPasswordChangeNotice(context.Background()); found || err != nil || len(sender.notices) != 1 {
		t.Fatalf("no due notice: found=%v sent=%d error=%v", found, len(sender.notices), err)
	}
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
	return newEmailWorkerFixtureForPurpose(t, string(domain.PurposeVerifyEmail))
}

func TestPasswordChangeNoticeStartsANewConsumerTrace(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	core, logs := observer.New(zap.InfoLevel)
	repository := &deliveryRepository{notices: []string{"Recipient@example.com"}}
	sender := &capturingSender{noticeErr: errors.New("smtp refused Recipient@example.com")}
	worker, err := identityevent.NewEmailWorker("127.0.0.1:1", "identity-email", "identity-mail-test", repository, nil, sender, zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	// The caller span must not become the parent of a notice.
	callerTrace, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	callerSpan, _ := trace.SpanIDFromHex("b7ad6b7169203331")
	caller := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: callerTrace, SpanID: callerSpan, TraceFlags: trace.FlagsSampled,
	}))

	if _, err := worker.DeliverPasswordChangeNotice(caller); !errors.Is(err, identityevent.ErrEmailDelivery) {
		t.Fatalf("failed notice = %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "identity.password_change_notice" || spans[0].SpanKind != trace.SpanKindConsumer ||
		spans[0].Parent.IsValid() || spans[0].Status.Code != otelcodes.Error {
		t.Fatalf("notice spans = %+v", spans)
	}
	lines := logs.FilterMessage("password_change_notice_failed").All()
	if len(lines) != 1 {
		t.Fatalf("notice logs = %v", logs.All())
	}
	fields := lines[0].ContextMap()
	if len(fields) != 2 || fields["trace_id"] != spans[0].SpanContext.TraceID().String() ||
		fields["operation"] != "identity.password_change_notice" {
		t.Fatalf("password_change_notice_failed = %v", fields)
	}
}

func newEmailWorkerFixtureForPurpose(t *testing.T, purpose string) (*identityevent.EmailWorker, *deliveryRepository, *capturingSender, *kgo.Record) {
	t.Helper()
	const eventID = "24cf7dd3-34a9-4f7d-9d51-d2a888b7ed72"
	const challengeID = "6f2a1f3e-77e4-40f8-9798-54a4522730ae"
	protector, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{1}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	material, err := protector.Protect(challengeID, purpose, "subject", "Recipient@example.com", "123456")
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
		EventId: eventID, ChallengeId: challengeID, Purpose: purpose,
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
