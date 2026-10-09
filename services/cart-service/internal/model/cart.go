package model

import "time"

type Cart struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	Items       []CartItem `json:"items"`
	TotalAmount float64    `json:"total_amount"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type CartItem struct {
	ID        string    `json:"id"`
	CartID    string    `json:"cart_id"`
	ProductID string    `json:"product_id"`
	VariantID string    `json:"variant_id,omitempty"`
	StoreID   string    `json:"store_id,omitempty"`
	Quantity  int32     `json:"quantity"`
	UnitPrice float64   `json:"unit_price"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *Cart) CalculateTotal() {
	var total float64
	for _, item := range c.Items {
		total += float64(item.Quantity) * item.UnitPrice
	}
	c.TotalAmount = total
}
