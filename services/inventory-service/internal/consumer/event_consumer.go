package consumer

import (
	"context"
	"fmt"
	"strings"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/service"
)

type InventoryEventConsumer struct {
	inventoryService service.InventoryService
}

func NewInventoryEventConsumer(inventoryService service.InventoryService) *InventoryEventConsumer {
	return &InventoryEventConsumer{
		inventoryService: inventoryService,
	}
}

// HandleEvent dispatches incoming domain events to corresponding inventory service operations.
func (c *InventoryEventConsumer) HandleEvent(ctx context.Context, envelope *events.EventEnvelope) error {
	if envelope == nil {
		return appErrors.BadRequest("event envelope is nil")
	}

	switch envelope.EventType {
	case events.EventTypeOrderCreated:
		var payload events.OrderCreatedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			return fmt.Errorf("failed to unmarshal OrderCreatedEvent payload: %w", err)
		}
		return c.handleOrderCreated(ctx, &payload)

	case events.EventTypePaymentFailed:
		var payload events.PaymentFailedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			return fmt.Errorf("failed to unmarshal PaymentFailedEvent payload: %w", err)
		}
		return c.handlePaymentFailed(ctx, &payload)

	case events.EventTypeOrderCancelled:
		var payload events.OrderCancelledEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			return fmt.Errorf("failed to unmarshal OrderCancelledEvent payload: %w", err)
		}
		return c.handleOrderCancelled(ctx, &payload)

	case events.EventTypeOrderConfirmed:
		var payload events.OrderConfirmedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			return fmt.Errorf("failed to unmarshal OrderConfirmedEvent payload: %w", err)
		}
		return c.handleOrderConfirmed(ctx, &payload)

	default:
		// Unknown or unhandled event type is ignored without retrying
		return nil
	}
}

func (c *InventoryEventConsumer) handleOrderCreated(ctx context.Context, payload *events.OrderCreatedEvent) error {
	if payload == nil || strings.TrimSpace(payload.OrderID) == "" {
		return appErrors.BadRequest("order_id is required in OrderCreated payload")
	}
	if len(payload.Items) == 0 {
		return appErrors.BadRequest("no items in OrderCreated payload")
	}

	items := make([]dto.ReserveItemInput, len(payload.Items))
	for i, item := range payload.Items {
		if strings.TrimSpace(item.ProductID) == "" || item.Quantity <= 0 {
			return appErrors.BadRequest(fmt.Sprintf("invalid item product_id %s or quantity %d", item.ProductID, item.Quantity))
		}
		items[i] = dto.ReserveItemInput{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		}
	}

	_, _, err := c.inventoryService.ReserveStock(ctx, dto.ReserveStockInput{
		OrderID:           payload.OrderID,
		Items:             items,
		ExpirationMinutes: 15,
	})
	if err != nil {
		// Insufficient stock or conflict error
		if appErrors.IsConflict(err) {
			return nil // Idempotent or business conflict handled cleanly
		}
		return err
	}
	return nil
}

func (c *InventoryEventConsumer) handlePaymentFailed(ctx context.Context, payload *events.PaymentFailedEvent) error {
	if payload == nil || strings.TrimSpace(payload.OrderID) == "" {
		return appErrors.BadRequest("order_id is required in PaymentFailed payload")
	}

	reason := payload.Reason
	if reason == "" {
		reason = "PAYMENT_FAILED"
	}

	err := c.inventoryService.ReleaseStock(ctx, dto.ReleaseStockInput{
		OrderID: payload.OrderID,
		Reason:  reason,
	})
	if err != nil {
		if appErrors.IsNotFound(err) {
			return nil // Idempotent: reservation already released or expired
		}
		return err
	}
	return nil
}

func (c *InventoryEventConsumer) handleOrderCancelled(ctx context.Context, payload *events.OrderCancelledEvent) error {
	if payload == nil || strings.TrimSpace(payload.OrderID) == "" {
		return appErrors.BadRequest("order_id is required in OrderCancelled payload")
	}

	reason := payload.Reason
	if reason == "" {
		reason = "ORDER_CANCELLED"
	}

	err := c.inventoryService.ReleaseStock(ctx, dto.ReleaseStockInput{
		OrderID: payload.OrderID,
		Reason:  reason,
	})
	if err != nil {
		if appErrors.IsNotFound(err) {
			return nil // Idempotent: reservation already released or expired
		}
		return err
	}
	return nil
}

func (c *InventoryEventConsumer) handleOrderConfirmed(ctx context.Context, payload *events.OrderConfirmedEvent) error {
	if payload == nil || strings.TrimSpace(payload.OrderID) == "" {
		return appErrors.BadRequest("order_id is required in OrderConfirmed payload")
	}

	err := c.inventoryService.CommitStock(ctx, dto.CommitStockInput{
		OrderID: payload.OrderID,
	})
	if err != nil {
		if appErrors.IsNotFound(err) || appErrors.IsConflict(err) {
			return nil // Idempotent: already committed or status handled
		}
		return err
	}
	return nil
}
