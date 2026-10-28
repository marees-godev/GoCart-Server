package model

import "time"

type Rating struct {
	ID                 string    `json:"id" db:"id"`
	UserID             string    `json:"user_id" db:"user_id"`
	ProductID          string    `json:"product_id" db:"product_id"`
	OrderID            string    `json:"order_id" db:"order_id"`
	OrderItemID        string    `json:"order_item_id" db:"order_item_id"`
	Rating             int       `json:"rating" db:"rating"`
	Review             string    `json:"review" db:"review"`
	IsVerifiedPurchase bool      `json:"is_verified_purchase" db:"is_verified_purchase"`
	IsFlagged          bool      `json:"is_flagged" db:"is_flagged"`
	IsHidden           bool      `json:"is_hidden" db:"is_hidden"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}
