package event

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/protobuf/proto"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
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
	stale    bool
	err      error
}

func (r *deliveryRepository) WithNextPasswordChangeNotice(ctx context.Context, send func(context.Context, string) error) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
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
	if r.err != nil {
		return r.err
	}
	if r.stale {
		return nil
	}
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
	sendErr   error
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
	return s.sendErr
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
	if !found || !errors.Is(err, ErrEmailDelivery) || strings.Contains(err.Error(), "Recipient@example.com") ||
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
		if err := worker.HandleRecord(context.Background(), record); !errors.Is(err, ErrInvalidDeliveryEvent) {
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
	if err := worker.Run(context.Background()); !errors.Is(err, ErrEmailDelivery) || strings.Contains(err.Error(), "private detail") {
		t.Fatalf("startup cleanup error = %v", err)
	}
}

func newEmailWorkerFixture(t *testing.T) (*EmailWorker, *deliveryRepository, *capturingSender, *kgo.Record) {
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
	worker, err := NewEmailWorker("127.0.0.1:1", "identity-email", "identity-mail-test", repository, nil, sender, zap.New(core))
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

	if _, err := worker.DeliverPasswordChangeNotice(caller); !errors.Is(err, ErrEmailDelivery) {
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

func newEmailWorkerFixtureForPurpose(t *testing.T, purpose string) (*EmailWorker, *deliveryRepository, *capturingSender, *kgo.Record) {
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
	worker, err := NewEmailWorker("127.0.0.1:1", "identity-email", "identity-mail-test", repository, protector, sender, zap.NewNop())
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

func TestEmailWorkerCountsRecordDeliveriesByKindAndOutcome(t *testing.T) {
	reader := useDeliveryMeter(t)
	committed := func(context.Context, ...*kgo.Record) error { return nil }
	tests := []struct {
		name    string
		purpose domain.CodePurpose
		commit  func(context.Context, ...*kgo.Record) error
		change  func(*deliveryRepository, *capturingSender, *kgo.Record)
		wantErr error
		count   string
	}{
		{name: "verify email", purpose: domain.PurposeVerifyEmail, count: "verify-email delivered"},
		{name: "claim account", purpose: domain.PurposeClaimAccount, count: "claim-account delivered"},
		{name: "password reset", purpose: domain.PurposePasswordReset, count: "password-reset delivered"},
		{
			name: "failed send", purpose: domain.PurposeVerifyEmail, wantErr: ErrEmailDelivery, count: "verify-email failed",
			change: func(_ *deliveryRepository, sender *capturingSender, _ *kgo.Record) {
				sender.sendErr = errors.New("smtp refused Recipient@example.com")
			},
		},
		{
			name: "failed commit", purpose: domain.PurposeVerifyEmail, wantErr: ErrDeliveryBroker, count: "verify-email commit_failed",
			commit: func(context.Context, ...*kgo.Record) error {
				return errors.New("broker refused 24cf7dd3-34a9-4f7d-9d51-d2a888b7ed72")
			},
		},
		// These records send no email, so they are not delivery attempts.
		{
			name: "stale delivery", purpose: domain.PurposeVerifyEmail,
			change: func(repository *deliveryRepository, _ *capturingSender, _ *kgo.Record) { repository.stale = true },
		},
		{
			name: "repository failure", purpose: domain.PurposeVerifyEmail, wantErr: ErrEmailDelivery,
			change: func(repository *deliveryRepository, _ *capturingSender, _ *kgo.Record) {
				repository.err = errors.New("database unavailable for Recipient@example.com")
			},
		},
		{
			name: "malformed record", purpose: domain.PurposeVerifyEmail, wantErr: ErrInvalidDeliveryEvent,
			change: func(_ *deliveryRepository, _ *capturingSender, record *kgo.Record) { record.Value = []byte("invalid") },
		},
	}
	want := map[string]int64{}
	for _, test := range tests {
		worker, repository, sender, record := newEmailWorkerFixtureForPurpose(t, string(test.purpose))
		if test.change != nil {
			test.change(repository, sender, record)
		}
		commit := test.commit
		if commit == nil {
			commit = committed
		}
		err := worker.processRecord(context.Background(), record, commit)
		worker.Close()
		if !errors.Is(err, test.wantErr) || (test.count == "" && sender.called != 0) {
			t.Fatalf("%s: sends=%d error=%v", test.name, sender.called, err)
		}
		if test.count != "" {
			want[test.count]++
		}
		assertDeliveryCounts(t, reader, want)
	}
}

func TestEmailWorkerCountsPasswordChangeNoticeDeliveries(t *testing.T) {
	reader := useDeliveryMeter(t)
	worker, repository, sender, _ := newEmailWorkerFixture(t)
	defer worker.Close()
	repository.notices = []string{"Recipient@example.com"}
	tests := []struct {
		name    string
		change  func()
		wantErr error
		count   string
	}{
		// A repository failure before the send is not a delivery attempt.
		{
			name: "repository failure", wantErr: ErrEmailDelivery,
			change: func() { repository.err = errors.New("database unavailable for Recipient@example.com") },
		},
		{
			name: "failed send", wantErr: ErrEmailDelivery, count: "password-change-notice failed",
			change: func() {
				repository.err = nil
				sender.noticeErr = errors.New("smtp refused Recipient@example.com")
			},
		},
		{name: "delivered", count: "password-change-notice delivered", change: func() { sender.noticeErr = nil }},
		// No due notice is not a delivery attempt.
		{name: "no due notice", change: func() {}},
	}
	want := map[string]int64{}
	for _, test := range tests {
		test.change()
		if _, err := worker.DeliverPasswordChangeNotice(context.Background()); !errors.Is(err, test.wantErr) {
			t.Fatalf("%s: error=%v", test.name, err)
		}
		if test.count != "" {
			want[test.count]++
		}
		assertDeliveryCounts(t, reader, want)
	}
	if len(sender.notices) != 1 {
		t.Fatalf("sent notices = %d", len(sender.notices))
	}
}

func useDeliveryMeter(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	return reader
}

// assertDeliveryCounts requires exactly the kind and outcome attributes, so no
// address, code, event ID, or error text can reach a delivery count.
func assertDeliveryCounts(t *testing.T, reader *sdkmetric.ManualReader, want map[string]int64) {
	t.Helper()
	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int64{}
	for _, scope := range collected.ScopeMetrics {
		for _, measured := range scope.Metrics {
			if measured.Name != "identity.email.deliveries" {
				continue
			}
			sum, ok := measured.Data.(metricdata.Sum[int64])
			if !ok || !sum.IsMonotonic || measured.Unit != "{delivery}" {
				t.Fatalf("identity.email.deliveries = %+v", measured)
			}
			for _, point := range sum.DataPoints {
				kind, _ := point.Attributes.Value("identity.email.kind")
				outcome, _ := point.Attributes.Value("outcome")
				if point.Attributes.Len() != 2 || kind.Type() != attribute.STRING || outcome.Type() != attribute.STRING {
					t.Fatalf("delivery attributes = %v", point.Attributes.ToSlice())
				}
				counts[kind.AsString()+" "+outcome.AsString()] += point.Value
			}
		}
	}
	if len(counts) != len(want) {
		t.Fatalf("delivery counts = %v, want %v", counts, want)
	}
	for key, value := range want {
		if counts[key] != value {
			t.Fatalf("delivery counts = %v, want %v", counts, want)
		}
	}
}
