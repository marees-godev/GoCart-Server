package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/service"
)

type DeliveryEventConsumer struct {
	deliveryService service.DeliveryService
}

func NewDeliveryEventConsumer(deliveryService service.DeliveryService) *DeliveryEventConsumer {
	return &DeliveryEventConsumer{
		deliveryService: deliveryService,
	}
}

type orderConfirmedRawPayload struct {
	OrderID         string          `json:"order_id"`
	DeliveryPartner string          `json:"delivery_partner"`
	ShippingAddress json.RawMessage `json:"shipping_address"`
	ContactNumber   string          `json:"contact_number,omitempty"`
	Phone           string          `json:"phone,omitempty"`
	ConfirmedAt     time.Time       `json:"confirmed_at"`
}

func (c *DeliveryEventConsumer) HandleEvent(ctx context.Context, envelope *events.EventEnvelope) error {
	if envelope == nil {
		return appErrors.BadRequest("event envelope is nil")
	}

	switch envelope.EventType {
	case events.EventTypeOrderConfirmed:
		return c.handleOrderConfirmed(ctx, envelope)
	default:
		return nil
	}
}

func (c *DeliveryEventConsumer) handleOrderConfirmed(ctx context.Context, envelope *events.EventEnvelope) error {
	var raw orderConfirmedRawPayload
	if err := json.Unmarshal(envelope.Data, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal OrderConfirmed payload: %w", err)
	}

	orderID := strings.TrimSpace(raw.OrderID)
	if orderID == "" {
		orderID = strings.TrimSpace(envelope.AggregateID)
	}
	if orderID == "" {
		return appErrors.BadRequest("order_id is required in OrderConfirmed payload")
	}

	var addr model.ShippingAddress
	if len(raw.ShippingAddress) > 0 {
		if err := json.Unmarshal(raw.ShippingAddress, &addr); err != nil || (addr.Street == "" && addr.City == "") {
			var addrStr string
			if errStr := json.Unmarshal(raw.ShippingAddress, &addrStr); errStr == nil && addrStr != "" {
				addr = model.ShippingAddress{
					Street: addrStr,
				}
			}
		}
	}

	if addr.ContactNumber == "" {
		if raw.ContactNumber != "" {
			addr.ContactNumber = raw.ContactNumber
		} else if raw.Phone != "" {
			addr.ContactNumber = raw.Phone
		}
	}

	if strings.TrimSpace(addr.ContactNumber) == "" {
		return appErrors.BadRequest("contact_number is required in shipping address payload")
	}

	payload := &dto.OrderConfirmedPayload{
		OrderID:         orderID,
		DeliveryPartner: raw.DeliveryPartner,
		ShippingAddress: &addr,
		ConfirmedAt:     raw.ConfirmedAt,
	}

	_, err := c.deliveryService.HandleOrderConfirmed(ctx, payload)
	return err
}
