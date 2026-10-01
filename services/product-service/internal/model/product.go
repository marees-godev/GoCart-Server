package model

import (
	"time"
)

type ProductStatus string

const (
	StatusStockIn      ProductStatus = "stock_in"
	StatusStockOut     ProductStatus = "stock_out"
	StatusLowStock     ProductStatus = "low_stock"
	StatusReserved     ProductStatus = "reserved"
	StatusDiscontinued ProductStatus = "discontinued"
)

func (s ProductStatus) IsValid() bool {
	switch s {
	case StatusStockIn, StatusStockOut, StatusLowStock, StatusReserved, StatusDiscontinued:
		return true
	default:
		return false
	}
}

type Product struct {
	ID          string           `json:"id" db:"id"`
	StoreID     string           `json:"store_id" db:"store_id"`
	CategoryID  string           `json:"category_id" db:"category_id"`
	SKU         string           `json:"sku" db:"sku"`
	Name        string           `json:"name" db:"name"`
	Description string           `json:"description" db:"description"`
	Price       float64          `json:"price" db:"price"`
	MRP         float64          `json:"mrp" db:"mrp"`
	Tax         float64          `json:"tax" db:"tax"`
	Currency    string           `json:"currency" db:"currency"`
	Status      ProductStatus    `json:"status" db:"status"`
	ImageURL    string           `json:"image_url,omitempty" db:"image_url"`
	AvgRating   float64          `json:"avg_rating" db:"avg_rating"`
	Images      []ProductImage   `json:"images,omitempty"`
	Variants    []ProductVariant `json:"variants,omitempty"`
	CreatedAt   time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at" db:"updated_at"`
	DeletedAt   *time.Time       `json:"deleted_at,omitempty" db:"deleted_at"`
}

type ProductImage struct {
	ID           string    `json:"id" db:"id"`
	ProductID    string    `json:"product_id" db:"product_id"`
	URL          string    `json:"url" db:"url"`
	AltText      string    `json:"alt_text,omitempty" db:"alt_text"`
	IsPrimary    bool      `json:"is_primary" db:"is_primary"`
	DisplayOrder int       `json:"display_order" db:"display_order"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type ProductVariant struct {
	ID             string        `json:"id" db:"id"`
	ProductID      string        `json:"product_id" db:"product_id"`
	SKU            string        `json:"sku" db:"sku"`
	Name           string        `json:"name" db:"name"`
	Price          float64       `json:"price" db:"price"`
	MRP            float64       `json:"mrp" db:"mrp"`
	Stock          int           `json:"stock" db:"stock"`
	AttributesJSON string        `json:"attributes_json,omitempty" db:"attributes"`
	Status         ProductStatus `json:"status" db:"status"`
	CreatedAt      time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at" db:"updated_at"`
	DeletedAt      *time.Time    `json:"deleted_at,omitempty" db:"deleted_at"`
}
