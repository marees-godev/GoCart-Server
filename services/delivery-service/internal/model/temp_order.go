package model

import "time"

type TempOrderStatus string

const (
	TempOrderStatusPending    TempOrderStatus = "PENDING"
	TempOrderStatusConfirmed  TempOrderStatus = "CONFIRMED"
	TempOrderStatusProcessing TempOrderStatus = "PROCESSING"
	TempOrderStatusShipped    TempOrderStatus = "SHIPPED"
	TempOrderStatusDelivered  TempOrderStatus = "DELIVERED"
	TempOrderStatusCancelled  TempOrderStatus = "CANCELLED"
)

type TempOrder struct {
	ID              string          `json:"id"`
	OrderNumber     string          `json:"order_number"`
	CustomerID      *string         `json:"customer_id,omitempty"`
	Status          TempOrderStatus `json:"status"`
	TotalAmount     float64         `json:"total_amount"`
	Currency        string          `json:"currency"`
	ShippingAddress ShippingAddress `json:"shipping_address"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}
