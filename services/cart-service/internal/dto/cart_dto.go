package dto

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
