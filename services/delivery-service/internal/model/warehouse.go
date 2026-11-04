package model

import (
	"time"

	"github.com/google/uuid"
)

type PartnerApprovalStatus string

const (
	ApprovalStatusPending  PartnerApprovalStatus = "PENDING"
	ApprovalStatusApproved PartnerApprovalStatus = "APPROVED"
	ApprovalStatusRejected PartnerApprovalStatus = "REJECTED"
	ApprovalStatusAppealed PartnerApprovalStatus = "APPEALED"
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

type DeliveryPartnerProfile struct {
	ID                 uuid.UUID             `json:"id" db:"id"`
	PartnerID          uuid.UUID             `json:"partner_id" db:"partner_id"`
	CompanyName        string                `json:"company_name" db:"company_name"`
	Email              string                `json:"email" db:"email"`
	ContactNumber      string                `json:"contact_number" db:"contact_number"`
	GSTIN              *string               `json:"gstin,omitempty" db:"gstin"`
	IsGSTINVerified    bool                  `json:"is_gstin_verified" db:"is_gstin_verified"`
	IsActive           bool                  `json:"is_active" db:"is_active"`
	ApprovalStatus     PartnerApprovalStatus `json:"approval_status" db:"approval_status"`
	RejectionReason    *string               `json:"rejection_reason,omitempty" db:"rejection_reason"`
	AppealReason       *string               `json:"appeal_reason,omitempty" db:"appeal_reason"`
	DeactivationReason *string               `json:"deactivation_reason,omitempty" db:"deactivation_reason"`
	DeletionReason     *string               `json:"deletion_reason,omitempty" db:"deletion_reason"`
	AppealedAt         *time.Time            `json:"appealed_at,omitempty" db:"appealed_at"`
	DeactivatedAt      *time.Time            `json:"deactivated_at,omitempty" db:"deactivated_at"`
	DeletedAt          *time.Time            `json:"deleted_at,omitempty" db:"deleted_at"`
	CreatedAt          time.Time             `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time             `json:"updated_at" db:"updated_at"`
}
