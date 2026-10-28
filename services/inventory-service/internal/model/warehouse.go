package model

import (
	"time"

	"github.com/google/uuid"
)

type Warehouse struct {
	ID                uuid.UUID `json:"id" db:"id"`
	DeliveryPartnerID uuid.UUID `json:"delivery_partner_id" db:"delivery_partner_id"`
	Name              string    `json:"name" db:"name"`
	Code              string    `json:"code" db:"code"`
	Street            string    `json:"street" db:"street"`
	City              string    `json:"city" db:"city"`
	State             string    `json:"state" db:"state"`
	PostalCode        string    `json:"postal_code" db:"postal_code"`
	IsActive          bool      `json:"is_active" db:"is_active"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}
