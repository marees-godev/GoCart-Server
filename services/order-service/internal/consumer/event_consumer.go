package consumer

import (
	"context"
	"fmt"
	"strings"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/model"
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

	case events.EventTypeOrderShipped:
		var payload events.OrderShippedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			return fmt.Errorf("failed to unmarshal OrderShippedEvent payload: %w", err)
		}
		orderID := strings.TrimSpace(payload.OrderID)
		if orderID == "" {
			orderID = strings.TrimSpace(envelope.AggregateID)
		}
		if orderID == "" {
			return nil
		}
		_, err := c.orderService.UpdateOrderStatus(ctx, orderID, model.OrderStatusShipped)
		return err

	case events.EventTypeOrderDelivered:
		var payload events.OrderDeliveredEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			return fmt.Errorf("failed to unmarshal OrderDeliveredEvent payload: %w", err)
		}
		orderID := strings.TrimSpace(payload.OrderID)
		if orderID == "" {
			orderID = strings.TrimSpace(envelope.AggregateID)
		}
		if orderID == "" {
			return nil
		}
		_, err := c.orderService.UpdateOrderStatus(ctx, orderID, model.OrderStatusDelivered)
		return err

	case events.EventTypeOrderCancelled:
		var payload events.OrderCancelledEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			return fmt.Errorf("failed to unmarshal OrderCancelledEvent payload: %w", err)
		}
		orderID := strings.TrimSpace(payload.OrderID)
		if orderID == "" {
			orderID = strings.TrimSpace(envelope.AggregateID)
		}
		if orderID == "" {
			return nil
		}
		reason := payload.Reason
		if strings.TrimSpace(reason) == "" {
			reason = "Order cancelled"
		}
		_, err := c.orderService.UpdateOrderStatus(ctx, orderID, model.OrderStatusCancelled, reason)
		return err

	case events.EventTypeDeliveryStatusChanged:
		var payload events.DeliveryStatusChangedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			return fmt.Errorf("failed to unmarshal DeliveryStatusChangedEvent payload: %w", err)
		}
		orderID := strings.TrimSpace(payload.OrderID)
		if orderID == "" {
			orderID = strings.TrimSpace(envelope.AggregateID)
		}
		if orderID == "" {
			return nil
		}
		switch strings.ToUpper(strings.TrimSpace(payload.NewStatus)) {
		case "PICKED_UP", "IN_TRANSIT":
			_, err := c.orderService.UpdateOrderStatus(ctx, orderID, model.OrderStatusShipped)
			return err
		case "OUT_FOR_DELIVERY":
			_, err := c.orderService.UpdateOrderStatus(ctx, orderID, model.OrderStatusOutForDelivery)
			return err
		case "DELIVERED":
			_, err := c.orderService.UpdateOrderStatus(ctx, orderID, model.OrderStatusDelivered)
			return err
		case "CANCELLED":
			_, err := c.orderService.UpdateOrderStatus(ctx, orderID, model.OrderStatusCancelled, "Delivery cancelled")
			return err
		default:
			return nil
		}

	case events.EventTypeDeliveryStatusUpdated:
		var payload events.DeliveryStatusUpdatedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			return fmt.Errorf("failed to unmarshal DeliveryStatusUpdatedEvent payload: %w", err)
		}
		orderID := strings.TrimSpace(payload.OrderID)
		if orderID == "" {
			orderID = strings.TrimSpace(envelope.AggregateID)
		}
		if orderID == "" {
			return nil
		}
		switch strings.ToUpper(strings.TrimSpace(payload.NewStatus)) {
		case "PICKED_UP", "IN_TRANSIT":
			_, err := c.orderService.UpdateOrderStatus(ctx, orderID, model.OrderStatusShipped)
			return err
		case "OUT_FOR_DELIVERY":
			_, err := c.orderService.UpdateOrderStatus(ctx, orderID, model.OrderStatusOutForDelivery)
			return err
		case "DELIVERED":
			_, err := c.orderService.UpdateOrderStatus(ctx, orderID, model.OrderStatusDelivered)
			return err
		case "CANCELLED":
			_, err := c.orderService.UpdateOrderStatus(ctx, orderID, model.OrderStatusCancelled, "Delivery cancelled")
			return err
		default:
			return nil
		}

	default:
		// Silently acknowledge unhandled events
		return nil
	}
}
