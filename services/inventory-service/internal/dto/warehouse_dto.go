package dto

import "time"

type CreateWarehouseRequest struct {
	DeliveryPartnerID string `json:"delivery_partner_id"`
	Name              string `json:"name"`
	Code              string `json:"code"`
	Street            string `json:"street"`
	City              string `json:"city"`
	State             string `json:"state"`
	PostalCode        string `json:"postal_code"`
	IsActive          *bool  `json:"is_active"`
}

type UpdateWarehouseRequest struct {
	DeliveryPartnerID string  `json:"delivery_partner_id"`
	ID                string  `json:"id"`
	Name              *string `json:"name"`
	Code              *string `json:"code"`
	Street            *string `json:"street"`
	City              *string `json:"city"`
	State             *string `json:"state"`
	PostalCode        *string `json:"postal_code"`
	IsActive          *bool   `json:"is_active"`
}

type WarehouseResponse struct {
	ID                string    `json:"id"`
	DeliveryPartnerID string    `json:"delivery_partner_id"`
	Name              string    `json:"name"`
	Code              string    `json:"code"`
	Street            string    `json:"street"`
	City              string    `json:"city"`
	State             string    `json:"state"`
	PostalCode        string    `json:"postal_code"`
	IsActive          bool      `json:"is_active"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
