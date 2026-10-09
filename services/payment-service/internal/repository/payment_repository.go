package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
)

type PaymentRepository interface {
	CreatePayment(ctx context.Context, payment *model.Payment) error
	GetPaymentByID(ctx context.Context, id string) (*model.Payment, error)
	GetPaymentByOrderID(ctx context.Context, orderID string) (*model.Payment, error)
	GetPaymentByGatewayTransactionID(ctx context.Context, gatewayTxID string) (*model.Payment, error)
	GetPaymentByIdempotencyKey(ctx context.Context, idempotencyKey string) (*model.Payment, error)
	UpdatePaymentStatus(ctx context.Context, id string, status model.PaymentStatus, gatewayTxID string, failureReason string) (*model.Payment, error)
	CreateRefund(ctx context.Context, refund *model.Refund) error
	GetRefundByID(ctx context.Context, id string) (*model.Refund, error)
	GetRefundByGatewayRefundID(ctx context.Context, gatewayRefundID string) (*model.Refund, error)
	UpdateRefundStatus(ctx context.Context, id string, status model.RefundStatus, gatewayRefundID string) (*model.Refund, error)
	SaveWebhookEvent(ctx context.Context, w *model.PaymentWebhook) error
	IsWebhookProcessed(ctx context.Context, eventID string) (bool, error)
	MarkWebhookProcessed(ctx context.Context, eventID string) error
}

type pgPaymentRepository struct {
	pool *pgxpool.Pool
}

func NewPaymentRepository(pool *pgxpool.Pool) PaymentRepository {
	return &pgPaymentRepository{
		pool: pool,
	}
}

func (r *pgPaymentRepository) CreatePayment(ctx context.Context, p *model.Payment) error {
	if p == nil {
		slog.WarnContext(ctx, "payment model is nil in CreatePayment")
		return appErrors.BadRequest("payment model cannot be nil")
	}

	query := `
		INSERT INTO payments (
			id,
			order_id,
			user_id,
			payment_method,
			payment_method_id,
			amount,
			currency,
			status,
			transaction_id,
			gateway_transaction_id,
			idempotency_key,
			failure_reason,
			created_at,
			updated_at
		) VALUES (
			COALESCE(NULLIF($1::text, '')::uuid, gen_random_uuid()),
			$2::uuid,
			$3::uuid,
			$4::text,
			COALESCE(
				$5::uuid,
				(SELECT id FROM payment_methods WHERE code = $4::text LIMIT 1),
				(SELECT id FROM payment_methods WHERE code = UPPER($4::text) LIMIT 1),
				(SELECT id FROM payment_methods WHERE code = REPLACE(UPPER($4::text), '_', '') LIMIT 1)
			),
			$6,
			$7::text,
			$8,
			NULLIF($9::text, ''),
			NULLIF($10::text, ''),
			$11::text,
			NULLIF($12::text, ''),
			NOW(),
			NOW()
		)
		RETURNING id, payment_method_id, created_at, updated_at
	`

	var pIDParam *string
	if p.ID != "" {
		if _, err := uuid.Parse(p.ID); err == nil {
			pIDParam = &p.ID
		}
	}

	var pmIDParam *string
	if p.PaymentMethodID != nil && *p.PaymentMethodID != "" {
		if _, err := uuid.Parse(*p.PaymentMethodID); err == nil {
			pmIDParam = p.PaymentMethodID
		}
	}

	var scannedPmID *string
	err := r.pool.QueryRow(ctx, query,
		pIDParam,
		p.OrderID,
		p.UserID,
		p.PaymentMethod,
		pmIDParam,
		p.Amount,
		p.Currency,
		p.Status,
		p.TransactionID,
		p.GatewayTransactionID,
		p.IdempotencyKey,
		p.FailureReason,
	).Scan(&p.ID, &scannedPmID, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			slog.WarnContext(ctx, "duplicate payment creation attempt in repository", "idempotency_key", p.IdempotencyKey, "order_id", p.OrderID)
			return appErrors.Conflict("payment request with this idempotency key already exists")
		}
		slog.ErrorContext(ctx, "failed to create payment in repository", "order_id", p.OrderID, "error", err)
		return appErrors.Internal(err, "failed to create payment record")
	}

	if scannedPmID != nil && *scannedPmID != "" {
		p.PaymentMethodID = scannedPmID
	}

	slog.InfoContext(ctx, "payment created in repository", "payment_id", p.ID, "order_id", p.OrderID, "payment_method_id", p.PaymentMethodID)
	return nil
}

