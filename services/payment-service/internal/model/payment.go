package model

import (
	"time"
)

type PaymentStatus string

const (
	PaymentStatusInitiated PaymentStatus = "INITIATED"
	PaymentStatusPending   PaymentStatus = "PENDING"
	PaymentStatusSuccess   PaymentStatus = "SUCCESS"
	PaymentStatusFailed    PaymentStatus = "FAILED"
	PaymentStatusCancelled PaymentStatus = "CANCELLED"
	PaymentStatusRefunded  PaymentStatus = "REFUNDED"
)

type RefundStatus string

const (
	RefundStatusPending RefundStatus = "PENDING"
	RefundStatusSuccess RefundStatus = "SUCCESS"
	RefundStatusFailed  RefundStatus = "FAILED"
)

type Payment struct {
	ID                   string        `json:"id"`
	OrderID              string        `json:"order_id"`
	UserID               string        `json:"user_id"`
	PaymentMethod        string        `json:"payment_method"`
	PaymentMethodID      *string       `json:"payment_method_id,omitempty"`
	Amount               float64       `json:"amount"`
	Currency             string        `json:"currency"`
	Status               PaymentStatus `json:"status"`
	TransactionID        string        `json:"transaction_id"`
	GatewayTransactionID string        `json:"gateway_transaction_id,omitempty"`
	IdempotencyKey       string        `json:"idempotency_key"`
	FailureReason        string        `json:"failure_reason,omitempty"`
	CreatedAt            time.Time     `json:"created_at"`
	UpdatedAt            time.Time     `json:"updated_at"`
}

type Refund struct {
	ID              string       `json:"id"`
	PaymentID       string       `json:"payment_id"`
	OrderID         string       `json:"order_id"`
	Amount          float64      `json:"amount"`
	Reason          string       `json:"reason,omitempty"`
	Status          RefundStatus `json:"status"`
	GatewayRefundID string       `json:"gateway_refund_id,omitempty"`
	IdempotencyKey  string       `json:"idempotency_key,omitempty"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

type PaymentWebhook struct {
	ID          string     `json:"id"`
	EventID     string     `json:"event_id"`
	Provider    string     `json:"provider"`
	EventType   string     `json:"event_type"`
	Payload     []byte     `json:"payload"`
	Processed   bool       `json:"processed"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}
