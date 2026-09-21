package model

import (
	"time"
)

type StoreBankAccount struct {
	ID                string    `json:"id" db:"id"`
	StoreID           string    `json:"store_id" db:"store_id"`
	AccountHolderName string    `json:"account_holder_name" db:"account_holder_name"`
	AccountNumber     string    `json:"account_number" db:"account_number"`
	RoutingNumber     *string   `json:"routing_number,omitempty" db:"routing_number"`
	BankName          string    `json:"bank_name" db:"bank_name"`
	AccountType       string    `json:"account_type" db:"account_type"`
	BranchCode        *string   `json:"branch_code,omitempty" db:"branch_code"`
	IsPrimary         bool      `json:"is_primary" db:"is_primary"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

type Store struct {
	ID                 string            `json:"id" db:"id"`
	MerchantID         string            `json:"merchant_id" db:"merchant_id"`
	Name               string            `json:"name" db:"name"`
	Slug               string            `json:"slug" db:"slug"`
	Description        string            `json:"description" db:"description"`
	LogoURL            string            `json:"logo_url" db:"logo_url"`
	BannerURL          string            `json:"banner_url" db:"banner_url"`
	Address            string            `json:"address" db:"address"`
	ApprovalStatus     string            `json:"approval_status" db:"approval_status"`
	PublishStatus      bool              `json:"publish_status" db:"publish_status"`
	RejectionReason    *string           `json:"rejection_reason,omitempty" db:"rejection_reason"`
	KYCStatus          string            `json:"kyc_status" db:"kyc_status"`
	BankAccount        *StoreBankAccount `json:"bank_account,omitempty"`
	BankAccountDetails *string           `json:"bank_account_details,omitempty"`
	AvgStoreRating     float64           `json:"avg_store_rating" db:"avg_store_rating"`
	CreatedAt          time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at" db:"updated_at"`
}