func (r *pgPaymentRepository) GetPaymentByID(ctx context.Context, id string) (*model.Payment, error) {
	if _, err := uuid.Parse(id); err != nil {
		p, gErr := r.GetPaymentByGatewayTransactionID(ctx, id)
		if gErr == nil && p != nil {
			return p, nil
		}
		p, kErr := r.GetPaymentByIdempotencyKey(ctx, id)
		if kErr == nil && p != nil {
			return p, nil
		}
		slog.WarnContext(ctx, "invalid uuid provided for payment id lookup", "payment_id", id)
		return nil, appErrors.NotFound(fmt.Sprintf("payment with id %s not found", id))
	}
	query := `
		SELECT
			id,
			order_id,
			user_id,
			payment_method,
			payment_method_id,
			amount,
			currency,
			status,
			transaction_id,
			COALESCE(gateway_transaction_id, ''),
			idempotency_key,
			COALESCE(failure_reason, ''),
			created_at,
			updated_at
		FROM payments
		WHERE id = $1::uuid
	`

	var p model.Payment
	var pmID *string
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID,
		&p.OrderID,
		&p.UserID,
		&p.PaymentMethod,
		&pmID,
		&p.Amount,
		&p.Currency,
		&p.Status,
		&p.TransactionID,
		&p.GatewayTransactionID,
		&p.IdempotencyKey,
		&p.FailureReason,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "payment not found by id in repository", "payment_id", id)
			return nil, appErrors.NotFound(fmt.Sprintf("payment with id %s not found", id))
		}
		slog.ErrorContext(ctx, "failed to fetch payment by id in repository", "payment_id", id, "error", err)
		return nil, appErrors.Internal(err, "failed to fetch payment by id")
	}

	p.PaymentMethodID = pmID
	slog.InfoContext(ctx, "payment retrieved by id in repository", "payment_id", p.ID)
	return &p, nil
}

func (r *pgPaymentRepository) GetPaymentByOrderID(ctx context.Context, orderID string) (*model.Payment, error) {
	query := `
		SELECT
			id,
			order_id,
			user_id,
			payment_method,
			payment_method_id,
			amount,
			currency,
			status,
			transaction_id,
			COALESCE(gateway_transaction_id, ''),
			idempotency_key,
			COALESCE(failure_reason, ''),
			created_at,
			updated_at
		FROM payments
		WHERE order_id::text = $1 OR gateway_transaction_id = $1 OR idempotency_key = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	var p model.Payment
	var pmID *string
	err := r.pool.QueryRow(ctx, query, orderID).Scan(
		&p.ID,
		&p.OrderID,
		&p.UserID,
		&p.PaymentMethod,
		&pmID,
		&p.Amount,
		&p.Currency,
		&p.Status,
		&p.TransactionID,
		&p.GatewayTransactionID,
		&p.IdempotencyKey,
		&p.FailureReason,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "payment not found by order_id in repository", "order_id", orderID)
			return nil, appErrors.NotFound(fmt.Sprintf("payment for order %s not found", orderID))
		}
		slog.ErrorContext(ctx, "failed to fetch payment by order_id in repository", "order_id", orderID, "error", err)
		return nil, appErrors.Internal(err, "failed to fetch payment by order id")
	}

	p.PaymentMethodID = pmID
	slog.InfoContext(ctx, "payment retrieved by order_id in repository", "order_id", orderID, "payment_id", p.ID)
	return &p, nil
}

func (r *pgPaymentRepository) GetPaymentByGatewayTransactionID(ctx context.Context, gatewayTxID string) (*model.Payment, error) {
	if gatewayTxID == "" {
		return nil, appErrors.NotFound("empty gateway transaction id")
	}
	query := `
		SELECT
			id,
			order_id,
			user_id,
			payment_method,
			payment_method_id,
			amount,
			currency,
			status,
			transaction_id,
			COALESCE(gateway_transaction_id, ''),
			idempotency_key,
			COALESCE(failure_reason, ''),
			created_at,
			updated_at
		FROM payments
		WHERE gateway_transaction_id = $1 OR transaction_id = $1 OR idempotency_key = $1 OR order_id::text = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	var p model.Payment
	var pmID *string
	err := r.pool.QueryRow(ctx, query, gatewayTxID).Scan(
		&p.ID,
		&p.OrderID,
		&p.UserID,
		&p.PaymentMethod,
		&pmID,
		&p.Amount,
		&p.Currency,
		&p.Status,
		&p.TransactionID,
		&p.GatewayTransactionID,
		&p.IdempotencyKey,
		&p.FailureReason,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "payment not found by gateway_transaction_id in repository", "gateway_transaction_id", gatewayTxID)
			return nil, appErrors.NotFound(fmt.Sprintf("payment for gateway transaction %s not found", gatewayTxID))
		}
		slog.ErrorContext(ctx, "failed to fetch payment by gateway_transaction_id in repository", "gateway_transaction_id", gatewayTxID, "error", err)
		return nil, appErrors.Internal(err, "failed to fetch payment by gateway transaction id")
	}

	p.PaymentMethodID = pmID
	slog.InfoContext(ctx, "payment retrieved by gateway_transaction_id in repository", "gateway_transaction_id", gatewayTxID, "payment_id", p.ID)
	return &p, nil
}

