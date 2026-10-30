package consumer

import (
	"context"
	"fmt"
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

	default:
		// Silently acknowledge unhandled events
		return nil
	}
}
