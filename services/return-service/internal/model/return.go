package model

import "time"

type ReturnType string

const (
	ReturnTypeReturn      ReturnType = "RETURN"
	ReturnTypeReplacement ReturnType = "REPLACEMENT"
)

func (t ReturnType) IsValid() bool {
	switch t {
	case ReturnTypeReturn, ReturnTypeReplacement:
		return true
	default:
		return false
	}
}

type ReturnStatus string

const (
	ReturnStatusRequested            ReturnStatus = "RETURN_REQUESTED"
	ReturnStatusReplacementRequested ReturnStatus = "REPLACEMENT_REQUESTED"
	ReturnStatusApproved             ReturnStatus = "APPROVED"
	ReturnStatusRejected             ReturnStatus = "REJECTED"
	ReturnStatusVerified             ReturnStatus = "VERIFIED"
	ReturnStatusItemReceived         ReturnStatus = "ITEM_RECEIVED"
	ReturnStatusRefundInitiated      ReturnStatus = "REFUND_INITIATED"
	ReturnStatusReplacementInitiated ReturnStatus = "REPLACEMENT_INITIATED"
	ReturnStatusCompleted            ReturnStatus = "COMPLETED"
	ReturnStatusCancelled            ReturnStatus = "CANCELLED"
)

func (s ReturnStatus) IsValid() bool {
	switch s {
	case ReturnStatusRequested, ReturnStatusReplacementRequested, ReturnStatusApproved, ReturnStatusRejected,
		ReturnStatusVerified, ReturnStatusItemReceived, ReturnStatusRefundInitiated,
		ReturnStatusReplacementInitiated, ReturnStatusCompleted, ReturnStatusCancelled:
		return true
	default:
		return false
	}
}

type VerificationCondition string

const (
	ConditionGood      VerificationCondition = "GOOD"
	ConditionDefective VerificationCondition = "DEFECTIVE"
	ConditionDamaged   VerificationCondition = "DAMAGED"
	ConditionWrongItem VerificationCondition = "WRONG_ITEM"
	ConditionOpenBox   VerificationCondition = "OPEN_BOX"
)

func (c VerificationCondition) IsValid() bool {
	switch c {
	case ConditionGood, ConditionDefective, ConditionDamaged, ConditionWrongItem, ConditionOpenBox:
		return true
	default:
		return false
	}
}

type ProductVerification struct {
	ID          string                `json:"id" db:"id"`
	ReturnID    string                `json:"return_id" db:"return_id"`
	VerifiedBy  string                `json:"verified_by" db:"verified_by"`
	Condition   VerificationCondition `json:"condition" db:"condition"`
	IsApproved  bool                  `json:"is_approved" db:"is_approved"`
	ProofImages []string              `json:"proof_images,omitempty"`
	Notes       string                `json:"notes,omitempty" db:"notes"`
	VerifiedAt  time.Time             `json:"verified_at" db:"verified_at"`
	CreatedAt   time.Time             `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at" db:"updated_at"`
}

type VerificationImage struct {
	ID             string    `json:"id" db:"id"`
	VerificationID string    `json:"verification_id" db:"verification_id"`
	ImageURL       string    `json:"image_url" db:"image_url"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

type Return struct {
	ID           string               `json:"id" db:"id"`
	OrderID      string               `json:"order_id" db:"order_id"`
	OrderItemID  string               `json:"order_item_id" db:"order_item_id"`
	UserID       string               `json:"user_id" db:"user_id"`
	Reason       string               `json:"reason" db:"reason"`
	Status       ReturnStatus         `json:"status" db:"status"`
	Type         ReturnType           `json:"type" db:"type"`
	RefundAmount float64              `json:"refund_amount" db:"refund_amount"`
	Verification *ProductVerification `json:"verification,omitempty"`
	RequestedAt  time.Time            `json:"requested_at" db:"requested_at"`
	ApprovedAt   *time.Time           `json:"approved_at,omitempty" db:"approved_at"`
	CompletedAt  *time.Time           `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt    time.Time            `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at" db:"updated_at"`
}