func (r *pgPaymentRepository) GetPaymentByIdempotencyKey(ctx context.Context, key string) (*model.Payment, error) {
	query := `
		SELECT
			id,
			order_id,
			user_id,
			payment_method,
			payment_method_id,
			amount,
			currency,
			status,
			transaction_id,
			COALESCE(gateway_transaction_id, ''),
			idempotency_key,
			COALESCE(failure_reason, ''),
			created_at,
			updated_at
		FROM payments
		WHERE idempotency_key = $1
	`

	var p model.Payment
	var pmID *string
	err := r.pool.QueryRow(ctx, query, key).Scan(
		&p.ID,
		&p.OrderID,
		&p.UserID,
		&p.PaymentMethod,
		&pmID,
		&p.Amount,
		&p.Currency,
		&p.Status,
		&p.TransactionID,
		&p.GatewayTransactionID,
		&p.IdempotencyKey,
		&p.FailureReason,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "payment not found by idempotency_key in repository", "idempotency_key", key)
			return nil, appErrors.NotFound("payment not found by idempotency key")
		}
		slog.ErrorContext(ctx, "failed to query payment by idempotency_key in repository", "idempotency_key", key, "error", err)
		return nil, appErrors.Internal(err, "failed to query payment by idempotency key")
	}

	p.PaymentMethodID = pmID
	slog.InfoContext(ctx, "payment retrieved by idempotency_key in repository", "idempotency_key", key, "payment_id", p.ID)
	return &p, nil
}

