package bootstrap

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	"github.com/twmb/franz-go/pkg/sr"
	"go.uber.org/zap"

	inboundevent "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/event"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/email"
	outboundevent "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/event"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
)

type Worker struct {
	pool     *pgxpool.Pool
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
	pool, err := pgxpool.New(ctx, string(config.DatabaseURL))
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
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
		postgres.NewDeliveryRepository(pool), opener, sender, logger, brokerOptions[1:]...)
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
		relay: outboundevent.NewOutboxRelay(postgres.NewOutboxRepository(pool), publisher.Publish, rand.Text()),
		group: config.DeliveryGroup, health: newHTTPServer(config.HealthAddress, health), logger: logger,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	defer w.pool.Close()
	defer w.producer.Close()
	defer w.consumer.Close()
	cleanupCtx, cleanupCancel := context.WithTimeout(ctx, requestTimeout)
	err := postgres.NewDeliveryRepository(w.pool).PurgeTerminal(cleanupCtx)
	cleanupCancel()
	if err != nil {
		return errors.New("delivery cleanup failed")
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, 2)
	go func() { results <- w.health.ListenAndServe() }()
	go func() { w.runEmail(runCtx); results <- nil }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	nextAgeLog := time.Now()
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
			_, err := w.relay.RunOnce(stepCtx)
			stop()
			if err != nil {
				w.logger.Warn("outbox_publish_failed")
			}
			if !time.Now().Before(nextAgeLog) {
				w.logOutboxAge(runCtx)
				w.logBrokerLag(runCtx)
				nextAgeLog = time.Now().Add(time.Minute)
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

func (w *Worker) logOutboxAge(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var age float64
	if err := w.pool.QueryRow(checkCtx, `SELECT COALESCE(EXTRACT(EPOCH FROM (statement_timestamp() - min(created_at))), 0)
		FROM identity_outbox_events WHERE published_at IS NULL`).Scan(&age); err == nil {
		w.logger.Info("identity_outbox_age", zap.Float64("oldest_seconds", age))
	} else {
		w.logger.Warn("identity_outbox_age_unavailable")
	}
}

func (w *Worker) logBrokerLag(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	lags, err := kadm.NewClient(w.producer).Lag(checkCtx, w.group)
	lag, found := lags[w.group]
	if err != nil || !found || (&lag).Error() != nil {
		w.logger.Warn("identity_broker_lag_unavailable")
		return
	}
	w.logger.Info("identity_broker_lag", zap.Int64("records", lag.Lag.Total()))
}
