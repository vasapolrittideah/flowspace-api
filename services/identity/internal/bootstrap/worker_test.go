package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestWorkerMetricsReportTheOutboxAgeAndTheConsumerLag(t *testing.T) {
	reader, logs := registeredWorkerMetrics(t, time.Second,
		func(context.Context) (float64, error) { return 12.5, nil },
		func(context.Context) (int64, error) { return 7, nil })

	metrics := collectMetrics(t, reader)

	age, ok := metrics["identity.outbox.oldest_age"].Data.(metricdata.Gauge[float64])
	if !ok || metrics["identity.outbox.oldest_age"].Unit != "s" || len(age.DataPoints) != 1 ||
		age.DataPoints[0].Value != 12.5 || age.DataPoints[0].Attributes.Len() != 0 {
		t.Fatalf("outbox age = %+v, want 12.5 s", metrics["identity.outbox.oldest_age"])
	}
	lag, ok := metrics["identity.broker.consumer_lag"].Data.(metricdata.Gauge[int64])
	if !ok || metrics["identity.broker.consumer_lag"].Unit != "{record}" || len(lag.DataPoints) != 1 ||
		lag.DataPoints[0].Value != 7 || lag.DataPoints[0].Attributes.Len() != 0 {
		t.Fatalf("consumer lag = %+v, want 7 {record}", metrics["identity.broker.consumer_lag"])
	}
	if logs.Len() != 0 {
		t.Fatalf("logs = %v, want none", logs.All())
	}
}

func TestWorkerMetricsOmitAFailedOutboxAge(t *testing.T) {
	reader, logs := registeredWorkerMetrics(t, time.Second,
		func(context.Context) (float64, error) { return 0, errors.New("database failed") },
		func(context.Context) (int64, error) { return 3, nil })

	metrics := collectMetrics(t, reader)

	assertNoPoints(t, metrics, "identity.outbox.oldest_age")
	assertLag(t, metrics, 3)
	assertOnlyLog(t, logs, "identity_outbox_age_unavailable")
}

func TestWorkerMetricsOmitAFailedConsumerLag(t *testing.T) {
	reader, logs := registeredWorkerMetrics(t, time.Second,
		func(context.Context) (float64, error) { return 0, nil },
		func(context.Context) (int64, error) { return 0, errors.New("broker failed") })

	metrics := collectMetrics(t, reader)

	assertNoPoints(t, metrics, "identity.broker.consumer_lag")
	assertAge(t, metrics, 0)
	assertOnlyLog(t, logs, "identity_broker_lag_unavailable")
}

func TestWorkerMetricsReadCurrentValuesAtEachExport(t *testing.T) {
	age, lag := 1.0, int64(1)
	var ageErr, lagErr error
	reader, logs := registeredWorkerMetrics(t, time.Second,
		func(context.Context) (float64, error) { return age, ageErr },
		func(context.Context) (int64, error) { return lag, lagErr })

	assertAge(t, collectMetrics(t, reader), 1)
	age, lag = 2, 5
	metrics := collectMetrics(t, reader)
	assertAge(t, metrics, 2)
	assertLag(t, metrics, 5)
	ageErr, lagErr = errors.New("database failed"), errors.New("broker failed")
	metrics = collectMetrics(t, reader)
	assertNoPoints(t, metrics, "identity.outbox.oldest_age")
	assertNoPoints(t, metrics, "identity.broker.consumer_lag")
	age, lag, ageErr, lagErr = 3, 6, nil, nil
	metrics = collectMetrics(t, reader)

	assertAge(t, metrics, 3)
	assertLag(t, metrics, 6)
	if logs.Len() != 2 {
		t.Fatalf("logs = %v, want 1 unavailable line for each failed measurement", logs.All())
	}
}

