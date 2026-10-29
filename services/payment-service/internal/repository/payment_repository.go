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
	UpdatePaymentStatus(ctx context.Context, id string, status model.PaymentStatus, gatewayTxID string, failureReason string, paymentMethod ...string) (*model.Payment, error)
	CreateRefund(ctx context.Context, refund *model.Refund) error
	GetRefundByID(ctx context.Context, id string) (*model.Refund, error)
	GetRefundByGatewayRefundID(ctx context.Context, gatewayRefundID string) (*model.Refund, error)
	GetRefundByIdempotencyKey(ctx context.Context, key string) (*model.Refund, error)
	GetRefundsByPaymentID(ctx context.Context, paymentID string) ([]*model.Refund, error)
	GetTotalRefundedAmount(ctx context.Context, paymentID string) (float64, error)
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

	if p.Status == model.PaymentStatusInitiated {
		initEvt := events.PaymentInitiatedEvent{
			PaymentID:     p.ID,
			OrderID:       p.OrderID,
			UserID:        p.UserID,
			Amount:        p.Amount,
			Currency:      p.Currency,
			PaymentMethod: p.PaymentMethod,
			InitiatedAt:   p.CreatedAt,
		}
		if env, envErr := events.NewEventEnvelopeWithAggregate(events.EventTypePaymentInitiated, "payment-service", p.ID, initEvt); envErr == nil {
			if payloadBytes, mErr := env.Marshal(); mErr == nil {
				outboxQuery := `
					INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, topic, status, retry_count, created_at)
					SELECT 'payment', $1::text, $2::text, $3::jsonb, $4::text, 'PENDING'::outbox_status, 0, NOW()
					WHERE NOT EXISTS (
						SELECT 1 FROM outbox_events WHERE aggregate_id = $1::text AND event_type = $2::text
					)
				`
				_, _ = r.pool.Exec(ctx, outboxQuery, p.ID, events.EventTypePaymentInitiated, payloadBytes, events.TopicPaymentInitiated)
			}
		}
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

func (r *pgPaymentRepository) UpdatePaymentStatus(ctx context.Context, id string, status model.PaymentStatus, gatewayTxID string, failureReason string, paymentMethod ...string) (*model.Payment, error) {
	pm := ""
	if len(paymentMethod) > 0 && paymentMethod[0] != "" {
		pm = model.NormalizePaymentMethod(paymentMethod[0])
	}

	targetID := id
	if _, err := uuid.Parse(targetID); err != nil {
		if existing, errLookup := r.GetPaymentByID(ctx, targetID); errLookup == nil && existing != nil {
			targetID = existing.ID
		} else if existingByGateway, errGateway := r.GetPaymentByGatewayTransactionID(ctx, targetID); errGateway == nil && existingByGateway != nil {
			targetID = existingByGateway.ID
		}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin transaction in UpdatePaymentStatus", "payment_id", targetID, "error", err)
		return nil, appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var oldStatus model.PaymentStatus
	lookupQuery := `SELECT status FROM payments WHERE id = $1::uuid FOR UPDATE`
	errLookup := tx.QueryRow(ctx, lookupQuery, targetID).Scan(&oldStatus)
	if errLookup != nil {
		if errors.Is(errLookup, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "payment not found for status update in repository", "payment_id", id)
			return nil, appErrors.NotFound(fmt.Sprintf("payment %s not found for status update", id))
		}
		slog.ErrorContext(ctx, "failed to query payment status in repository", "payment_id", id, "error", errLookup)
		return nil, appErrors.Internal(errLookup, "failed to query payment status")
	}

	if oldStatus != status && !model.CanTransitionPayment(oldStatus, status) {
		slog.WarnContext(ctx, "illegal payment status transition rejected in repository", "payment_id", targetID, "from", oldStatus, "to", status)
		return nil, appErrors.BadRequest(fmt.Sprintf("cannot transition payment from %s to %s", oldStatus, status))
	}

	query := `
		UPDATE payments
		SET status = $2,
		    gateway_transaction_id = COALESCE(NULLIF($3, ''), gateway_transaction_id),
		    failure_reason = COALESCE(NULLIF($4, ''), failure_reason),
		    payment_method = COALESCE(NULLIF($5, ''), payment_method),
		    payment_method_id = COALESCE(
		        (SELECT id FROM payment_methods WHERE code = $5::text LIMIT 1),
		        (SELECT id FROM payment_methods WHERE code = UPPER($5::text) LIMIT 1),
		        payment_method_id
		    ),
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
	err = tx.QueryRow(ctx, query, targetID, status, gatewayTxID, failureReason, pm).Scan(
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

	// Insert Outbox Event only on genuine status transitions to prevent duplicate events
	if oldStatus != status {
		var eventType, topic string
		var eventPayload interface{}

		switch status {
		case model.PaymentStatusSuccess:
			eventType = events.EventTypePaymentSuccessful
			topic = events.TopicPaymentSuccessful
			eventPayload = events.PaymentSuccessfulEvent{
				PaymentID:     p.ID,
				OrderID:       p.OrderID,
				UserID:        p.UserID,
				Amount:        p.Amount,
				Currency:      p.Currency,
				PaymentMethod: p.PaymentMethod,
				PaidAt:        p.UpdatedAt,
			}
		case model.PaymentStatusFailed:
			eventType = events.EventTypePaymentFailed
			topic = events.TopicPaymentFailed
			eventPayload = events.PaymentFailedEvent{
				PaymentID: p.ID,
				OrderID:   p.OrderID,
				UserID:    p.UserID,
				Amount:    p.Amount,
				Reason:    p.FailureReason,
				FailedAt:  p.UpdatedAt,
			}
		case model.PaymentStatusPending:
			eventType = events.EventTypePaymentPending
			topic = events.TopicPaymentPending
			eventPayload = events.PaymentPendingEvent{
				PaymentID:            p.ID,
				OrderID:              p.OrderID,
				UserID:               p.UserID,
				Amount:               p.Amount,
				Currency:             p.Currency,
				GatewayTransactionID: p.GatewayTransactionID,
				PendingAt:            p.UpdatedAt,
			}
		case model.PaymentStatusCancelled:
			eventType = events.EventTypePaymentCancelled
			topic = events.TopicPaymentCancelled
			eventPayload = events.PaymentCancelledEvent{
				PaymentID:   p.ID,
				OrderID:     p.OrderID,
				UserID:      p.UserID,
				Amount:      p.Amount,
				Currency:    p.Currency,
				Reason:      p.FailureReason,
				CancelledAt: p.UpdatedAt,
			}
		}

		if eventType != "" && eventPayload != nil {
			env, envErr := events.NewEventEnvelopeWithAggregate(eventType, "payment-service", p.ID, eventPayload)
			if envErr != nil {
				slog.ErrorContext(ctx, "failed to create outbox event envelope in repository", "payment_id", p.ID, "error", envErr)
				return nil, appErrors.Internal(envErr, "failed to create outbox event envelope")
			}
			payloadBytes, mErr := env.Marshal()
			if mErr != nil {
				slog.ErrorContext(ctx, "failed to marshal outbox event in repository", "payment_id", p.ID, "error", mErr)
				return nil, appErrors.Internal(mErr, "failed to marshal outbox event")
			}

			outboxQuery := `
				INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, topic, status, retry_count, created_at)
				SELECT 'payment', $1::text, $2::text, $3::jsonb, $4::text, 'PENDING'::outbox_status, 0, NOW()
				WHERE NOT EXISTS (
					SELECT 1 FROM outbox_events WHERE aggregate_id = $1::text AND event_type = $2::text
				)
			`
			if _, execErr := tx.Exec(ctx, outboxQuery, p.ID, eventType, payloadBytes, topic); execErr != nil {
				slog.ErrorContext(ctx, "failed to insert outbox event in repository", "payment_id", p.ID, "error", execErr)
				return nil, appErrors.Internal(execErr, "failed to record outbox event")
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit transaction in UpdatePaymentStatus", "payment_id", p.ID, "error", err)
		return nil, appErrors.Internal(err, "failed to commit transaction")
	}

	slog.InfoContext(ctx, "payment status updated in repository", "payment_id", p.ID, "old_status", oldStatus, "new_status", p.Status)
	return &p, nil
}


func (r *pgPaymentRepository) CreateRefund(ctx context.Context, ref *model.Refund) error {
	if ref == nil {
		slog.WarnContext(ctx, "refund model is nil in CreateRefund")
		return appErrors.BadRequest("refund model cannot be nil")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin transaction in CreateRefund", "payment_id", ref.PaymentID, "error", err)
		return appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

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

	err = tx.QueryRow(ctx, query,
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

	// Insert Outbox Event for RefundInitiated
	env, envErr := events.NewEventEnvelopeWithAggregate(events.EventTypeRefundInitiated, "payment-service", ref.ID, events.RefundInitiatedEvent{
		RefundID:    ref.ID,
		PaymentID:   ref.PaymentID,
		OrderID:     ref.OrderID,
		Amount:      ref.Amount,
		Reason:      ref.Reason,
		InitiatedAt: ref.CreatedAt,
	})
	if envErr != nil {
		slog.ErrorContext(ctx, "failed to create outbox event envelope for RefundInitiated", "refund_id", ref.ID, "error", envErr)
		return appErrors.Internal(envErr, "failed to create outbox event envelope")
	}
	payloadBytes, mErr := env.Marshal()
	if mErr != nil {
		slog.ErrorContext(ctx, "failed to marshal outbox event for RefundInitiated", "refund_id", ref.ID, "error", mErr)
		return appErrors.Internal(mErr, "failed to marshal outbox event")
	}

	outboxQuery := `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, topic, status, retry_count, created_at)
		SELECT 'refund', $1::text, $2::text, $3::jsonb, $4::text, 'PENDING'::outbox_status, 0, NOW()
		WHERE NOT EXISTS (
			SELECT 1 FROM outbox_events WHERE aggregate_id = $1::text AND event_type = $2::text
		)
	`
	if _, execErr := tx.Exec(ctx, outboxQuery, ref.ID, events.EventTypeRefundInitiated, payloadBytes, events.TopicRefundInitiated); execErr != nil {
		slog.ErrorContext(ctx, "failed to insert outbox event in repository", "refund_id", ref.ID, "error", execErr)
		return appErrors.Internal(execErr, "failed to record outbox event")
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit transaction in CreateRefund", "refund_id", ref.ID, "error", err)
		return appErrors.Internal(err, "failed to commit transaction")
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
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin transaction in UpdateRefundStatus", "refund_id", id, "error", err)
		return nil, appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var oldStatus model.RefundStatus
	lookupQuery := `SELECT status FROM refunds WHERE id = $1::uuid FOR UPDATE`
	errLookup := tx.QueryRow(ctx, lookupQuery, id).Scan(&oldStatus)
	if errLookup != nil {
		if errors.Is(errLookup, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "refund not found for status update in repository", "refund_id", id)
			return nil, appErrors.NotFound(fmt.Sprintf("refund %s not found for status update", id))
		}
		slog.ErrorContext(ctx, "failed to query refund status in repository", "refund_id", id, "error", errLookup)
		return nil, appErrors.Internal(errLookup, "failed to query refund status")
	}

	query := `
		UPDATE refunds
		SET status = $2,
		    gateway_refund_id = COALESCE(NULLIF($3, ''), gateway_refund_id),
		    updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING id, payment_id, order_id, amount, COALESCE(reason, ''), status, COALESCE(gateway_refund_id, ''), COALESCE(idempotency_key, ''), created_at, updated_at
	`
	var ref model.Refund
	err = tx.QueryRow(ctx, query, id, status, gatewayRefundID).Scan(
		&ref.ID, &ref.PaymentID, &ref.OrderID, &ref.Amount, &ref.Reason, &ref.Status, &ref.GatewayRefundID, &ref.IdempotencyKey, &ref.CreatedAt, &ref.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound(fmt.Sprintf("refund %s not found for status update", id))
		}
		return nil, appErrors.Internal(err, "failed to update refund status")
	}

	if oldStatus != status {
		var eventType, topic string
		var eventPayload interface{}

		switch status {
		case model.RefundStatusSuccess:
			eventType = events.EventTypeRefundCompleted
			topic = events.TopicRefundCompleted
			eventPayload = events.RefundCompletedEvent{
				RefundID:        ref.ID,
				PaymentID:       ref.PaymentID,
				OrderID:         ref.OrderID,
				Amount:          ref.Amount,
				GatewayRefundID: ref.GatewayRefundID,
				CompletedAt:     ref.UpdatedAt,
			}
		case model.RefundStatusFailed:
			eventType = events.EventTypeRefundFailed
			topic = events.TopicRefundFailed
			eventPayload = events.RefundFailedEvent{
				RefundID:  ref.ID,
				PaymentID: ref.PaymentID,
				OrderID:   ref.OrderID,
				Amount:    ref.Amount,
				Reason:    "refund failed at gateway",
				FailedAt:  ref.UpdatedAt,
			}
		}

		if eventType != "" && eventPayload != nil {
			env, envErr := events.NewEventEnvelopeWithAggregate(eventType, "payment-service", ref.ID, eventPayload)
			if envErr != nil {
				slog.ErrorContext(ctx, "failed to create outbox event envelope in UpdateRefundStatus", "refund_id", ref.ID, "error", envErr)
				return nil, appErrors.Internal(envErr, "failed to create outbox event envelope")
			}
			payloadBytes, mErr := env.Marshal()
			if mErr != nil {
				slog.ErrorContext(ctx, "failed to marshal outbox event in UpdateRefundStatus", "refund_id", ref.ID, "error", mErr)
				return nil, appErrors.Internal(mErr, "failed to marshal outbox event")
			}

			outboxQuery := `
				INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, topic, status, retry_count, created_at)
				SELECT 'refund', $1::text, $2::text, $3::jsonb, $4::text, 'PENDING'::outbox_status, 0, NOW()
				WHERE NOT EXISTS (
					SELECT 1 FROM outbox_events WHERE aggregate_id = $1::text AND event_type = $2::text
				)
			`
			if _, execErr := tx.Exec(ctx, outboxQuery, ref.ID, eventType, payloadBytes, topic); execErr != nil {
				slog.ErrorContext(ctx, "failed to insert outbox event in UpdateRefundStatus", "refund_id", ref.ID, "error", execErr)
				return nil, appErrors.Internal(execErr, "failed to record outbox event")
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit transaction in UpdateRefundStatus", "refund_id", ref.ID, "error", err)
		return nil, appErrors.Internal(err, "failed to commit transaction")
	}

	slog.InfoContext(ctx, "refund status updated in repository", "refund_id", ref.ID, "old_status", oldStatus, "new_status", ref.Status)
	return &ref, nil
}

func (r *pgPaymentRepository) GetRefundByIdempotencyKey(ctx context.Context, key string) (*model.Refund, error) {
	if key == "" {
		return nil, appErrors.BadRequest("idempotency key cannot be empty")
	}
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
		WHERE idempotency_key = $1
	`
	var ref model.Refund
	err := r.pool.QueryRow(ctx, query, key).Scan(
		&ref.ID, &ref.PaymentID, &ref.OrderID, &ref.Amount, &ref.Reason, &ref.Status, &ref.GatewayRefundID, &ref.IdempotencyKey, &ref.CreatedAt, &ref.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("refund not found by idempotency key")
		}
		return nil, appErrors.Internal(err, "failed to query refund by idempotency key")
	}
	return &ref, nil
}

