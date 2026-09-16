package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/jackc/pgx/v5/pgxpool"
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
	return &Publisher{pool: pool, producer: producer, store: store, cfg: cfg}
}

// Start runs the publish loop until ctx is cancelled. Call this in a goroutine.
// It is safe to run multiple Publisher instances against the same database.
func (p *Publisher) Start(ctx context.Context) {
	slog.Info("outbox publisher started",
		"poll_interval", p.cfg.PollInterval,
		"batch_size", p.cfg.BatchSize,
		"max_retries", p.cfg.MaxRetries,
	)

	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("outbox publisher stopped")
			return
		case <-ticker.C:
			if err := p.processOnce(ctx); err != nil {
				slog.Error("outbox publisher cycle error", "error", err)
			}
		}
	}
}

// processOnce executes a single poll-and-publish cycle within one database
// transaction. All status mutations for the batch are committed atomically.
func (p *Publisher) processOnce(ctx context.Context) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	evts, err := p.store.FetchAndLockPending(ctx, tx, p.cfg.BatchSize)
	if err != nil {
		return err
	}
	if len(evts) == 0 {
		return tx.Commit(ctx)
	}

	for _, evt := range evts {
		envelope, err := events.UnmarshalEnvelope(evt.Payload)
		if err != nil {
			slog.Error("outbox: invalid payload, marking failed",
				"event_id", evt.ID,
				"event_type", evt.EventType,
				"error", err,
			)
			if mErr := p.store.MarkFailed(ctx, tx, evt.ID, p.cfg.MaxRetries); mErr != nil {
				slog.Error("outbox: could not mark event failed", "event_id", evt.ID, "error", mErr)
			}
			continue
		}

		if err := p.producer.Publish(ctx, evt.Topic, evt.AggregateID, envelope); err != nil {
			slog.Warn("outbox: publish failed",
				"event_id", evt.ID,
				"event_type", evt.EventType,
				"topic", evt.Topic,
				"retry_count", evt.RetryCount,
				"error", err,
			)
			if mErr := p.store.MarkFailed(ctx, tx, evt.ID, p.cfg.MaxRetries); mErr != nil {
				slog.Error("outbox: could not mark event failed", "event_id", evt.ID, "error", mErr)
			}
			continue
		}

		slog.Info("outbox: event published",
			"event_id", evt.ID,
			"event_type", evt.EventType,
			"topic", evt.Topic,
		)
		if mErr := p.store.MarkPublished(ctx, tx, evt.ID); mErr != nil {
			slog.Error("outbox: could not mark event published", "event_id", evt.ID, "error", mErr)
		}
	}

	return tx.Commit(ctx)
}
