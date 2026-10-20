package dto

import (
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/model"
)

type CreateOrderItemInput struct {
	ProductID string  `json:"product_id"`
	StoreID   string  `json:"store_id"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

type CreateOrderInput struct {
	UserID            string                 `json:"user_id"`
	CartID            string                 `json:"cart_id"`
	ShippingAddress   string                 `json:"shipping_address"`
	ShippingAddressID string                 `json:"shipping_address_id"`
	BillingAddressID  string                 `json:"billing_address_id"`
	Currency          string                 `json:"currency"`
	IdempotencyKey    string                 `json:"idempotency_key"`
	ShippingFee       float64                `json:"shipping_fee"`
	DiscountAmount    float64                `json:"discount_amount"`
	Items             []CreateOrderItemInput `json:"items,omitempty"`
}

type CreateOrderResult struct {
	PrimaryOrder  *model.Order   `json:"primary_order"`
	Orders        []*model.Order `json:"orders"`
	ParentOrderID string         `json:"parent_order_id"`
}

type ListOrdersInput struct {
	UserID string `json:"user_id"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type ListOrdersResult struct {
	Orders []*model.Order `json:"orders"`
	Total  int            `json:"total"`
}
