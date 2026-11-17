package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
)

const notificationSelectColumns = `
	n.id::text, n.user_id::text, n.channel_id::text, COALESCE(c.code::text, ''),
	n.event_type, n.event_id, n.recipient, n.title, n.content, n.status::text,
	n.error_message, n.is_read, n.read_at, COALESCE(n.correlation_id, ''), n.retry_count,
	n.created_at, n.updated_at, n.sent_at
`

func scanNotification(scanner interface {
	Scan(dest ...any) error
}) (*model.Notification, error) {
	var n model.Notification
	var eventID, errMsg *string
	var readAt, sentAt *time.Time
	var statusStr string

	err := scanner.Scan(
		&n.ID,
		&n.UserID,
		&n.ChannelID,
		&n.ChannelCode,
		&n.EventType,
		&eventID,
		&n.Recipient,
		&n.Title,
		&n.Content,
		&statusStr,
		&errMsg,
		&n.IsRead,
		&readAt,
		&n.CorrelationID,
		&n.RetryCount,
		&n.CreatedAt,
		&n.UpdatedAt,
		&sentAt,
	)
	if err != nil {
		return nil, err
	}
	n.Status = model.DeliveryStatus(statusStr)
	n.EventID = eventID
	n.ErrorMessage = errMsg
	n.ReadAt = readAt
	n.SentAt = sentAt
	return &n, nil
}

func (r *notificationRepository) CreateNotification(ctx context.Context, n *model.Notification) error {
	status := n.Status
	if status == "" {
		status = model.DeliveryStatusPending
	}
	query := `
		INSERT INTO notifications (
			id, user_id, channel_id, event_type, event_id, recipient,
			title, content, status, error_message, is_read, correlation_id,
			retry_count, created_at, updated_at, sent_at
		)
		VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()),
			$2::uuid, $3::uuid, $4, $5, $6,
			$7, $8, $9::notification_delivery_status, $10, $11, $12,
			$13, NOW(), NOW(), $14
		)
		RETURNING id::text, created_at, updated_at
	`
	err := r.pool.QueryRow(
		ctx, query,
		n.ID, n.UserID, n.ChannelID, n.EventType, n.EventID, n.Recipient,
		n.Title, n.Content, string(status), n.ErrorMessage, n.IsRead, n.CorrelationID,
		n.RetryCount, n.SentAt,
	).Scan(&n.ID, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("notification already exists: %w", err)
		}
		return fmt.Errorf("failed to create notification: %w", err)
	}
	n.Status = status
	return nil
}

func (r *notificationRepository) GetNotificationByID(ctx context.Context, id string) (*model.Notification, error) {
	query := `
		SELECT ` + notificationSelectColumns + `
		FROM notifications n
		LEFT JOIN notification_channels c ON n.channel_id = c.id
		WHERE n.id = $1::uuid
		LIMIT 1
	`
	n, err := scanNotification(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get notification by id: %w", err)
	}
	return n, nil
}

