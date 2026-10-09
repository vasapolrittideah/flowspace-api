package bootstrap

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	"github.com/twmb/franz-go/pkg/sr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"

	"github.com/vasapolrittideah/flowspace-api/internal/postgrespool"
	inboundevent "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/event"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/email"
	outboundevent "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/event"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
)

type Worker struct {
	pool     *postgrespool.Pool
	producer *kgo.Client
	consumer *inboundevent.EmailWorker
	relay    *outboundevent.OutboxRelay
	group    string
	health   *http.Server
	logger   *zap.Logger
}

func NewWorker(ctx context.Context, config WorkerConfig, logger *zap.Logger) (*Worker, error) {
	deliveryKey, err := readKey(config.DeliveryKeyFile)
	if err != nil {
		return nil, err
	}
	opener, err := crypto.NewDeliveryProtector(deliveryKey, 1)
	if err != nil {
		return nil, err
	}
	sender, err := email.NewMailpitSender(config.MailAddress, config.MailFrom)
	if err != nil {
		return nil, errors.New("invalid mail configuration")
	}
	pool, err := postgrespool.Open(ctx, string(config.DatabaseURL))
	if errors.Is(err, postgrespool.ErrInvalidConfiguration) {
		return nil, errors.New("invalid database configuration")
	}
	if err != nil {
		return nil, errors.New("identity database unavailable")
	}
	brokerOptions := []kgo.Opt{kgo.SeedBrokers(config.BrokerAddress)}
	registryOptions := []sr.ClientOpt{sr.URLs(config.SchemaRegistryURL)}
	if config.BrokerUsername != "" {
		auth := scram.Auth{User: config.BrokerUsername, Pass: string(config.BrokerPassword)}
		brokerOptions = append(brokerOptions, kgo.SASL(auth.AsSha256Mechanism()))
		registryOptions = append(registryOptions, sr.BasicAuth(config.BrokerUsername, string(config.BrokerPassword)))
	}
	producer, err := kgo.NewClient(brokerOptions...)
	if err != nil {
		pool.Close()
		return nil, errors.New("invalid broker configuration")
	}
	registry, err := sr.NewClient(registryOptions...)
	if err != nil {
		producer.Close()
		pool.Close()
		return nil, errors.New("invalid schema registry configuration")
	}
	publisher, err := outboundevent.NewPublisher(ctx, producer, registry, config.DeliveryTopic)
	if err != nil {
		producer.Close()
		pool.Close()
		return nil, errors.New("schema registry unavailable")
	}
	consumer, err := inboundevent.NewEmailWorker(config.BrokerAddress, config.DeliveryTopic, config.DeliveryGroup,
		postgres.NewDeliveryRepository(pool.Pool), opener, sender, logger, brokerOptions[1:]...)
	if err != nil {
		producer.Close()
		pool.Close()
		return nil, errors.New("invalid consumer configuration")
	}
	health := http.NewServeMux()
	health.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	health.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		checkCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if pool.Ping(checkCtx) != nil || producer.Ping(checkCtx) != nil {
			http.Error(w, "worker unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return &Worker{
		pool: pool, producer: producer, consumer: consumer,
		relay: outboundevent.NewOutboxRelay(postgres.NewOutboxRepository(pool.Pool), publisher.Publish, rand.Text(), logger),
		group: config.DeliveryGroup, health: newHTTPServer(config.HealthAddress, health), logger: logger,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	defer w.pool.Close()
	defer w.producer.Close()
	defer w.consumer.Close()
	cleanupCtx, cleanupCancel := context.WithTimeout(ctx, requestTimeout)
	err := postgres.NewDeliveryRepository(w.pool.Pool).PurgeTerminal(cleanupCtx)
	cleanupCancel()
	if err != nil {
		return errors.New("delivery cleanup failed")
	}
	metrics, err := registerWorkerMetrics(otel.Meter("flowspace/identity/worker"), w.logger, requestTimeout,
		w.outboxAge, w.consumerLag)
	if err != nil {
		return errors.New("worker metrics unavailable")
	}
	defer func() { _ = metrics.Unregister() }()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, 2)
	go func() { results <- w.health.ListenAndServe() }()
	go func() { w.runEmail(runCtx); results <- nil }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	nextPurge := time.Now()
	completed := 0
	var result error
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case err := <-results:
			completed++
			if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.Canceled) {
				result = err
			} else {
				result = errors.New("worker stopped unexpectedly")
			}
			break loop
		case <-ticker.C:
			stepCtx, stop := context.WithTimeout(runCtx, requestTimeout)
			// The relay logs a publish failure. The event stays queued for its next attempt.
			_, _ = w.relay.RunOnce(stepCtx)
			stop()
			// The email worker logs a notice failure; the notice stays queued for its next attempt.
			stepCtx, stop = context.WithTimeout(runCtx, requestTimeout)
			_, _ = w.consumer.DeliverPasswordChangeNotice(stepCtx)
			stop()
			if !time.Now().Before(nextPurge) {
				w.purgeProviderAttempts(runCtx)
				nextPurge = time.Now().Add(time.Minute)
			}
		}
	}
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
	defer shutdownCancel()
	_ = w.health.Shutdown(shutdownCtx)
	for completed < 2 {
		<-results
		completed++
	}
	return result
}

