package dto

import (
	"strings"
	"time"

	"github.com/google/uuid"
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/utils"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/model"
)

type CreateVariantRequest struct {
	SKU            string  `json:"sku"`
	Name           string  `json:"name"`
	Price          float64 `json:"price"`
	MRP            float64 `json:"mrp"`
	Stock          int     `json:"stock"`
	AttributesJSON string  `json:"attributes_json"`
}

type CreateProductRequest struct {
	StoreID     string                 `json:"store_id"`
	CategoryID  string                 `json:"category_id"`
	SKU         string                 `json:"sku"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Price       float64                `json:"price"`
	MRP         float64                `json:"mrp"`
	Tax         float64                `json:"tax"`
	Status      string                 `json:"status"`
	ImageURL    string                 `json:"image_url,omitempty"`
	Images      []string               `json:"images"`
	Variants    []CreateVariantRequest `json:"variants"`
}

func (r *CreateProductRequest) Validate() error {
	r.StoreID = strings.TrimSpace(r.StoreID)
	if r.StoreID == "" {
		return appErrors.InvalidArgument("store_id is required")
	}
	if _, err := uuid.Parse(r.StoreID); err != nil {
		return appErrors.InvalidArgument("store_id must be a valid UUID")
	}

	r.CategoryID = strings.TrimSpace(r.CategoryID)
	if r.CategoryID == "" {
		return appErrors.InvalidArgument("category_id is required")
	}
	if _, err := uuid.Parse(r.CategoryID); err != nil {
		return appErrors.InvalidArgument("category_id must be a valid UUID")
	}

	r.SKU = strings.TrimSpace(r.SKU)
	if r.SKU == "" {
		r.SKU = utils.GenerateSKU()
	}

	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return appErrors.InvalidArgument("product name is required")
	}

	if r.Price < 0 {
		return appErrors.InvalidArgument("price cannot be negative")
	}

	if r.MRP < r.Price {
		return appErrors.InvalidArgument("MRP must be greater than or equal to price")
	}

	if r.Tax < 0 {
		return appErrors.InvalidArgument("tax cannot be negative")
	}

	if r.Status != "" {
		st := model.ProductStatus(strings.ToUpper(strings.TrimSpace(r.Status)))
		if !st.IsValid() {
			return appErrors.InvalidArgument("invalid product status")
		}
		r.Status = string(st)
	} else {
		r.Status = string(model.StatusInStock)
	}

	for i, v := range r.Variants {
		v.SKU = strings.TrimSpace(v.SKU)
		v.Name = strings.TrimSpace(v.Name)
		if v.SKU == "" {
			v.SKU = utils.GenerateSKU()
		}
		if v.Price < 0 {
			return appErrors.InvalidArgument("variant price cannot be negative")
		}
		if v.MRP < v.Price {
			return appErrors.InvalidArgument("variant MRP must be greater than or equal to price")
		}
		if v.Stock < 0 {
			return appErrors.InvalidArgument("variant stock cannot be negative")
		}
		r.Variants[i] = v
	}

	return nil
}

type UpdateVariantRequest struct {
	ID             string  `json:"id"`
	SKU            string  `json:"sku"`
	Name           string  `json:"name"`
	Price          float64 `json:"price"`
	MRP            float64 `json:"mrp"`
	Stock          int     `json:"stock"`
	AttributesJSON string  `json:"attributes_json"`
	Status         string  `json:"status"`
}

type UpdateProductRequest struct {
	ID          string                 `json:"id"`
	Name        *string                `json:"name,omitempty"`
	Description *string                `json:"description,omitempty"`
	Price       *float64               `json:"price,omitempty"`
	MRP         *float64               `json:"mrp,omitempty"`
	Tax         *float64               `json:"tax,omitempty"`
	Status      *string                `json:"status,omitempty"`
	CategoryID  *string                `json:"category_id,omitempty"`
	ImageURL    *string                `json:"image_url,omitempty"`
	Images      []string               `json:"images,omitempty"`
	Variants    []UpdateVariantRequest `json:"variants,omitempty"`
}

func (r *UpdateProductRequest) Validate() error {
	r.ID = strings.TrimSpace(r.ID)
	if r.ID == "" {
		return appErrors.InvalidArgument("product id is required")
	}
	if _, err := uuid.Parse(r.ID); err != nil {
		return appErrors.InvalidArgument("product id must be a valid UUID")
	}

	if r.Name != nil {
		n := strings.TrimSpace(*r.Name)
		if n == "" {
			return appErrors.InvalidArgument("product name cannot be empty")
		}
		r.Name = &n
	}

	if r.CategoryID != nil {
		c := strings.TrimSpace(*r.CategoryID)
		if c == "" {
			return appErrors.InvalidArgument("category_id cannot be empty")
		}
		if _, err := uuid.Parse(c); err != nil {
			return appErrors.InvalidArgument("category_id must be a valid UUID")
		}
		r.CategoryID = &c
	}

	if r.Price != nil && *r.Price < 0 {
		return appErrors.InvalidArgument("price cannot be negative")
	}

	if r.Price != nil && r.MRP != nil && *r.MRP < *r.Price {
		return appErrors.InvalidArgument("MRP must be greater than or equal to price")
	}

	if r.Tax != nil && *r.Tax < 0 {
		return appErrors.InvalidArgument("tax cannot be negative")
	}

	if r.Status != nil {
		st := model.ProductStatus(strings.ToUpper(strings.TrimSpace(*r.Status)))
		if !st.IsValid() {
			return appErrors.InvalidArgument("invalid product status")
		}
		sStr := string(st)
		r.Status = &sStr
	}

	return nil
}

type ListProductsRequest struct {
	Limit      int32  `json:"limit"`
	Offset     int32  `json:"offset"`
	CategoryID string `json:"category_id"`
	StoreID    string `json:"store_id"`
	Status     string `json:"status"`
}

func ToProductPB(p *model.Product) *productpb.Product {
	if p == nil {
		return nil
	}

	images := make([]string, 0, len(p.Images))
	for _, img := range p.Images {
		images = append(images, img.URL)
	}

	variants := make([]*productpb.ProductVariant, 0, len(p.Variants))
	for _, v := range p.Variants {
		variants = append(variants, ToProductVariantPB(&v))
	}

	var deletedAtStr string
	if p.DeletedAt != nil {
		deletedAtStr = p.DeletedAt.Format(time.RFC3339)
	}

	return &productpb.Product{
		Id:          p.ID,
		StoreId:     p.StoreID,
		CategoryId:  p.CategoryID,
		Sku:         p.SKU,
		Name:        p.Name,
		Productname: p.Name,
		Description: p.Description,
		Price:       p.Price,
		Mrp:         p.MRP,
		Tax:         p.Tax,
		Status:      string(p.Status),
		Images:      images,
		Variants:    variants,
		AvgRating:   p.AvgRating,
		CreatedAt:   p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   p.UpdatedAt.Format(time.RFC3339),
		DeletedAt:   deletedAtStr,
	}
}

func ToProductListPB(products []*model.Product) []*productpb.Product {
	res := make([]*productpb.Product, 0, len(products))
	for _, p := range products {
		res = append(res, ToProductPB(p))
	}
	return res
}

func ToProductVariantPB(v *model.ProductVariant) *productpb.ProductVariant {
	if v == nil {
		return nil
	}
	var deletedAtStr string
	if v.DeletedAt != nil {
		deletedAtStr = v.DeletedAt.Format(time.RFC3339)
	}

	return &productpb.ProductVariant{
		Id:             v.ID,
		ProductId:      v.ProductID,
		Sku:            v.SKU,
		Name:           v.Name,
		Price:          v.Price,
		Mrp:            v.MRP,
		Stock:          int32(v.Stock),
		AttributesJson: v.AttributesJSON,
		Status:         string(v.Status),
		CreatedAt:      v.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      v.UpdatedAt.Format(time.RFC3339),
		DeletedAt:      deletedAtStr,
	}
}
