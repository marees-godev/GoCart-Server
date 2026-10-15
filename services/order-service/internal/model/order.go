package model

import (
	"time"
)

type OrderStatus string

const (
	OrderStatusCreated    OrderStatus = "CREATED"
	OrderStatusConfirmed  OrderStatus = "CONFIRMED"
	OrderStatusProcessing OrderStatus = "PROCESSING"
	OrderStatusShipped    OrderStatus = "SHIPPED"
	OrderStatusDelivered  OrderStatus = "DELIVERED"
	OrderStatusCancelled  OrderStatus = "CANCELLED"
	OrderStatusReturned   OrderStatus = "RETURNED"
)

func (s OrderStatus) IsValid() bool {
	switch s {
	case OrderStatusCreated, OrderStatusConfirmed, OrderStatusProcessing,
		OrderStatusShipped, OrderStatusDelivered, OrderStatusCancelled, OrderStatusReturned:
		return true
	default:
		return false
	}
}

type PaymentStatus string

const (
	PaymentStatusPending  PaymentStatus = "PENDING"
	PaymentStatusPaid     PaymentStatus = "PAID"
	PaymentStatusFailed   PaymentStatus = "FAILED"
	PaymentStatusRefunded PaymentStatus = "REFUNDED"
)

func (p PaymentStatus) IsValid() bool {
	switch p {
	case PaymentStatusPending, PaymentStatusPaid, PaymentStatusFailed, PaymentStatusRefunded:
		return true
	default:
		return false
	}
}

type Order struct {
	ID                string        `json:"id" db:"id"`
	OrderNumber       string        `json:"order_number" db:"order_number"`
	ParentOrderID     *string       `json:"parent_order_id,omitempty" db:"parent_order_id"`
	UserID            string        `json:"user_id" db:"user_id"`
	StoreID           string        `json:"store_id" db:"store_id"`
	Subtotal          float64       `json:"subtotal" db:"subtotal"`
	TaxAmount         float64       `json:"tax_amount" db:"tax_amount"`
	ShippingFee       float64       `json:"shipping_fee" db:"shipping_fee"`
	DiscountAmount    float64       `json:"discount_amount" db:"discount_amount"`
	TotalAmount       float64       `json:"total_amount" db:"total_amount"`
	Currency          string        `json:"currency" db:"currency"`
	PaymentStatus     PaymentStatus `json:"payment_status" db:"payment_status"`
	Status            OrderStatus   `json:"status" db:"status"`
	ShippingAddress   string        `json:"shipping_address,omitempty" db:"shipping_address"`
	BillingAddress    string        `json:"billing_address,omitempty" db:"billing_address"`
	ShippingAddressID *string       `json:"shipping_address_id,omitempty" db:"shipping_address_id"`
	BillingAddressID  *string       `json:"billing_address_id,omitempty" db:"billing_address_id"`
	IdempotencyKey    *string       `json:"idempotency_key,omitempty" db:"idempotency_key"`
	Items             []OrderItem   `json:"items,omitempty" db:"-"`
	CreatedAt         time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at" db:"updated_at"`
}

type OrderItem struct {
	ID          string  `json:"id" db:"id"`
	OrderID     string  `json:"order_id" db:"order_id"`
	ProductID   string  `json:"product_id" db:"product_id"`
	StoreID     string  `json:"store_id" db:"store_id"`
	SKU         string  `json:"sku" db:"sku"`
	ProductName string  `json:"product_name" db:"product_name"`
	Quantity    int     `json:"quantity" db:"quantity"`
	UnitPrice   float64 `json:"unit_price" db:"unit_price"`
	Tax         float64 `json:"tax" db:"tax"`
	Discount    float64 `json:"discount" db:"discount"`
	TotalPrice  float64 `json:"total_price" db:"total_price"`
}
