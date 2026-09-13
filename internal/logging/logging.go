// Package logging creates shared application loggers and records process lifecycles.
package logging

import "go.uber.org/zap"

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

func newConfig(service, environment string) zap.Config {
	config := zap.NewProductionConfig()
	config.InitialFields = map[string]any{
		"environment": environment,
		"service":     service,
	}
	return config
}
