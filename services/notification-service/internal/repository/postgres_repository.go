package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// ChannelRepository implementation
func (r *PostgresRepository) GetByCode(ctx context.Context, code string) (*model.Channel, error) {
	query := `
		SELECT id, code, name, is_active, created_at
		FROM notification_channels
		WHERE code = $1 AND is_active = TRUE
		LIMIT 1
	`
	var c model.Channel
	err := r.pool.QueryRow(ctx, query, code).Scan(&c.ID, &c.Code, &c.Name, &c.IsActive, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get channel by code: %w", err)
	}
	return &c, nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id string) (*model.Channel, error) {
	query := `
		SELECT id, code, name, is_active, created_at
		FROM notification_channels
		WHERE id = $1
		LIMIT 1
	`
	var c model.Channel
	err := r.pool.QueryRow(ctx, query, id).Scan(&c.ID, &c.Code, &c.Name, &c.IsActive, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get channel by id: %w", err)
	}
	return &c, nil
}

// TemplateRepository implementation
func (r *PostgresRepository) GetByEventType(ctx context.Context, eventType string) (*model.Template, error) {
	query := `
		SELECT id, event_type, channel_id, subject_template, body_template, is_active, created_at, updated_at
		FROM notification_templates
		WHERE event_type = $1 AND is_active = TRUE
		LIMIT 1
	`
	var t model.Template
	err := r.pool.QueryRow(ctx, query, eventType).Scan(
		&t.ID, &t.EventType, &t.ChannelID, &t.SubjectTemplate, &t.BodyTemplate, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get template by event type: %w", err)
	}
	return &t, nil
}

func (r *PostgresRepository) GetByEventTypeAndChannel(ctx context.Context, eventType string, channelID string) (*model.Template, error) {
	query := `
		SELECT id, event_type, channel_id, subject_template, body_template, is_active, created_at, updated_at
		FROM notification_templates
		WHERE event_type = $1 AND channel_id = $2 AND is_active = TRUE
		LIMIT 1
	`
	var t model.Template
	err := r.pool.QueryRow(ctx, query, eventType, channelID).Scan(
		&t.ID, &t.EventType, &t.ChannelID, &t.SubjectTemplate, &t.BodyTemplate, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get template by event type and channel: %w", err)
	}
	return &t, nil
}

// PreferenceRepository implementation
func (r *PostgresRepository) GetPreference(ctx context.Context, userID, channelID, eventType string) (*model.Preference, error) {
	query := `
		SELECT id, user_id, channel_id, event_type, is_enabled, created_at, updated_at
		FROM user_notification_preferences
		WHERE user_id = $1 AND channel_id = $2 AND event_type = $3
		LIMIT 1
	`
	var p model.Preference
	err := r.pool.QueryRow(ctx, query, userID, channelID, eventType).Scan(
		&p.ID, &p.UserID, &p.ChannelID, &p.EventType, &p.IsEnabled, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user preference: %w", err)
	}
	return &p, nil
}

func (r *PostgresRepository) UpsertPreference(ctx context.Context, pref *model.Preference) error {
	query := `
		INSERT INTO user_notification_preferences (user_id, channel_id, event_type, is_enabled, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (user_id, channel_id, event_type)
		DO UPDATE SET is_enabled = EXCLUDED.is_enabled, updated_at = NOW()
	`
	_, err := r.pool.Exec(ctx, query, pref.UserID, pref.ChannelID, pref.EventType, pref.IsEnabled)
	if err != nil {
		return fmt.Errorf("failed to upsert preference: %w", err)
	}
	return nil
}

