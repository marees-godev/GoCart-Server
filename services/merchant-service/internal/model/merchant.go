package model

import (
	"time"

	"github.com/google/uuid"
)

type MerchantStatus string

const (
	MerchantStatusPending  MerchantStatus = "PENDING"
	MerchantStatusApproved MerchantStatus = "APPROVED"
	MerchantStatusRejected MerchantStatus = "REJECTED"
)

type Merchant struct {
	ID              uuid.UUID `json:"id" db:"id"`
	UserID          uuid.UUID `json:"user_id" db:"user_id"`
	BusinessName    string    `json:"business_name" db:"business_name"`
	FirstName       string    `json:"first_name" db:"first_name"`
	LastName        string    `json:"last_name" db:"last_name"`
	BusinessEmail   string    `json:"business_email" db:"business_email"`
	BusinessPhone   string    `json:"business_phone" db:"business_phone"`
	TaxID           string    `json:"tax_id" db:"tax_id"`
	Status          string    `json:"status" db:"status"`
	RejectionReason string    `json:"rejection_reason" db:"rejection_reason"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}
