package model

import (
	"time"

	"github.com/google/uuid"
)

type MerchantStatus string

const (
	MerchantStatusPending   MerchantStatus = "PENDING"
	MerchantStatusApproved  MerchantStatus = "APPROVED"
	MerchantStatusRejected  MerchantStatus = "REJECTED"
	MerchantStatusSuspended MerchantStatus = "SUSPENDED"

	// Compatibility aliases mapping to lifecycle states
	MerchantStatusActive        MerchantStatus = "ACTIVE"
	MerchantStatusInactive      MerchantStatus = "INACTIVE"
	MerchantStatusPendingReview MerchantStatus = "PENDING_REVIEW"
	MerchantStatusTerminated    MerchantStatus = "TERMINATED"
)

type MerchantLifecycleStatus string

const (
	MerchantLifecycleStatusActive        MerchantLifecycleStatus = "ACTIVE"
	MerchantLifecycleStatusInactive      MerchantLifecycleStatus = "INACTIVE"
	MerchantLifecycleStatusPendingReview MerchantLifecycleStatus = "PENDING_REVIEW"
	MerchantLifecycleStatusSuspended     MerchantLifecycleStatus = "SUSPENDED"
	MerchantLifecycleStatusTerminated    MerchantLifecycleStatus = "TERMINATED"
)

type MerchantAppealStatus string

const (
	MerchantAppealStatusPending  MerchantAppealStatus = "PENDING"
	MerchantAppealStatusApproved MerchantAppealStatus = "APPROVED"
	MerchantAppealStatusRejected MerchantAppealStatus = "REJECTED"
)

type MerchantAppeal struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	MerchantID   uuid.UUID  `json:"merchant_id" db:"merchant_id"`
	Reason       string     `json:"reason" db:"reason"`
	Status       string     `json:"status" db:"status"`
	AdminComment *string    `json:"admin_comment,omitempty" db:"admin_comment"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty" db:"reviewed_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

type Merchant struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	BusinessName    string     `json:"business_name" db:"business_name"`
	FirstName       string     `json:"first_name" db:"first_name"`
	LastName        string     `json:"last_name" db:"last_name"`
	BusinessEmail   string     `json:"business_email" db:"business_email"`
	BusinessPhone   string     `json:"business_phone" db:"business_phone"`
	PanCardNumber   string     `json:"pan_card_number" db:"pan_card_number"`
	Status          string     `json:"status" db:"status"`
	LifecycleStatus *string    `json:"lifecycle_status,omitempty" db:"lifecycle_status"`
	RejectionReason string     `json:"rejection_reason" db:"rejection_reason"`
	Version         int        `json:"version" db:"version"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}