func (r *pgPaymentRepository) GetRefundsByPaymentID(ctx context.Context, paymentID string) ([]*model.Refund, error) {
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
		WHERE payment_id = $1::uuid
		ORDER BY created_at ASC
	`
	rows, err := r.pool.Query(ctx, query, paymentID)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to query refunds by payment id")
	}
	defer rows.Close()

	var refunds []*model.Refund
	for rows.Next() {
		var ref model.Refund
		if err := rows.Scan(
			&ref.ID, &ref.PaymentID, &ref.OrderID, &ref.Amount, &ref.Reason, &ref.Status, &ref.GatewayRefundID, &ref.IdempotencyKey, &ref.CreatedAt, &ref.UpdatedAt,
		); err != nil {
			return nil, appErrors.Internal(err, "failed to scan refund row")
		}
		refunds = append(refunds, &ref)
	}
	if err := rows.Err(); err != nil {
		return nil, appErrors.Internal(err, "error reading refund rows")
	}
	return refunds, nil
}

func (r *pgPaymentRepository) GetTotalRefundedAmount(ctx context.Context, paymentID string) (float64, error) {
	query := `
		SELECT COALESCE(SUM(amount), 0.00)
		FROM refunds
		WHERE payment_id = $1::uuid AND status = 'SUCCESS'
	`
	var total float64
	err := r.pool.QueryRow(ctx, query, paymentID).Scan(&total)
	if err != nil {
		return 0, appErrors.Internal(err, "failed to calculate total refunded amount")
	}
	return total, nil
}

