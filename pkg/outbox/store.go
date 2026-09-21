package outbox

import (
	"context"
	"fmt"

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

	_, err := tx.Exec(ctx, `
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
//
// The caller MUST commit or rollback the transaction. Committing after updating
// each event's status releases the locks and finalises the state change atomically.
// If the publisher crashes, Postgres automatically rolls back the open transaction,
// releasing the locks and leaving events in their original PENDING state.
func (s *Store) FetchAndLockPending(ctx context.Context, tx pgx.Tx, limit int) ([]*Event, error) {
	rows, err := tx.Query(ctx, `
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
// time of successful broker acknowledgement. Must be called within the same
// transaction that FetchAndLockPending was issued on.
func (s *Store) MarkPublished(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `
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

// MarkFailed increments retry_count for an event. If the new count reaches
// maxRetries the event is permanently marked FAILED; otherwise it is reset to
// PENDING so the next poll cycle can retry it. Must be called within the same
// transaction that FetchAndLockPending was issued on.
func (s *Store) MarkFailed(ctx context.Context, tx pgx.Tx, id uuid.UUID, maxRetries int) error {
	_, err := tx.Exec(ctx, `
		UPDATE outbox_events
		SET retry_count = retry_count + 1,
		    status = CASE WHEN retry_count + 1 >= $2 THEN 'FAILED' ELSE 'PENDING' END
		WHERE id = $1`,
		id, maxRetries,
	)
	if err != nil {
		return fmt.Errorf("outbox: mark failed %s: %w", id, err)
	}
	return nil
}

// RequeueFailed resets FAILED events back to PENDING with retry_count = 0 so
// the publisher will attempt them again. Filters are optional; pass empty
// strings to requeue all failed events regardless of type.
func (s *Store) RequeueFailed(ctx context.Context, pool *pgxpool.Pool, aggregateType, eventType string) (int64, error) {
	query := `UPDATE outbox_events SET status = 'PENDING', retry_count = 0 WHERE status = 'FAILED'`
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
		return 0, fmt.Errorf("outbox: requeue failed: %w", err)
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