func TestWorkerMetricsMeasureAtTheSameTimeWithinTheTimeout(t *testing.T) {
	const timeout = 2 * time.Second
	ageStarted, lagStarted := make(chan struct{}), make(chan struct{})
	var ageBudget, lagBudget time.Duration
	// Each measurement waits until the other one starts, so a measurement
	// fails at its timeout when both run one after the other.
	reader, logs := registeredWorkerMetrics(t, timeout,
		func(ctx context.Context) (float64, error) {
			ageBudget = budget(ctx)
			close(ageStarted)
			select {
			case <-lagStarted:
				return 4, nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		},
		func(ctx context.Context) (int64, error) {
			lagBudget = budget(ctx)
			close(lagStarted)
			select {
			case <-ageStarted:
				return 9, nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		})

	metrics := collectMetrics(t, reader)

	assertAge(t, metrics, 4)
	assertLag(t, metrics, 9)
	if logs.Len() != 0 {
		t.Fatalf("logs = %v, want none", logs.All())
	}
	for name, value := range map[string]time.Duration{"outbox age": ageBudget, "consumer lag": lagBudget} {
		if value <= 0 || value > timeout {
			t.Errorf("%s deadline = %s after its start, want at most %s", name, value, timeout)
		}
	}
}

func TestWorkerMetricsStopBlockedMeasurementsAtTheTimeout(t *testing.T) {
	const timeout = 300 * time.Millisecond
	var ageBudget, lagBudget time.Duration
	var ageErr, lagErr error
	reader, logs := registeredWorkerMetrics(t, timeout,
		func(ctx context.Context) (float64, error) {
			ageBudget = budget(ctx)
			<-ctx.Done()
			ageErr = ctx.Err()
			return 1, ageErr
		},
		func(ctx context.Context) (int64, error) {
			lagBudget = budget(ctx)
			<-ctx.Done()
			lagErr = ctx.Err()
			return 1, lagErr
		})

	var data metricdata.ResourceMetrics
	collected := make(chan error, 1)
	go func() { collected <- reader.Collect(context.Background(), &data) }()
	// The 1 second allowance covers scheduling and stays far below the
	// 5 second limit of an export.
	select {
	case err := <-collected:
		if err != nil {
			t.Fatalf("collection error = %v, want none", err)
		}
	case <-time.After(timeout + time.Second):
		t.Fatalf("collection did not end within %s after the measurements timed out", time.Second)
	}
	metrics := metricsByName(data)

	for name, value := range map[string]time.Duration{"outbox age": ageBudget, "consumer lag": lagBudget} {
		if value <= 0 || value > timeout {
			t.Errorf("%s deadline = %s after its start, want at most %s", name, value, timeout)
		}
	}
	if !errors.Is(ageErr, context.DeadlineExceeded) || !errors.Is(lagErr, context.DeadlineExceeded) {
		t.Errorf("measurement errors = %v and %v, want the deadline", ageErr, lagErr)
	}
	assertNoPoints(t, metrics, "identity.outbox.oldest_age")
	assertNoPoints(t, metrics, "identity.broker.consumer_lag")
	if logs.FilterMessage("identity_outbox_age_unavailable").Len() != 1 ||
		logs.FilterMessage("identity_broker_lag_unavailable").Len() != 1 || logs.Len() != 2 {
		t.Fatalf("logs = %v, want 1 unavailable line for each measurement", logs.All())
	}
}

func TestWorkerMetricsKeepTheOtherValueWhenOneMeasurementBlocks(t *testing.T) {
	const timeout = 100 * time.Millisecond
	blockAge := func(ctx context.Context) (float64, error) { <-ctx.Done(); return 1, ctx.Err() }
	blockLag := func(ctx context.Context) (int64, error) { <-ctx.Done(); return 1, ctx.Err() }

	t.Run("outbox age", func(t *testing.T) {
		reader, logs := registeredWorkerMetrics(t, timeout, blockAge,
			func(context.Context) (int64, error) { return 8, nil })

		metrics := collectMetrics(t, reader)

		assertNoPoints(t, metrics, "identity.outbox.oldest_age")
		assertLag(t, metrics, 8)
		assertOnlyLog(t, logs, "identity_outbox_age_unavailable")
	})
	t.Run("consumer lag", func(t *testing.T) {
		reader, logs := registeredWorkerMetrics(t, timeout,
			func(context.Context) (float64, error) { return 6.5, nil }, blockLag)

		metrics := collectMetrics(t, reader)

		assertNoPoints(t, metrics, "identity.broker.consumer_lag")
		assertAge(t, metrics, 6.5)
		assertOnlyLog(t, logs, "identity_broker_lag_unavailable")
	})
}

// budget returns the time from now until the deadline of ctx, or 0 when ctx
// has no deadline.
func budget(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0
	}
	return time.Until(deadline)
}

func registeredWorkerMetrics(t *testing.T, timeout time.Duration, outboxAge func(context.Context) (float64, error),
	consumerLag func(context.Context) (int64, error),
) (*sdkmetric.ManualReader, *observer.ObservedLogs) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	core, logs := observer.New(zap.InfoLevel)
	if _, err := registerWorkerMetrics(provider.Meter("test"), zap.New(core), timeout, outboxAge, consumerLag); err != nil {
		t.Fatal(err)
	}
	return reader, logs
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	return metricsByName(data)
}

func metricsByName(data metricdata.ResourceMetrics) map[string]metricdata.Metrics {
	metrics := map[string]metricdata.Metrics{}
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			metrics[metric.Name] = metric
		}
	}
	return metrics
}

func assertNoPoints(t *testing.T, metrics map[string]metricdata.Metrics, name string) {
	t.Helper()
	switch data := metrics[name].Data.(type) {
	case nil:
	case metricdata.Gauge[float64]:
		if len(data.DataPoints) != 0 {
			t.Fatalf("%s = %+v, want no value", name, data)
		}
	case metricdata.Gauge[int64]:
		if len(data.DataPoints) != 0 {
			t.Fatalf("%s = %+v, want no value", name, data)
		}
	default:
		t.Fatalf("%s = %T", name, data)
	}
}

func assertOnlyLog(t *testing.T, logs *observer.ObservedLogs, message string) {
	t.Helper()
	if logs.Len() != 1 || logs.All()[0].Message != message || logs.All()[0].Level != zap.WarnLevel {
		t.Fatalf("logs = %v, want only the %s warning", logs.All(), message)
	}
}

func assertAge(t *testing.T, metrics map[string]metricdata.Metrics, want float64) {
	t.Helper()
	age, ok := metrics["identity.outbox.oldest_age"].Data.(metricdata.Gauge[float64])
	if !ok || len(age.DataPoints) != 1 || age.DataPoints[0].Value != want {
		t.Fatalf("outbox age = %+v, want %v", metrics["identity.outbox.oldest_age"], want)
	}
}

func assertLag(t *testing.T, metrics map[string]metricdata.Metrics, want int64) {
	t.Helper()
	lag, ok := metrics["identity.broker.consumer_lag"].Data.(metricdata.Gauge[int64])
	if !ok || len(lag.DataPoints) != 1 || lag.DataPoints[0].Value != want {
		t.Fatalf("consumer lag = %+v, want %d", metrics["identity.broker.consumer_lag"], want)
	}
}
