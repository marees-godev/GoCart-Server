package outbox

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/metrics"
)

// Publisher is a background worker that polls outbox_events for PENDING events
// and publishes them to the configured Kafka topic.
//
// Concurrency safety is guaranteed by FOR UPDATE SKIP LOCKED: rows claimed by
// one Publisher instance are invisible to all other instances during the same
// poll cycle. If the process crashes mid-cycle, Postgres automatically rolls
// back the open transaction and returns all rows to PENDING.
type Publisher struct {
	pool     *pgxpool.Pool
	producer KafkaPublisher
	store    *Store
	cfg      Config
}

// NewPublisher constructs a Publisher. Use DefaultConfig() if you have no
// service-specific overrides.
func NewPublisher(pool *pgxpool.Pool, producer KafkaPublisher, store *Store, cfg Config) *Publisher {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = DefaultConfig().PollInterval
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultConfig().BatchSize
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = DefaultConfig().MaxRetries
	}
	if cfg.BaseRetryDelay <= 0 {
		cfg.BaseRetryDelay = DefaultConfig().BaseRetryDelay
	}
	if cfg.MaxRetryDelay <= 0 {
		cfg.MaxRetryDelay = DefaultConfig().MaxRetryDelay
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = DefaultConfig().ServiceName
	}
	return &Publisher{pool: pool, producer: producer, store: store, cfg: cfg}
}

// Start runs the publish loop until ctx is cancelled. Call this in a goroutine.
// It is safe to run multiple Publisher instances against the same database.
func (p *Publisher) Start(ctx context.Context) {
	slog.Info("outbox publisher started",
		"service", p.cfg.ServiceName,
		"poll_interval", p.cfg.PollInterval,
		"batch_size", p.cfg.BatchSize,
		"max_retries", p.cfg.MaxRetries,
		"base_retry_delay", p.cfg.BaseRetryDelay,
		"max_retry_delay", p.cfg.MaxRetryDelay,
		"dlq_topic", p.cfg.DLQTopic,
	)

	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("outbox publisher stopped", "service", p.cfg.ServiceName)
			return
		case <-ticker.C:
			if err := p.processOnce(ctx); err != nil {
				if ctx.Err() != nil || errors.Is(err, context.Canceled) {
					slog.Info("outbox publisher stopped", "service", p.cfg.ServiceName)
					return
				}
				slog.Error("outbox publisher cycle error", "service", p.cfg.ServiceName, "error", err)
			}
		}
	}
}

// ProcessOnce executes a single poll-and-publish cycle. Exported for tests and scheduled triggers.
func (p *Publisher) ProcessOnce(ctx context.Context) error {
	return p.processOnce(ctx)
}

// processOnce executes a single poll-and-publish cycle within one database
// transaction. All status mutations for the batch are committed atomically.
func (p *Publisher) processOnce(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	evts, err := p.store.FetchAndLockPending(ctx, tx, p.cfg.BatchSize)
	if err != nil {
		return err
	}
	if len(evts) == 0 {
		return tx.Commit(ctx)
	}

	for _, evt := range evts {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		envelope, err := events.UnmarshalEnvelope(evt.Payload)
		if err != nil {
			slog.Error("outbox: invalid payload, marking failed",
				"event_id", evt.ID,
				"event_type", evt.EventType,
				"error", err,
			)
			metrics.RecordOutboxError(p.cfg.ServiceName, evt.Topic, "invalid_payload")
			if mErr := p.store.MarkFailed(ctx, tx, evt.ID, p.cfg.MaxRetries); mErr != nil {
				slog.Error("outbox: could not mark event failed", "event_id", evt.ID, "error", mErr)
			}
			continue
		}

		// Ensure envelope has AggregateID and EventID
		if envelope.AggregateID == "" {
			envelope.AggregateID = evt.AggregateID
		}
		if envelope.EventID == "" {
			envelope.EventID = evt.ID.String()
		}

		if err := p.producer.Publish(ctx, evt.Topic, evt.AggregateID, envelope); err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return ctx.Err()
			}
			metrics.RecordOutboxError(p.cfg.ServiceName, evt.Topic, "publish_error")

			// Calculate exponential backoff
			delay := p.cfg.BaseRetryDelay
			for i := 0; i < evt.RetryCount; i++ {
				delay *= 2
				if p.cfg.MaxRetryDelay > 0 && delay > p.cfg.MaxRetryDelay {
					delay = p.cfg.MaxRetryDelay
					break
				}
			}

			slog.Error("outbox: publish failed, applying backoff",
				"event_id", evt.ID,
				"event_type", evt.EventType,
				"topic", evt.Topic,
				"retry_count", evt.RetryCount,
				"next_retry_delay", delay,
				"error", err,
			)

			// If max retries exceeded, route to DLQ
			if evt.RetryCount+1 >= p.cfg.MaxRetries {
				dlqTopic := p.cfg.DLQTopic
				if dlqTopic == "" && evt.Topic != "" {
					dlqTopic = evt.Topic + ".dlq"
				}
				if dlqTopic != "" {
					slog.Warn("outbox: max retries reached, forwarding to DLQ",
						"event_id", evt.ID,
						"topic", evt.Topic,
						"dlq_topic", dlqTopic,
					)
					if dlqErr := p.producer.Publish(ctx, dlqTopic, evt.AggregateID, envelope); dlqErr != nil {
						slog.Error("outbox: failed to route to DLQ", "event_id", evt.ID, "dlq_topic", dlqTopic, "error", dlqErr)
					}
				}
			}

			if mErr := p.store.MarkFailedWithBackoff(ctx, tx, evt.ID, p.cfg.MaxRetries, delay); mErr != nil {
				slog.Error("outbox: could not mark event failed with backoff", "event_id", evt.ID, "error", mErr)
			}
			continue
		}

		lag := time.Since(evt.CreatedAt)
		metrics.RecordOutboxLag(p.cfg.ServiceName, lag)
		metrics.RecordOutboxPublished(p.cfg.ServiceName, evt.Topic)

		slog.Info("outbox: event published",
			"event_id", evt.ID,
			"event_type", evt.EventType,
			"topic", evt.Topic,
			"lag_ms", lag.Milliseconds(),
		)
		if mErr := p.store.MarkPublished(ctx, tx, evt.ID); mErr != nil {
			slog.Error("outbox: could not mark event published", "event_id", evt.ID, "error", mErr)
		}
	}

	return tx.Commit(ctx)
}
