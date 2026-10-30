package consumer

import (
	"context"
	"fmt"
	"strings"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/service"
)

type OrderEventConsumer struct {
	orderService service.OrderService
}

func NewOrderEventConsumer(orderService service.OrderService) *OrderEventConsumer {
	return &OrderEventConsumer{
		orderService: orderService,
	}
}

// HandleEvent processes payment events to transition order lifecycle states idempotently.
func (c *OrderEventConsumer) HandleEvent(ctx context.Context, envelope *events.EventEnvelope) error {
	if envelope == nil {
		return appErrors.BadRequest("event envelope is nil")
	}

	// Propagate Correlation ID and Request ID into context
	if envelope.CorrelationID != "" {
		ctx = logger.WithCorrelationID(ctx, envelope.CorrelationID)
		ctx = logger.WithRequestID(ctx, envelope.CorrelationID)
	}

	switch envelope.EventType {
	case events.EventTypePaymentSuccessful:
		var payload events.PaymentSuccessfulEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			return fmt.Errorf("failed to unmarshal PaymentSuccessfulEvent payload: %w", err)
		}
		if strings.TrimSpace(payload.OrderID) == "" {
			payload.OrderID = envelope.AggregateID
		}
		_, err := c.orderService.HandlePaymentSuccessful(ctx, &payload, envelope.CorrelationID, envelope.TraceID)
		return err

	case events.EventTypePaymentFailed:
		var payload events.PaymentFailedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			return fmt.Errorf("failed to unmarshal PaymentFailedEvent payload: %w", err)
		}
		if strings.TrimSpace(payload.OrderID) == "" {
			payload.OrderID = envelope.AggregateID
		}
		_, err := c.orderService.HandlePaymentFailed(ctx, &payload, envelope.CorrelationID, envelope.TraceID)
		return err

	default:
		// Silently acknowledge unhandled events
		return nil
	}
}
