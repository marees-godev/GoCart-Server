package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
)

func scanPushSubscription(scanner interface {
	Scan(dest ...any) error
}) (*model.PushSubscription, error) {
	var sub model.PushSubscription
	var lastErr *string
	var lastUsedAt, deactivatedAt *time.Time
	err := scanner.Scan(
		&sub.ID,
		&sub.UserID,
		&sub.Endpoint,
		&sub.P256DH,
		&sub.Auth,
		&sub.UserAgent,
		&sub.DeviceName,
		&sub.IsActive,
		&sub.FailureCount,
		&lastErr,
		&lastUsedAt,
		&deactivatedAt,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	sub.LastError = lastErr
	sub.LastUsedAt = lastUsedAt
	sub.DeactivatedAt = deactivatedAt
	return &sub, nil
}

const pushSubSelectColumns = `
	id::text, user_id::text, endpoint, p256dh, auth, user_agent, device_name,
	is_active, failure_count, last_error, last_used_at, deactivated_at, created_at, updated_at
`

func (r *notificationRepository) UpsertSubscription(ctx context.Context, sub *model.PushSubscription) (*model.PushSubscription, error) {
	query := `
		INSERT INTO push_subscriptions (
			id, user_id, endpoint, p256dh, auth, user_agent, device_name,
			is_active, failure_count, last_error, deactivated_at, created_at, updated_at
		)
		VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()),
			$2::uuid, $3, $4, $5, $6, $7,
			TRUE, 0, NULL, NULL, NOW(), NOW()
		)
		ON CONFLICT (endpoint) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			p256dh = EXCLUDED.p256dh,
			auth = EXCLUDED.auth,
			user_agent = CASE WHEN EXCLUDED.user_agent <> '' THEN EXCLUDED.user_agent ELSE push_subscriptions.user_agent END,
			device_name = CASE WHEN EXCLUDED.device_name <> '' THEN EXCLUDED.device_name ELSE push_subscriptions.device_name END,
			is_active = TRUE,
			failure_count = 0,
			last_error = NULL,
			deactivated_at = NULL,
			updated_at = NOW()
		RETURNING ` + pushSubSelectColumns

	res, err := scanPushSubscription(r.pool.QueryRow(
		ctx, query,
		sub.ID, sub.UserID, sub.Endpoint, sub.P256DH, sub.Auth, sub.UserAgent, sub.DeviceName,
	))
	if err != nil {
		return nil, fmt.Errorf("failed to upsert push subscription: %w", err)
	}
	return res, nil
}

func (r *notificationRepository) GetSubscriptionByID(ctx context.Context, id string) (*model.PushSubscription, error) {
	query := `SELECT ` + pushSubSelectColumns + ` FROM push_subscriptions WHERE id = $1::uuid LIMIT 1`
	sub, err := scanPushSubscription(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get push subscription by id: %w", err)
	}
	return sub, nil
}

func (r *notificationRepository) GetSubscriptionByEndpoint(ctx context.Context, endpoint string) (*model.PushSubscription, error) {
	query := `SELECT ` + pushSubSelectColumns + ` FROM push_subscriptions WHERE endpoint = $1 LIMIT 1`
	sub, err := scanPushSubscription(r.pool.QueryRow(ctx, query, endpoint))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get push subscription by endpoint: %w", err)
	}
	return sub, nil
}