func (r *pgPaymentRepository) UpdatePaymentStatus(ctx context.Context, id string, status model.PaymentStatus, gatewayTxID string, failureReason string) (*model.Payment, error) {
	query := `
		UPDATE payments
		SET status = $2,
		    gateway_transaction_id = COALESCE(NULLIF($3, ''), gateway_transaction_id),
		    failure_reason = COALESCE(NULLIF($4, ''), failure_reason),
		    updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING
			id,
			order_id,
			user_id,
			payment_method,
			payment_method_id,
			amount,
			currency,
			status,
			transaction_id,
			COALESCE(gateway_transaction_id, ''),
			idempotency_key,
			COALESCE(failure_reason, ''),
			created_at,
			updated_at
	`

	var p model.Payment
	var pmID *string
	err := r.pool.QueryRow(ctx, query, id, status, gatewayTxID, failureReason).Scan(
		&p.ID,
		&p.OrderID,
		&p.UserID,
		&p.PaymentMethod,
		&pmID,
		&p.Amount,
		&p.Currency,
		&p.Status,
		&p.TransactionID,
		&p.GatewayTransactionID,
		&p.IdempotencyKey,
		&p.FailureReason,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "payment not found for status update in repository", "payment_id", id)
			return nil, appErrors.NotFound(fmt.Sprintf("payment %s not found for status update", id))
		}
		slog.ErrorContext(ctx, "failed to update payment status in repository", "payment_id", id, "status", status, "error", err)
		return nil, appErrors.Internal(err, "failed to update payment status")
	}

	p.PaymentMethodID = pmID

	// Insert Outbox Event on status updates (PaymentSuccessful or PaymentFailed)
	if status == model.PaymentStatusSuccess {
		env, envErr := events.NewEventEnvelope(events.EventTypePaymentSuccessful, "payment-service", events.PaymentSuccessfulEvent{
			PaymentID:     p.ID,
			OrderID:       p.OrderID,
			UserID:        p.UserID,
			Amount:        p.Amount,
			Currency:      p.Currency,
			PaymentMethod: p.PaymentMethod,
			PaidAt:        p.UpdatedAt,
		})
		if envErr == nil {
			payloadBytes, _ := env.Marshal()
			outboxQuery := `
				INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, topic, status, retry_count, created_at)
				VALUES ('payment', $1, $2, $3, $4, 'PENDING', 0, NOW())
			`
			_, _ = r.pool.Exec(ctx, outboxQuery, p.ID, events.EventTypePaymentSuccessful, payloadBytes, "gocart.payment.payment-successful")
		}
	} else if status == model.PaymentStatusFailed {
		env, envErr := events.NewEventEnvelope(events.EventTypePaymentFailed, "payment-service", events.PaymentFailedEvent{
			PaymentID: p.ID,
			OrderID:   p.OrderID,
			UserID:    p.UserID,
			Amount:    p.Amount,
			Reason:    p.FailureReason,
			FailedAt:  p.UpdatedAt,
		})
		if envErr == nil {
			payloadBytes, _ := env.Marshal()
			outboxQuery := `
				INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, topic, status, retry_count, created_at)
				VALUES ('payment', $1, $2, $3, $4, 'PENDING', 0, NOW())
			`
			_, _ = r.pool.Exec(ctx, outboxQuery, p.ID, events.EventTypePaymentFailed, payloadBytes, events.TopicPaymentFailed)
		}
	}

	slog.InfoContext(ctx, "payment status updated in repository", "payment_id", p.ID, "status", p.Status)
	return &p, nil
}