func (r *notificationRepository) GetNotificationByEventIDAndChannel(ctx context.Context, eventID, channelID string) (*model.Notification, error) {
	query := `
		SELECT ` + notificationSelectColumns + `
		FROM notifications n
		LEFT JOIN notification_channels c ON n.channel_id = c.id
		WHERE n.event_id = $1 AND n.channel_id = $2::uuid
		LIMIT 1
	`
	n, err := scanNotification(r.pool.QueryRow(ctx, query, eventID, channelID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get notification by event_id and channel: %w", err)
	}
	return n, nil
}

func (r *notificationRepository) ListNotifications(ctx context.Context, filter model.NotificationFilter) ([]*model.Notification, int, int, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	whereParts := []string{"1=1"}
	args := []any{}
	argIdx := 1

	if strings.TrimSpace(filter.UserID) != "" {
		whereParts = append(whereParts, fmt.Sprintf("n.user_id = $%d::uuid", argIdx))
		args = append(args, strings.TrimSpace(filter.UserID))
		argIdx++
	}
	if strings.TrimSpace(filter.Status) != "" {
		whereParts = append(whereParts, fmt.Sprintf("n.status::text = $%d", argIdx))
		args = append(args, strings.ToUpper(strings.TrimSpace(filter.Status)))
		argIdx++
	}
	if strings.TrimSpace(filter.Channel) != "" {
		whereParts = append(whereParts, fmt.Sprintf("c.code::text = $%d", argIdx))
		args = append(args, strings.ToUpper(strings.TrimSpace(filter.Channel)))
		argIdx++
	}
	if strings.TrimSpace(filter.EventType) != "" {
		whereParts = append(whereParts, fmt.Sprintf("n.event_type = $%d", argIdx))
		args = append(args, strings.TrimSpace(filter.EventType))
		argIdx++
	}
	if filter.IsRead != nil {
		whereParts = append(whereParts, fmt.Sprintf("n.is_read = $%d", argIdx))
		args = append(args, *filter.IsRead)
		argIdx++
	}

	whereSQL := strings.Join(whereParts, " AND ")

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM notifications n
		LEFT JOIN notification_channels c ON n.channel_id = c.id
		WHERE %s
	`, whereSQL)

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, 0, fmt.Errorf("failed to count notifications: %w", err)
	}

	unreadCount := 0
	if strings.TrimSpace(filter.UserID) != "" {
		cnt, uErr := r.GetUnreadCount(ctx, strings.TrimSpace(filter.UserID))
		if uErr != nil {
			return nil, 0, 0, uErr
		}
		unreadCount = cnt
	}

	listQuery := fmt.Sprintf(`
		SELECT %s
		FROM notifications n
		LEFT JOIN notification_channels c ON n.channel_id = c.id
		WHERE %s
		ORDER BY n.created_at DESC
		LIMIT $%d OFFSET $%d
	`, notificationSelectColumns, whereSQL, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to list notifications: %w", err)
	}
	defer rows.Close()

	var items []*model.Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("failed to scan notification: %w", err)
		}
		items = append(items, n)
	}
	return items, total, unreadCount, nil
}

func (r *notificationRepository) UpdateDeliveryStatus(ctx context.Context, id string, status model.DeliveryStatus, errMsg *string, incrementRetry bool) error {
	query := `
		UPDATE notifications
		SET status = $2::notification_delivery_status,
		    error_message = $3,
		    retry_count = CASE WHEN $4 THEN retry_count + 1 ELSE retry_count END,
		    sent_at = CASE WHEN $2::notification_delivery_status = 'SENT'::notification_delivery_status THEN COALESCE(sent_at, NOW()) ELSE sent_at END,
		    updated_at = NOW()
		WHERE id = $1::uuid
	`
	_, err := r.pool.Exec(ctx, query, id, string(status), errMsg, incrementRetry)
	if err != nil {
		return fmt.Errorf("failed to update notification delivery status: %w", err)
	}
	return nil
}

func (r *notificationRepository) MarkAsRead(ctx context.Context, id, userID string, isRead bool) (*model.Notification, error) {
	whereParts := []string{"id = $1::uuid"}
	args := []any{id, isRead}
	if strings.TrimSpace(userID) != "" {
		whereParts = append(whereParts, "user_id = $3::uuid")
		args = append(args, strings.TrimSpace(userID))
	}

	query := fmt.Sprintf(`
		WITH updated AS (
			UPDATE notifications
			SET is_read = $2,
			    read_at = CASE WHEN $2 THEN COALESCE(read_at, NOW()) ELSE NULL END,
			    updated_at = NOW()
			WHERE %s
			RETURNING id, user_id, channel_id, event_type, event_id, recipient,
			          title, content, status, error_message, is_read, read_at,
			          correlation_id, retry_count, created_at, updated_at, sent_at
		)
		SELECT n.id::text, n.user_id::text, n.channel_id::text, COALESCE(c.code::text, ''),
		       n.event_type, n.event_id, n.recipient, n.title, n.content, n.status::text,
		       n.error_message, n.is_read, n.read_at, COALESCE(n.correlation_id, ''), n.retry_count,
		       n.created_at, n.updated_at, n.sent_at
		FROM updated n
		LEFT JOIN notification_channels c ON n.channel_id = c.id
	`, strings.Join(whereParts, " AND "))

	n, err := scanNotification(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to mark notification read status: %w", err)
	}
	return n, nil
}

func (r *notificationRepository) MarkAllAsRead(ctx context.Context, userID string) (int, error) {
	query := `
		UPDATE notifications
		SET is_read = TRUE, read_at = NOW(), updated_at = NOW()
		WHERE user_id = $1::uuid AND is_read = FALSE
	`
	tag, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to mark all notifications as read: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *notificationRepository) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	query := `SELECT COUNT(*) FROM notifications WHERE user_id = $1::uuid AND is_read = FALSE`
	var count int
	if err := r.pool.QueryRow(ctx, query, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to get unread count: %w", err)
	}
	return count, nil
}

