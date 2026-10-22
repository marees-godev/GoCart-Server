package dto

import (
	"time"

	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
)

type TrackingResponse struct {
	OrderID           string                `json:"order_id"`
	TrackingNumber    string                `json:"tracking_number"`
	DeliveryPartner   string                `json:"delivery_partner"`
	Status            model.DeliveryStatus  `json:"status"`
	ShippingAddress   model.ShippingAddress `json:"shipping_address"`
	EstimatedDelivery *string               `json:"estimated_delivery,omitempty"`
	ActualDelivery    *string               `json:"actual_delivery,omitempty"`
	UpdatedAt         string                `json:"updated_at"`
}

func FromDelivery(d *model.Delivery) *TrackingResponse {
	if d == nil {
		return nil
	}
	res := &TrackingResponse{
		OrderID:         d.OrderID,
		TrackingNumber:  d.TrackingNumber,
		DeliveryPartner: d.DeliveryPartner,
		Status:          d.Status,
		ShippingAddress: d.ShippingAddress,
		UpdatedAt:       d.UpdatedAt.Format(time.RFC3339),
	}
	if d.EstimatedDelivery != nil {
		est := d.EstimatedDelivery.Format(time.RFC3339)
		res.EstimatedDelivery = &est
	}
	if d.ActualDelivery != nil {
		act := d.ActualDelivery.Format(time.RFC3339)
		res.ActualDelivery = &act
	}
	return res
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

type WarehouseResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Code       string `json:"code"`
	Street     string `json:"street"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	IsActive   bool   `json:"is_active"`
}

func FromWarehouse(w *model.Warehouse) *WarehouseResponse {
	if w == nil {
		return nil
	}
	return &WarehouseResponse{
		ID:         w.ID.String(),
		Name:       w.Name,
		Code:       w.Code,
		Street:     w.Street,
		City:       w.City,
		State:      w.State,
		PostalCode: w.PostalCode,
		IsActive:   w.IsActive,
	}
}

type AssignWarehousesRequest struct {
	WarehouseIDs []string `json:"warehouse_ids"`
}

type PartnerProfileResponse struct {
	ID            string `json:"id"`
	PartnerID     string `json:"partner_id"`
	CompanyName   string `json:"company_name"`
	Email         string `json:"email"`
	ContactNumber string `json:"contact_number"`
	IsActive      bool   `json:"is_active"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

func FromPartnerProfile(p *model.DeliveryPartnerProfile) *PartnerProfileResponse {
	if p == nil {
		return nil
	}
	return &PartnerProfileResponse{
		ID:            p.ID.String(),
		PartnerID:     p.PartnerID.String(),
		CompanyName:   p.CompanyName,
		Email:         p.Email,
		ContactNumber: p.ContactNumber,
		IsActive:      p.IsActive,
		CreatedAt:     p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     p.UpdatedAt.Format(time.RFC3339),
	}
}

type UpdatePartnerProfileRequest struct {
	CompanyName   *string `json:"company_name,omitempty"`
	Email         *string `json:"email,omitempty"`
	ContactNumber *string `json:"contact_number,omitempty"`
}