func (r *notificationRepository) ListSubscriptionsByUserID(ctx context.Context, userID string, activeOnly bool) ([]*model.PushSubscription, error) {
	query := `SELECT ` + pushSubSelectColumns + ` FROM push_subscriptions WHERE user_id = $1::uuid`
	if activeOnly {
		query += ` AND is_active = TRUE`
	}
	query += ` ORDER BY updated_at DESC, created_at DESC`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list push subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []*model.PushSubscription
	for rows.Next() {
		sub, err := scanPushSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan push subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, nil
}

func (r *notificationRepository) UpdateSubscription(ctx context.Context, sub *model.PushSubscription) (*model.PushSubscription, error) {
	setClauses := []string{"updated_at = NOW()"}
	args := []any{}
	argIdx := 1

	if strings.TrimSpace(sub.Endpoint) != "" {
		setClauses = append(setClauses, fmt.Sprintf("endpoint = $%d", argIdx))
		args = append(args, sub.Endpoint)
		argIdx++
	}
	if strings.TrimSpace(sub.P256DH) != "" {
		setClauses = append(setClauses, fmt.Sprintf("p256dh = $%d", argIdx))
		args = append(args, sub.P256DH)
		argIdx++
	}
	if strings.TrimSpace(sub.Auth) != "" {
		setClauses = append(setClauses, fmt.Sprintf("auth = $%d", argIdx))
		args = append(args, sub.Auth)
		argIdx++
	}
	if strings.TrimSpace(sub.UserAgent) != "" {
		setClauses = append(setClauses, fmt.Sprintf("user_agent = $%d", argIdx))
		args = append(args, sub.UserAgent)
		argIdx++
	}
	if strings.TrimSpace(sub.DeviceName) != "" {
		setClauses = append(setClauses, fmt.Sprintf("device_name = $%d", argIdx))
		args = append(args, sub.DeviceName)
		argIdx++
	}
	setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", argIdx))
	args = append(args, sub.IsActive)
	argIdx++

	if sub.IsActive {
		setClauses = append(setClauses, "deactivated_at = NULL", "failure_count = 0", "last_error = NULL")
	} else {
		setClauses = append(setClauses, "deactivated_at = COALESCE(deactivated_at, NOW())")
	}

	whereClause := ""
	if strings.TrimSpace(sub.ID) != "" {
		whereClause = fmt.Sprintf("id = $%d::uuid", argIdx)
		args = append(args, sub.ID)
		argIdx++
	} else if strings.TrimSpace(sub.Endpoint) != "" {
		whereClause = fmt.Sprintf("endpoint = $%d", argIdx)
		args = append(args, sub.Endpoint)
		argIdx++
	} else {
		return nil, fmt.Errorf("id or endpoint is required to update subscription")
	}

	if strings.TrimSpace(sub.UserID) != "" {
		whereClause += fmt.Sprintf(" AND user_id = $%d::uuid", argIdx)
		args = append(args, sub.UserID)
	}

	query := fmt.Sprintf(
		"UPDATE push_subscriptions SET %s WHERE %s RETURNING %s",
		strings.Join(setClauses, ", "),
		whereClause,
		pushSubSelectColumns,
	)

	updated, err := scanPushSubscription(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to update push subscription: %w", err)
	}
	return updated, nil
}

func (r *notificationRepository) DeactivateSubscription(ctx context.Context, id, userID, endpoint string) (bool, error) {
	whereParts := []string{}
	args := []any{}
	argIdx := 1

	if strings.TrimSpace(id) != "" {
		whereParts = append(whereParts, fmt.Sprintf("id = $%d::uuid", argIdx))
		args = append(args, strings.TrimSpace(id))
		argIdx++
	} else if strings.TrimSpace(endpoint) != "" {
		whereParts = append(whereParts, fmt.Sprintf("endpoint = $%d", argIdx))
		args = append(args, strings.TrimSpace(endpoint))
		argIdx++
	} else {
		return false, fmt.Errorf("id or endpoint is required to deactivate subscription")
	}

	if strings.TrimSpace(userID) != "" {
		whereParts = append(whereParts, fmt.Sprintf("user_id = $%d::uuid", argIdx))
		args = append(args, strings.TrimSpace(userID))
	}

	query := fmt.Sprintf(`
		UPDATE push_subscriptions
		SET is_active = FALSE, deactivated_at = COALESCE(deactivated_at, NOW()), updated_at = NOW()
		WHERE %s
	`, strings.Join(whereParts, " AND "))

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("failed to deactivate push subscription: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *notificationRepository) MarkSubscriptionGone(ctx context.Context, id string, errMsg string) error {
	query := `
		UPDATE push_subscriptions
		SET is_active = FALSE,
		    failure_count = failure_count + 1,
		    last_error = $2,
		    deactivated_at = COALESCE(deactivated_at, NOW()),
		    updated_at = NOW()
		WHERE id = $1::uuid
	`
	_, err := r.pool.Exec(ctx, query, id, errMsg)
	if err != nil {
		return fmt.Errorf("failed to mark push subscription gone: %w", err)
	}
	return nil
}

func (r *notificationRepository) RecordSubscriptionFailure(ctx context.Context, id string, errMsg string) error {
	query := `
		UPDATE push_subscriptions
		SET failure_count = failure_count + 1,
		    last_error = $2,
		    updated_at = NOW()
		WHERE id = $1::uuid
	`
	_, err := r.pool.Exec(ctx, query, id, errMsg)
	if err != nil {
		return fmt.Errorf("failed to record push subscription failure: %w", err)
	}
	return nil
}

func (r *notificationRepository) MarkSubscriptionUsed(ctx context.Context, id string) error {
	query := `
		UPDATE push_subscriptions
		SET failure_count = 0,
		    last_error = NULL,
		    last_used_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1::uuid
	`
	_, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to mark push subscription used: %w", err)
	}
	return nil
}
