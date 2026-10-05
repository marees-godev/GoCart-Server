package dto_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/dto"
)

func TestCreateProductRequest_AutoGenerateSKU(t *testing.T) {
	req := dto.CreateProductRequest{
		StoreID:    uuid.New().String(),
		CategoryID: uuid.New().String(),
		Name:       "Test Product",
		Price:      100.0,
		MRP:        120.0,
		SKU:        "", // Empty SKU should be auto-generated
		Variants: []dto.CreateVariantRequest{
			{
				Name:  "Variant 1",
				Price: 100.0,
				MRP:   120.0,
				SKU:   "", // Empty Variant SKU should be auto-generated
			},
		},
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid request, got error: %v", err)
	}

	if req.SKU == "" || !strings.HasPrefix(req.SKU, "SKU-") {
		t.Errorf("expected product SKU to be generated with prefix SKU-, got %s", req.SKU)
	}

	if req.Variants[0].SKU == "" || !strings.HasPrefix(req.Variants[0].SKU, "SKU-") {
		t.Errorf("expected variant SKU to be generated with prefix SKU-, got %s", req.Variants[0].SKU)
	}
}
