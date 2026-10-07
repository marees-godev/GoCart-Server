package outbox

import (
	"context"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store handles all SQL operations against the outbox_events table.
// It is stateless; callers supply the transaction or pool for each call.
type Store struct{}

// NewStore returns a new Store instance.
func NewStore() *Store { return &Store{} }

// Insert writes a new PENDING outbox event inside an existing business
// transaction. The event is guaranteed to be persisted only when the caller
// commits the transaction that contains the associated business state change.
func (s *Store) Insert(ctx context.Context, tx pgx.Tx, evt *Event) error {
	id := evt.ID
	if id == uuid.Nil {
		id = uuid.Must(uuid.NewV7())
	}

	headers := evt.Headers
	if len(headers) == 0 {
		headers = []byte("{}")
	}

	// Try inserting with headers and next_retry_at using a savepoint
	sp, spErr := tx.Begin(ctx)
	if spErr != nil {
		return spErr
	}

	_, err := sp.Exec(ctx, `
		INSERT INTO outbox_events
			(id, aggregate_type, aggregate_id, event_type, payload, headers, topic, status, retry_count, next_retry_at, created_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, 'PENDING', 0, NOW(), NOW(), NOW())`,
		id, evt.AggregateType, evt.AggregateID, evt.EventType, evt.Payload, headers, evt.Topic,
	)
	if err == nil {
		return sp.Commit(ctx)
	}

	_ = sp.Rollback(ctx)

	// Fallback for tables without headers or next_retry_at columns
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events
			(id, aggregate_type, aggregate_id, event_type, payload, topic, status, retry_count, created_at)
		VALUES
			($1, $2, $3, $4, $5, $6, 'PENDING', 0, NOW())`,
		id, evt.AggregateType, evt.AggregateID, evt.EventType, evt.Payload, evt.Topic,
	)
	if err != nil {
		return fmt.Errorf("outbox: insert: %w", err)
	}
	return nil
}

// FetchAndLockPending selects up to limit PENDING events and locks them with
// FOR UPDATE SKIP LOCKED inside the provided transaction. Rows locked by one
// publisher instance are invisible to concurrent publisher instances, preventing
// duplicate delivery without any external coordination.
func (s *Store) FetchAndLockPending(ctx context.Context, tx pgx.Tx, limit int) ([]*Event, error) {
	// Try query with next_retry_at backoff condition first using a savepoint
	sp, spErr := tx.Begin(ctx)
	if spErr != nil {
		return nil, spErr
	}

	rows, err := sp.Query(ctx, `
		SELECT id, aggregate_type, aggregate_id, event_type, payload, topic, retry_count, created_at
		FROM outbox_events
		WHERE status = 'PENDING' AND (next_retry_at IS NULL OR next_retry_at <= NOW())
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`,
		limit,
	)
	if err == nil {
		evts, scanErr := scanEvents(rows)
		rows.Close()
		if scanErr != nil {
			_ = sp.Rollback(ctx)
			return nil, scanErr
		}
		if cErr := sp.Commit(ctx); cErr != nil {
			return nil, cErr
		}
		return evts, nil
	}

	_ = sp.Rollback(ctx)

	// Fallback for tables without next_retry_at column
	rows, err = tx.Query(ctx, `
		SELECT id, aggregate_type, aggregate_id, event_type, payload, topic, retry_count, created_at
		FROM outbox_events
		WHERE status = 'PENDING'
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("outbox: fetch pending: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

func scanEvents(rows pgx.Rows) ([]*Event, error) {
	var evts []*Event
	for rows.Next() {
		e := &Event{}
		if err := rows.Scan(
			&e.ID, &e.AggregateType, &e.AggregateID,
			&e.EventType, &e.Payload, &e.Topic,
			&e.RetryCount, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("outbox: scan: %w", err)
		}
		evts = append(evts, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox: rows: %w", err)
	}
	return evts, nil
}

// MarkPublished transitions an event to PUBLISHED and records the wall-clock
// time of successful broker acknowledgement.
func (s *Store) MarkPublished(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	sp, spErr := tx.Begin(ctx)
	if spErr != nil {
		return spErr
	}

	_, err := sp.Exec(ctx, `
		UPDATE outbox_events
		SET status = 'PUBLISHED', published_at = NOW(), updated_at = NOW()
		WHERE id = $1`,
		id,
	)
	if err == nil {
		return sp.Commit(ctx)
	}

	_ = sp.Rollback(ctx)

	// Fallback for tables without updated_at
	_, err = tx.Exec(ctx, `
		UPDATE outbox_events
		SET status = 'PUBLISHED', published_at = NOW()
		WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("outbox: mark published %s: %w", id, err)
	}
	return nil
}

// MarkFailed increments retry_count for an event and sets status based on maxRetries.
func (s *Store) MarkFailed(ctx context.Context, tx pgx.Tx, id uuid.UUID, maxRetries int) error {
	return s.MarkFailedWithBackoff(ctx, tx, id, maxRetries, 0)
}

// MarkFailedWithBackoff increments retry_count, sets next_retry_at with exponential backoff delay,
// and sets status to FAILED if maxRetries is exceeded.
func (s *Store) MarkFailedWithBackoff(ctx context.Context, tx pgx.Tx, id uuid.UUID, maxRetries int, backoffDelay time.Duration) error {
	seconds := backoffDelay.Seconds()
	if seconds <= 0 {
		seconds = 1
	}

	var newRetryCount int
	err := tx.QueryRow(ctx, `
		UPDATE outbox_events
		SET retry_count = retry_count + 1
		WHERE id = $1
		RETURNING retry_count`,
		id,
	).Scan(&newRetryCount)
	if err != nil {
		return fmt.Errorf("outbox: mark failed %s: %w", id, err)
	}

	status := StatusPending
	if newRetryCount >= maxRetries {
		status = StatusFailed
	}

	sp, spErr := tx.Begin(ctx)
	if spErr != nil {
		return spErr
	}

	_, err = sp.Exec(ctx, `
		UPDATE outbox_events
		SET status = $2,
		    next_retry_at = NOW() + ($3 || ' seconds')::interval,
		    updated_at = NOW()
		WHERE id = $1`,
		id, status, fmt.Sprintf("%f", seconds),
	)
	if err == nil {
		return sp.Commit(ctx)
	}

	_ = sp.Rollback(ctx)

	// Fallback for tables without next_retry_at / updated_at
	_, err = tx.Exec(ctx, `
		UPDATE outbox_events
		SET status = $2
		WHERE id = $1`,
		id, status,
	)
	if err != nil {
		return fmt.Errorf("outbox: mark failed fallback %s: %w", id, err)
	}

	return nil
}

// RequeueFailed resets FAILED events back to PENDING with retry_count = 0 so
// the publisher will attempt them again.
func (s *Store) RequeueFailed(ctx context.Context, pool *pgxpool.Pool, aggregateType, eventType string) (int64, error) {
	query := `UPDATE outbox_events SET status = 'PENDING', retry_count = 0, next_retry_at = NOW(), updated_at = NOW() WHERE status = 'FAILED'`
	args := []any{}
	n := 1

	if aggregateType != "" {
		query += fmt.Sprintf(" AND aggregate_type = $%d", n)
		args = append(args, aggregateType)
		n++
	}
	if eventType != "" {
		query += fmt.Sprintf(" AND event_type = $%d", n)
		args = append(args, eventType)
	}

	tag, err := pool.Exec(ctx, query, args...)
	if err != nil {
		// Fallback for tables without next_retry_at
		queryFallback := `UPDATE outbox_events SET status = 'PENDING', retry_count = 0 WHERE status = 'FAILED'`
		argsFallback := []any{}
		n = 1
		if aggregateType != "" {
			queryFallback += fmt.Sprintf(" AND aggregate_type = $%d", n)
			argsFallback = append(argsFallback, aggregateType)
			n++
		}
		if eventType != "" {
			queryFallback += fmt.Sprintf(" AND event_type = $%d", n)
			argsFallback = append(argsFallback, eventType)
		}
		tag, err = pool.Exec(ctx, queryFallback, argsFallback...)
		if err != nil {
			return 0, fmt.Errorf("outbox: requeue failed: %w", err)
		}
	}
	return tag.RowsAffected(), nil
}

// CountByStatus returns event counts grouped by status, suitable for exposing
// on a monitoring or admin endpoint.
func (s *Store) CountByStatus(ctx context.Context, pool *pgxpool.Pool) (map[Status]int64, error) {
	rows, err := pool.Query(ctx, `SELECT status, COUNT(*) FROM outbox_events GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("outbox: count by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[Status]int64)
	for rows.Next() {
		var st Status
		var count int64
		if err := rows.Scan(&st, &count); err != nil {
			return nil, fmt.Errorf("outbox: scan count: %w", err)
		}
		counts[st] = count
	}
	return counts, rows.Err()
}