func (w *Worker) runEmail(ctx context.Context) {
	for ctx.Err() == nil {
		err := w.consumer.Run(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			w.logger.Warn("email_worker_retry")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// purgeProviderAttempts removes expired provider login attempts each minute,
// so a provider email stays at most about eleven minutes.
func (w *Worker) purgeProviderAttempts(ctx context.Context) {
	purgeCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	if err := postgres.NewProviderAttemptRepository(w.pool.Pool).PurgeExpired(purgeCtx); err != nil {
		w.logger.Warn("provider_attempt_purge_failed")
	}
}

// outboxAge returns the age in seconds of the oldest unpublished outbox
// event, or 0 when no such event exists.
func (w *Worker) outboxAge(ctx context.Context) (float64, error) {
	var age float64
	err := w.pool.QueryRow(ctx, `SELECT COALESCE(EXTRACT(EPOCH FROM (statement_timestamp() - min(created_at))), 0)
		FROM identity_outbox_events WHERE published_at IS NULL`).Scan(&age)
	return age, err
}

// consumerLag returns the records that the consumer group of the email worker
// still needs to consume.
func (w *Worker) consumerLag(ctx context.Context) (int64, error) {
	lags, err := kadm.NewClient(w.producer).Lag(ctx, w.group)
	if err != nil {
		return 0, err
	}
	lag, found := lags[w.group]
	if !found {
		return 0, errors.New("consumer group lag missing")
	}
	if err := lag.Error(); err != nil {
		return 0, err
	}
	return lag.Lag.Total(), nil
}

// registerWorkerMetrics reports the outbox age and the consumer lag at each
// export. The measurements run at the same time, each within timeout. A
// measurement that fails omits its value for the interval and writes its
// unavailable line.
func registerWorkerMetrics(meter metric.Meter, logger *zap.Logger, timeout time.Duration,
	outboxAge func(context.Context) (float64, error), consumerLag func(context.Context) (int64, error),
) (metric.Registration, error) {
	ageGauge, err := meter.Float64ObservableGauge("identity.outbox.oldest_age", metric.WithUnit("s"),
		metric.WithDescription("Age of the oldest unpublished outbox event"))
	if err != nil {
		return nil, err
	}
	lagGauge, err := meter.Int64ObservableGauge("identity.broker.consumer_lag", metric.WithUnit("{record}"),
		metric.WithDescription("Records that the email worker consumer group has not consumed"))
	if err != nil {
		return nil, err
	}
	return meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		lagDone := make(chan struct{})
		var lag int64
		var lagErr error
		go func() {
			defer close(lagDone)
			lag, lagErr = consumerLag(ctx)
		}()
		if age, err := outboxAge(ctx); err == nil {
			observer.ObserveFloat64(ageGauge, age)
		} else {
			logger.Warn("identity_outbox_age_unavailable")
		}
		<-lagDone
		if lagErr == nil {
			observer.ObserveInt64(lagGauge, lag)
		} else {
			logger.Warn("identity_broker_lag_unavailable")
		}
		return nil
	}, ageGauge, lagGauge)
}
