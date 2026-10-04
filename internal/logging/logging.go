// Package logging creates shared application loggers, records process
// lifecycles, and writes the trace ID of a request.
package logging

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// New creates a production logger with service and environment context.
func New(service, environment string) *zap.Logger {
	return zap.Must(newConfig(service, environment).Build())
}

// Run logs a process lifecycle, flushes the logger, and returns its exit code.
func Run(logger *zap.Logger, process func() error) int {
	logger.Info("process_started")
	if err := process(); err != nil {
		logger.Error("process_failed", zap.Error(err))
		_ = logger.Sync()
		return 1
	}
	logger.Info("process_stopped")
	_ = logger.Sync()
	return 0
}

// TraceID returns the trace_id field of the span in ctx. Without a valid
// span, it returns a field that writes nothing.
func TraceID(ctx context.Context) zap.Field {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return zap.Skip()
	}
	return zap.String("trace_id", spanContext.TraceID().String())
}

func newConfig(service, environment string) zap.Config {
	config := zap.NewProductionConfig()
	config.InitialFields = map[string]any{
		"environment": environment,
		"service":     service,
	}
	return config
}
