package model

import (
	"time"

	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

type ProductStatus = productpb.ProductStatus

const (
	StatusDraft         = productpb.StatusDraft
	StatusPendingReview = productpb.StatusPendingReview
	StatusPublished     = productpb.StatusPublished
	StatusUnpublished   = productpb.StatusUnpublished
	StatusSuspended     = productpb.StatusSuspended
	StatusDeleted       = productpb.StatusDeleted

	// Legacy / Inventory compatibility statuses
	StatusInStock      = productpb.StatusInStock
	StatusOutOfStock   = productpb.StatusOutOfStock
	StatusLowStock     = productpb.StatusLowStock
	StatusReserved     = productpb.StatusReserved
	StatusDiscontinued = productpb.StatusDiscontinued
)

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

func (p *Product) ValidateForPublishing() error {
	if p == nil {
		return appErrors.BadRequest("product is nil")
	}
	if p.Name == "" {
		return appErrors.BadRequest("product name is required for publishing")
	}
	if p.Price <= 0 {
		return appErrors.BadRequest("product price must be greater than zero for publishing")
	}
	if p.CategoryID == "" {
		return appErrors.BadRequest("product category is required for publishing")
	}
	if p.ImageURL == "" && len(p.Images) == 0 {
		return appErrors.BadRequest("at least one product image is required for publishing")
	}
	return nil
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
