package model

import "time"

// PaymentInitiatedEvent represents the event payload when a payment process is initiated.
type PaymentInitiatedEvent struct {
	PaymentID     string    `json:"payment_id"`
	OrderID       string    `json:"order_id"`
	UserID        string    `json:"user_id"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	PaymentMethod string    `json:"payment_method"`
	InitiatedAt   time.Time `json:"initiated_at"`
}

// PaymentSuccessfulEvent represents the event payload when a payment succeeds.
type PaymentSuccessfulEvent struct {
	PaymentID            string    `json:"payment_id"`
	OrderID              string    `json:"order_id"`
	UserID               string    `json:"user_id"`
	Amount               float64   `json:"amount"`
	Currency             string    `json:"currency"`
	PaymentMethod        string    `json:"payment_method"`
	GatewayTransactionID string    `json:"gateway_transaction_id,omitempty"`
	PaidAt               time.Time `json:"paid_at"`
}

// PaymentFailedEvent represents the event payload when a payment attempt fails.
type PaymentFailedEvent struct {
	PaymentID string    `json:"payment_id"`
	OrderID   string    `json:"order_id"`
	UserID    string    `json:"user_id"`
	Amount    float64   `json:"amount"`
	Reason    string    `json:"reason"`
	FailedAt  time.Time `json:"failed_at"`
}

// PaymentRefundedEvent represents the event payload when a payment refund completes.
type PaymentRefundedEvent struct {
	RefundID        string    `json:"refund_id"`
	PaymentID       string    `json:"payment_id"`
	OrderID         string    `json:"order_id"`
	Amount          float64   `json:"amount"`
	GatewayRefundID string    `json:"gateway_refund_id,omitempty"`
	RefundedAt      time.Time `json:"refunded_at"`
}
