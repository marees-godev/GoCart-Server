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
	Status            model.DeliveryStatus `json:"status"`
	EstimatedDelivery *time.Time           `json:"estimated_delivery,omitempty"`
	ActualDelivery    *time.Time           `json:"actual_delivery,omitempty"`
	Location          string               `json:"location,omitempty"`
	Notes             string               `json:"notes,omitempty"`
}

type DeliveryStatusUpdateRequest struct {
	TrackingNumber    string               `json:"tracking_number,omitempty"`
	DeliveryID        string               `json:"delivery_id,omitempty"`
	OrderID           string               `json:"order_id,omitempty"`
	Status            model.DeliveryStatus `json:"status"`
	EstimatedDelivery *time.Time           `json:"estimated_delivery,omitempty"`
	ActualDelivery    *time.Time           `json:"actual_delivery,omitempty"`
	Location          string               `json:"location,omitempty"`
	Notes             string               `json:"notes,omitempty"`
}

type OrderConfirmedPayload struct {
	OrderID         string                 `json:"order_id"`
	DeliveryPartner string                 `json:"delivery_partner,omitempty"`
	ShippingAddress *model.ShippingAddress `json:"shipping_address,omitempty"`
	ConfirmedAt     time.Time              `json:"confirmed_at,omitempty"`
}

type WarehouseResponse struct {
	ID                string `json:"id"`
	DeliveryPartnerID string `json:"delivery_partner_id"`
	Name              string `json:"name"`
	Code              string `json:"code"`
	Street            string `json:"street"`
	City              string `json:"city"`
	State             string `json:"state"`
	PostalCode        string `json:"postal_code"`
	IsActive          bool   `json:"is_active"`
	CreatedAt         string `json:"created_at,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

func FromWarehouse(w *model.Warehouse) *WarehouseResponse {
	if w == nil {
		return nil
	}
	res := &WarehouseResponse{
		ID:                w.ID.String(),
		DeliveryPartnerID: w.DeliveryPartnerID.String(),
		Name:              w.Name,
		Code:              w.Code,
		Street:            w.Street,
		City:              w.City,
		State:             w.State,
		PostalCode:        w.PostalCode,
		IsActive:          w.IsActive,
	}
	if !w.CreatedAt.IsZero() {
		res.CreatedAt = w.CreatedAt.Format(time.RFC3339)
	}
	if !w.UpdatedAt.IsZero() {
		res.UpdatedAt = w.UpdatedAt.Format(time.RFC3339)
	}
	return res
}

type CreateWarehouseRequest struct {
	DeliveryPartnerID string `json:"delivery_partner_id"`
	Name              string `json:"name"`
	Code              string `json:"code"`
	Street            string `json:"street"`
	City              string `json:"city"`
	State             string `json:"state"`
	PostalCode        string `json:"postal_code"`
	IsActive          *bool  `json:"is_active,omitempty"`
}

type UpdateWarehouseRequest struct {
	DeliveryPartnerID string  `json:"delivery_partner_id"`
	Name              *string `json:"name,omitempty"`
	Code              *string `json:"code,omitempty"`
	Street            *string `json:"street,omitempty"`
	City              *string `json:"city,omitempty"`
	State             *string `json:"state,omitempty"`
	PostalCode        *string `json:"postal_code,omitempty"`
	IsActive          *bool   `json:"is_active,omitempty"`
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

type EstimatedDeliveryResponse struct {
	OrderID           string                  `json:"order_id"`
	DeliveryPartner   string                  `json:"delivery_partner"`
	DeliveryType      string                  `json:"delivery_type"`
	TransitEstimate   string                  `json:"transit_estimate"`
	MinDays           int                     `json:"min_days"`
	MaxDays           int                     `json:"max_days"`
	EstimatedDelivery time.Time               `json:"estimated_delivery"`
	OriginWarehouse   *model.Warehouse        `json:"origin_warehouse,omitempty"`
	DestinationCity   string                  `json:"destination_city"`
	DestinationState  string                  `json:"destination_state"`
	ShippingAddress   *model.ShippingAddress  `json:"shipping_address,omitempty"`
}



