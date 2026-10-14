package dto

import (
	"time"

	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
)

type TrackingResponse struct {
	OrderID         string                `json:"order_id"`
	TrackingNumber  string                `json:"tracking_number"`
	DeliveryPartner string                `json:"delivery_partner"`
	Status          model.DeliveryStatus  `json:"status"`
	ShippingAddress model.ShippingAddress `json:"shipping_address"`
	UpdatedAt       string                `json:"updated_at"`
}

func FromDelivery(d *model.Delivery) *TrackingResponse {
	if d == nil {
		return nil
	}
	return &TrackingResponse{
		OrderID:         d.OrderID,
		TrackingNumber:  d.TrackingNumber,
		DeliveryPartner: d.DeliveryPartner,
		Status:          d.Status,
		ShippingAddress: d.ShippingAddress,
		UpdatedAt:       d.UpdatedAt.Format(time.RFC3339),
	}
}

type UpdateStatusRequest struct {
	Status model.DeliveryStatus `json:"status"`
}

type OrderConfirmedPayload struct {
	OrderID         string                 `json:"order_id"`
	DeliveryPartner string                 `json:"delivery_partner,omitempty"`
	ShippingAddress *model.ShippingAddress `json:"shipping_address,omitempty"`
	ConfirmedAt     time.Time              `json:"confirmed_at,omitempty"`
}
