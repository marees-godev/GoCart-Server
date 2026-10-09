package dto

import "github.com/marees-godev/GoCart-Server/services/cart-service/internal/model"

type AddCartItemRequest struct {
	UserID    string  `json:"user_id"`
	ProductID string  `json:"product_id"`
	VariantID string  `json:"variant_id,omitempty"`
	StoreID   string  `json:"store_id,omitempty"`
	UnitPrice float64 `json:"unit_price"`
	Quantity  int32   `json:"quantity"`
}

type UpdateCartItemRequest struct {
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	VariantID string `json:"variant_id,omitempty"`
	Quantity  int32  `json:"quantity"`
}

type RemoveCartItemRequest struct {
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	VariantID string `json:"variant_id,omitempty"`
}

type ClearCartRequest struct {
	UserID string `json:"user_id"`
}

type GetCartRequest struct {
	UserID string `json:"user_id"`
}

const (
	ErrCodeEmptyCart       int32 = 400
	ErrCodeProductNotFound int32 = 404
	ErrCodeOutOfStock      int32 = 409
	ErrCodeProductInactive int32 = 422
)

type ValidationError struct {
	ProductID string `json:"product_id"`
	Message   string `json:"message"`
	Code      int32  `json:"code"`
}

type StoreOrderGroup struct {
	StoreID  string           `json:"store_id"`
	Items    []model.CartItem `json:"items"`
	Subtotal float64          `json:"subtotal"`
}

type ValidateCartRequest struct {
	UserID string `json:"user_id"`
}

type ValidateCartResponse struct {
	IsValid     bool              `json:"is_valid"`
	Cart        *model.Cart       `json:"cart"`
	Errors      []ValidationError `json:"errors"`
	StoreGroups []StoreOrderGroup `json:"store_groups"`
}

type PrepareCheckoutRequest struct {
	UserID          string `json:"user_id"`
	ShippingAddress string `json:"shipping_address"`
}

type StoreOrderPayload struct {
	ParentOrderID   string           `json:"parent_order_id"`
	StoreID         string           `json:"store_id"`
	UserID          string           `json:"user_id"`
	Items           []model.CartItem `json:"items"`
	Subtotal        float64          `json:"subtotal"`
	TotalAmount     float64          `json:"total_amount"`
	ShippingAddress string           `json:"shipping_address"`
}

type PrepareCheckoutResponse struct {
	IsValid       bool                `json:"is_valid"`
	ParentOrderID string              `json:"parent_order_id"`
	Orders        []StoreOrderPayload `json:"orders"`
	Errors        []ValidationError   `json:"errors"`
}
