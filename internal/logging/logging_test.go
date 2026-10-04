package logging

import (
	"errors"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestConfigUsesProductionDefaultsAndContext(t *testing.T) {
	config := newConfig("workspace-api", "local")

	if config.Encoding != "json" {
		t.Fatalf("Encoding = %q, want json", config.Encoding)
	}
	if config.Level.Enabled(zap.DebugLevel) || !config.Level.Enabled(zap.InfoLevel) {
		t.Fatalf("Level = %s, want info", config.Level)
	}
	if config.InitialFields["service"] != "workspace-api" || config.InitialFields["environment"] != "local" {
		t.Fatalf("InitialFields = %v", config.InitialFields)
	}
}

func TestNewUsesProductionLevel(t *testing.T) {
	logger := New("workspace-api", "local")
	defer func() { _ = logger.Sync() }()

	if logger.Check(zap.DebugLevel, "debug") != nil || logger.Check(zap.InfoLevel, "info") == nil {
		t.Fatal("New() did not enable info and disable debug")
	}
}

func TestRunLogsProcessLifecycle(t *testing.T) {
	tests := []struct {
		name     string
		process  func() error
		wantCode int
		wantLogs []string
	}{
		{name: "success", process: func() error { return nil }, wantLogs: []string{"process_started", "process_stopped"}},
		{name: "failure", process: func() error { return errors.New("failed") }, wantCode: 1, wantLogs: []string{"process_started", "process_failed"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core, logs := observer.New(zap.InfoLevel)

			if code := Run(zap.New(core), test.process); code != test.wantCode {
				t.Fatalf("Run() = %d, want %d", code, test.wantCode)
			}
			entries := logs.AllUntimed()
			if len(entries) != len(test.wantLogs) {
				t.Fatalf("logs = %v", entries)
			}
			for i, want := range test.wantLogs {
				if entries[i].Message != want {
					t.Fatalf("logs[%d].Message = %q, want %q", i, entries[i].Message, want)
				}
			}
			if test.wantCode == 1 && entries[1].ContextMap()["error"] != "failed" {
				t.Fatalf("failure context = %v", entries[1].ContextMap())
			}
		})
	}
}

func TestTraceIDWritesTheTraceIDOfAValidSpan(t *testing.T) {
	traceID := trace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36}
	for _, flags := range []trace.TraceFlags{trace.FlagsSampled, 0} {
		ctx := trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{
			TraceID: traceID, SpanID: trace.SpanID{0, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7}, TraceFlags: flags,
		}))
		core, logs := observer.New(zap.InfoLevel)
		zap.New(core).Info("request", TraceID(ctx))
		if got := logs.All()[0].ContextMap(); len(got) != 1 || got["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Fatalf("flags %v: fields = %v", flags, got)
		}
	}
}

func TestTraceIDOmitsTheFieldWithoutAValidSpan(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	zap.New(core).Info("request", TraceID(t.Context()))
	if got := logs.All()[0].ContextMap(); len(got) != 0 {
		t.Fatalf("fields = %v, want none", got)
	}
}