// JobRepository implementation
func (r *PostgresRepository) CreateJob(ctx context.Context, job *model.NotificationJob) error {
	query := `
		INSERT INTO notification_jobs (
			id, event_id, user_id, channel_id, template_id, event_type, recipient,
			subject, content, status, retry_count, max_retries, correlation_id, error_message,
			created_at, updated_at
		)
		VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()),
			$2, NULLIF($3, '')::uuid, $4::uuid, $5::uuid, $6, $7,
			$8, $9, $10, $11, $12, $13, $14,
			NOW(), NOW()
		)
		RETURNING id, created_at, updated_at
	`
	var userIDVal *string
	if job.UserID != nil && *job.UserID != "" {
		userIDVal = job.UserID
	}

	err := r.pool.QueryRow(
		ctx, query,
		job.ID, job.EventID, userIDVal, job.ChannelID, job.TemplateID, job.EventType, job.Recipient,
		job.Subject, job.Content, job.Status, job.RetryCount, job.MaxRetries, job.CorrelationID, job.ErrorMessage,
	).Scan(&job.ID, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // Unique violation
			return fmt.Errorf("job already exists for event_id %s: %w", job.EventID, err)
		}
		return fmt.Errorf("failed to create notification job: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetJobByEventID(ctx context.Context, eventID string) (*model.NotificationJob, error) {
	query := `
		SELECT id, event_id, user_id::text, channel_id::text, template_id::text, event_type, recipient,
		       subject, content, status, retry_count, max_retries, correlation_id, error_message,
		       created_at, updated_at, processed_at
		FROM notification_jobs
		WHERE event_id = $1
		LIMIT 1
	`
	var j model.NotificationJob
	var userID *string
	var processedAt *time.Time
	var errMsg *string
	var corrID *string

	err := r.pool.QueryRow(ctx, query, eventID).Scan(
		&j.ID, &j.EventID, &userID, &j.ChannelID, &j.TemplateID, &j.EventType, &j.Recipient,
		&j.Subject, &j.Content, &j.Status, &j.RetryCount, &j.MaxRetries, &corrID, &errMsg,
		&j.CreatedAt, &j.UpdatedAt, &processedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get job by event_id: %w", err)
	}
	j.UserID = userID
	j.ProcessedAt = processedAt
	j.ErrorMessage = errMsg
	if corrID != nil {
		j.CorrelationID = *corrID
	}
	return &j, nil
}

func (r *PostgresRepository) GetJobByID(ctx context.Context, id string) (*model.NotificationJob, error) {
	query := `
		SELECT id, event_id, user_id::text, channel_id::text, template_id::text, event_type, recipient,
		       subject, content, status, retry_count, max_retries, correlation_id, error_message,
		       created_at, updated_at, processed_at
		FROM notification_jobs
		WHERE id = $1::uuid
		LIMIT 1
	`
	var j model.NotificationJob
	var userID *string
	var processedAt *time.Time
	var errMsg *string
	var corrID *string

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&j.ID, &j.EventID, &userID, &j.ChannelID, &j.TemplateID, &j.EventType, &j.Recipient,
		&j.Subject, &j.Content, &j.Status, &j.RetryCount, &j.MaxRetries, &corrID, &errMsg,
		&j.CreatedAt, &j.UpdatedAt, &processedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get job by id: %w", err)
	}
	j.UserID = userID
	j.ProcessedAt = processedAt
	j.ErrorMessage = errMsg
	if corrID != nil {
		j.CorrelationID = *corrID
	}
	return &j, nil
}

func (r *PostgresRepository) UpdateJobStatus(ctx context.Context, id string, status model.JobStatus, errMsg *string) error {
	query := `
		UPDATE notification_jobs
		SET status = $2, error_message = $3, updated_at = NOW(),
		    processed_at = CASE WHEN $2 IN ('COMPLETED', 'OPTED_OUT', 'FAILED') THEN NOW() ELSE processed_at END
		WHERE id = $1::uuid
	`
	_, err := r.pool.Exec(ctx, query, id, status, errMsg)
	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}
	return nil
}

func (r *PostgresRepository) IncrementRetry(ctx context.Context, id string, errMsg string) error {
	query := `
		UPDATE notification_jobs
		SET retry_count = retry_count + 1,
		    error_message = $2,
		    status = CASE WHEN retry_count + 1 >= max_retries THEN 'FAILED'::notification_job_status ELSE 'PENDING'::notification_job_status END,
		    updated_at = NOW()
		WHERE id = $1::uuid
	`
	_, err := r.pool.Exec(ctx, query, id, errMsg)
	if err != nil {
		return fmt.Errorf("failed to increment job retry: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListPendingJobs(ctx context.Context, limit int) ([]*model.NotificationJob, error) {
	query := `
		SELECT id, event_id, user_id::text, channel_id::text, template_id::text, event_type, recipient,
		       subject, content, status, retry_count, max_retries, correlation_id, error_message,
		       created_at, updated_at, processed_at
		FROM notification_jobs
		WHERE status = 'PENDING' AND retry_count < max_retries
		ORDER BY created_at ASC
		LIMIT $1
	`
	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*model.NotificationJob
	for rows.Next() {
		var j model.NotificationJob
		var userID *string
		var processedAt *time.Time
		var errMsg *string
		var corrID *string
		if err := rows.Scan(
			&j.ID, &j.EventID, &userID, &j.ChannelID, &j.TemplateID, &j.EventType, &j.Recipient,
			&j.Subject, &j.Content, &j.Status, &j.RetryCount, &j.MaxRetries, &corrID, &errMsg,
			&j.CreatedAt, &j.UpdatedAt, &processedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan job: %w", err)
		}
		j.UserID = userID
		j.ProcessedAt = processedAt
		j.ErrorMessage = errMsg
		if corrID != nil {
			j.CorrelationID = *corrID
		}
		jobs = append(jobs, &j)
	}
	return jobs, nil
}

// IdempotencyRepository implementation
func (r *PostgresRepository) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	query := `
		SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1)
	`
	var exists bool
	err := r.pool.QueryRow(ctx, query, eventID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check processed event: %w", err)
	}
	return exists, nil
}

func (r *PostgresRepository) RecordProcessedEvent(ctx context.Context, event *model.ProcessedEvent) error {
	query := `
		INSERT INTO processed_events (event_id, event_type, source, processed_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (event_id) DO NOTHING
	`
	_, err := r.pool.Exec(ctx, query, event.EventID, event.EventType, event.Source)
	if err != nil {
		return fmt.Errorf("failed to record processed event: %w", err)
	}
	return nil
}

