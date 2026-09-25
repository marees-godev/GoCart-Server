package model

import (
	"time"
)

const (
	StoreStatusDraft           = "DRAFT"
	StoreStatusPendingApproval = "PENDING_APPROVAL"
	StoreStatusApproved        = "APPROVED"
	StoreStatusRejected        = "REJECTED"
	StoreStatusSuspended       = "SUSPENDED"
	StoreStatusClosed          = "CLOSED"
)

const (
	KYCStatusNotSubmitted = "NOT_SUBMITTED"
	KYCStatusPending      = "PENDING"
	KYCStatusVerified     = "VERIFIED"
	KYCStatusRejected     = "REJECTED"
)

const (
	AppealStatusPending  = "PENDING"
	AppealStatusApproved = "APPROVED"
	AppealStatusRejected = "REJECTED"
)

type StoreAppeal struct {
	ID           string     `json:"id" db:"id"`
	StoreID      string     `json:"store_id" db:"store_id"`
	MerchantID   string     `json:"merchant_id" db:"merchant_id"`
	Reason       string     `json:"reason" db:"reason"`
	Status       string     `json:"status" db:"status"`
	AdminComment *string    `json:"admin_comment,omitempty" db:"admin_comment"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty" db:"reviewed_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

type StoreBankAccount struct {
	ID                string    `json:"id" db:"id"`
	StoreID           string    `json:"store_id" db:"store_id"`
	AccountHolderName string    `json:"account_holder_name" db:"account_holder_name"`
	BankName          string    `json:"bank_name" db:"bank_name"`
	AccountNumber     string    `json:"account_number" db:"account_number"`
	IfscCode          *string   `json:"ifsc_code,omitempty" db:"ifsc_code"`
	GSTIN             *string   `json:"gstin,omitempty" db:"gstin"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

type Store struct {
	ID                   string            `json:"id" db:"id"`
	MerchantID           string            `json:"merchant_id" db:"merchant_id"`
	Name                 string            `json:"name" db:"name"`
	Slug                 string            `json:"slug" db:"slug"`
	BusinessEmail        string            `json:"business_email" db:"business_email"`
	BusinessPhone        string            `json:"business_phone" db:"business_phone"`
	Description          string            `json:"description" db:"description"`
	LogoURL              string            `json:"logo_url" db:"logo_url"`
	Address              string            `json:"address" db:"address"`
	IsVacationMode       bool              `json:"is_vacation_mode" db:"is_vacation_mode"`
	ApprovalStatus       string            `json:"approval_status" db:"approval_status"`
	RejectionReason      *string           `json:"rejection_reason,omitempty" db:"rejection_reason"`
	IsPublished          bool              `json:"is_published" db:"is_published"`
	KYCStatus            string            `json:"kyc_status" db:"kyc_status"`
	GSTIN                *string           `json:"gstin,omitempty" db:"gstin"`
	BankAccount          *StoreBankAccount `json:"bank_account,omitempty"`
	BankAccountDetails   *string           `json:"bank_account_details,omitempty"`
	AvgStoreRating       float64           `json:"avg_store_rating" db:"avg_store_rating"`
	CreatedAt            time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at" db:"updated_at"`
}