func (r *pgPaymentRepository) CreateRefund(ctx context.Context, ref *model.Refund) error {
	if ref == nil {
		slog.WarnContext(ctx, "refund model is nil in CreateRefund")
		return appErrors.BadRequest("refund model cannot be nil")
	}

	query := `
		INSERT INTO refunds (
			payment_id,
			order_id,
			amount,
			reason,
			status,
			gateway_refund_id,
			idempotency_key,
			created_at,
			updated_at
		) VALUES (
			$1::uuid,
			$2::uuid,
			$3,
			$4,
			$5,
			$6,
			$7,
			NOW(),
			NOW()
		)
		RETURNING id, created_at, updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		ref.PaymentID,
		ref.OrderID,
		ref.Amount,
		ref.Reason,
		ref.Status,
		ref.GatewayRefundID,
		ref.IdempotencyKey,
	).Scan(&ref.ID, &ref.CreatedAt, &ref.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			slog.WarnContext(ctx, "duplicate refund creation attempt in repository", "idempotency_key", ref.IdempotencyKey)
			return appErrors.Conflict("refund request with this idempotency key already exists")
		}
		slog.ErrorContext(ctx, "failed to create refund in repository", "payment_id", ref.PaymentID, "error", err)
		return appErrors.Internal(err, "failed to create refund record")
	}

	slog.InfoContext(ctx, "refund created in repository", "refund_id", ref.ID, "payment_id", ref.PaymentID)
	return nil
}

func (r *pgPaymentRepository) GetRefundByID(ctx context.Context, id string) (*model.Refund, error) {
	query := `
		SELECT
			id,
			payment_id,
			order_id,
			amount,
			COALESCE(reason, ''),
			status,
			COALESCE(gateway_refund_id, ''),
			COALESCE(idempotency_key, ''),
			created_at,
			updated_at
		FROM refunds
		WHERE id = $1::uuid
	`

	var ref model.Refund
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&ref.ID,
		&ref.PaymentID,
		&ref.OrderID,
		&ref.Amount,
		&ref.Reason,
		&ref.Status,
		&ref.GatewayRefundID,
		&ref.IdempotencyKey,
		&ref.CreatedAt,
		&ref.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "refund not found by id in repository", "refund_id", id)
			return nil, appErrors.NotFound(fmt.Sprintf("refund with id %s not found", id))
		}
		slog.ErrorContext(ctx, "failed to query refund by id in repository", "refund_id", id, "error", err)
		return nil, appErrors.Internal(err, "failed to query refund by id")
	}

	slog.InfoContext(ctx, "refund retrieved by id in repository", "refund_id", ref.ID)
	return &ref, nil
}

func (r *pgPaymentRepository) SaveWebhookEvent(ctx context.Context, w *model.PaymentWebhook) error {
	if w == nil {
		return appErrors.BadRequest("payment webhook model cannot be nil")
	}

	query := `
		INSERT INTO payment_webhooks (
			event_id, provider, event_type, payload, processed, created_at
		) VALUES (
			$1, $2, $3, $4, $5, NOW()
		)
		ON CONFLICT (event_id) DO NOTHING
		RETURNING id, created_at
	`

	err := r.pool.QueryRow(ctx, query, w.EventID, w.Provider, w.EventType, w.Payload, w.Processed).Scan(&w.ID, &w.CreatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.ErrorContext(ctx, "failed to save payment webhook event in repository", "event_id", w.EventID, "error", err)
		return appErrors.Internal(err, "failed to save webhook event")
	}
	return nil
}

func (r *pgPaymentRepository) IsWebhookProcessed(ctx context.Context, eventID string) (bool, error) {
	query := `SELECT processed FROM payment_webhooks WHERE event_id = $1`
	var processed bool
	err := r.pool.QueryRow(ctx, query, eventID).Scan(&processed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, appErrors.Internal(err, "failed to check webhook status")
	}
	return processed, nil
}

func (r *pgPaymentRepository) MarkWebhookProcessed(ctx context.Context, eventID string) error {
	query := `
		UPDATE payment_webhooks
		SET processed = TRUE, processed_at = NOW()
		WHERE event_id = $1
	`
	_, err := r.pool.Exec(ctx, query, eventID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to mark webhook event as processed", "event_id", eventID, "error", err)
		return appErrors.Internal(err, "failed to mark webhook event processed")
	}
	return nil
}

func (r *pgPaymentRepository) GetRefundByGatewayRefundID(ctx context.Context, gatewayRefundID string) (*model.Refund, error) {
	query := `
		SELECT id, payment_id, order_id, amount, COALESCE(reason, ''), status, COALESCE(gateway_refund_id, ''), COALESCE(idempotency_key, ''), created_at, updated_at
		FROM refunds
		WHERE gateway_refund_id = $1
	`
	var ref model.Refund
	err := r.pool.QueryRow(ctx, query, gatewayRefundID).Scan(
		&ref.ID, &ref.PaymentID, &ref.OrderID, &ref.Amount, &ref.Reason, &ref.Status, &ref.GatewayRefundID, &ref.IdempotencyKey, &ref.CreatedAt, &ref.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound(fmt.Sprintf("refund with gateway_refund_id %s not found", gatewayRefundID))
		}
		return nil, appErrors.Internal(err, "failed to query refund by gateway refund id")
	}
	return &ref, nil
}

func (r *pgPaymentRepository) UpdateRefundStatus(ctx context.Context, id string, status model.RefundStatus, gatewayRefundID string) (*model.Refund, error) {
	query := `
		UPDATE refunds
		SET status = $2,
		    gateway_refund_id = COALESCE(NULLIF($3, ''), gateway_refund_id),
		    updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING id, payment_id, order_id, amount, COALESCE(reason, ''), status, COALESCE(gateway_refund_id, ''), COALESCE(idempotency_key, ''), created_at, updated_at
	`
	var ref model.Refund
	err := r.pool.QueryRow(ctx, query, id, status, gatewayRefundID).Scan(
		&ref.ID, &ref.PaymentID, &ref.OrderID, &ref.Amount, &ref.Reason, &ref.Status, &ref.GatewayRefundID, &ref.IdempotencyKey, &ref.CreatedAt, &ref.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound(fmt.Sprintf("refund %s not found for status update", id))
		}
		return nil, appErrors.Internal(err, "failed to update refund status")
	}
	return &ref, nil
}
