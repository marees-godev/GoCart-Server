package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/service"
)

type PaymentEventConsumer struct {
	paymentService service.PaymentService
}

func NewPaymentEventConsumer(paymentService service.PaymentService) *PaymentEventConsumer {
	return &PaymentEventConsumer{
		paymentService: paymentService,
	}
}

// HandleEvent dispatches incoming domain events to corresponding payment service operations.
func (c *PaymentEventConsumer) HandleEvent(ctx context.Context, envelope *events.EventEnvelope) error {
	if envelope == nil {
		return appErrors.BadRequest("event envelope is nil")
	}

	// Propagate Correlation ID and Request ID into context
	if envelope.CorrelationID != "" {
		ctx = logger.WithCorrelationID(ctx, envelope.CorrelationID)
		ctx = logger.WithRequestID(ctx, envelope.CorrelationID)
	}

	switch envelope.EventType {
	case events.EventTypeOrderCreated:
		var payload events.OrderCreatedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			return fmt.Errorf("failed to unmarshal OrderCreatedEvent payload: %w", err)
		}
		if strings.TrimSpace(payload.OrderID) == "" {
			payload.OrderID = envelope.AggregateID
		}
		_, err := c.paymentService.HandleOrderCreated(ctx, &payload, envelope.CorrelationID, envelope.TraceID)
		return err

	case events.EventTypeReturnApproved:
		return c.handleReturnApproved(ctx, envelope)

	default:
		// Silently acknowledge unhandled events
		return nil
	}
}

func (c *PaymentEventConsumer) handleReturnApproved(ctx context.Context, envelope *events.EventEnvelope) error {
	var evt events.ReturnApprovedEvent
	if err := envelope.UnmarshalData(&evt); err != nil {
		slog.ErrorContext(ctx, "failed to unmarshal ReturnApproved event payload", "error", err)
		return fmt.Errorf("failed to unmarshal ReturnApproved payload: %w", err)
	}

	returnID := strings.TrimSpace(evt.ReturnID)
	if returnID == "" {
		returnID = strings.TrimSpace(envelope.AggregateID)
	}
	orderID := strings.TrimSpace(evt.OrderID)

	if returnID == "" || orderID == "" {
		slog.WarnContext(ctx, "missing return_id or order_id in ReturnApproved event",
			"return_id", returnID,
			"order_id", orderID,
		)
		return nil // Acknowledge bad message to avoid poisoning the queue
	}

	dto := &service.ProcessReturnRefundDTO{
		ReturnID:     returnID,
		OrderID:      orderID,
		OrderItemID:  evt.OrderItemID,
		UserID:       evt.UserID,
		Type:         evt.Type,
		RefundAmount: evt.RefundAmount,
		Reason:       fmt.Sprintf("Refund for approved return %s", returnID),
	}

	_, err := c.paymentService.ProcessReturnRefund(ctx, dto)
	if err != nil {
		var appErr *appErrors.AppError
		if errors.As(err, &appErr) && (appErr.Code == appErrors.CodeBadRequest || appErr.Code == appErrors.CodeConflict || appErr.Code == appErrors.CodeNotFound) {
			slog.WarnContext(ctx, "non-retryable error in handleReturnApproved",
				"return_id", returnID,
				"order_id", orderID,
				"error", err,
			)
			return nil
		}
		slog.ErrorContext(ctx, "failed to process return refund in event consumer",
			"return_id", returnID,
			"order_id", orderID,
			"error", err,
		)
		return err
	}

	slog.InfoContext(ctx, "successfully processed return refund from ReturnApproved event",
		"return_id", returnID,
		"order_id", orderID,
	)
	return nil
}
