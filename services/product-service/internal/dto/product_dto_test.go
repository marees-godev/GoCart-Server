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

func TestCreateProductRequest_VariantValidation(t *testing.T) {
	storeID := uuid.New().String()
	catID := uuid.New().String()

	t.Run("valid product with multiple variants", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    storeID,
			CategoryID: catID,
			Name:       "T-Shirt",
			Price:      799.0,
			MRP:        999.0,
			Variants: []dto.CreateVariantRequest{
				{
					SKU:            "TS-RED-M",
					Name:           "Red / M",
					Price:          799.0,
					MRP:            999.0,
					Stock:          10,
					AttributesJSON: `{"color":"Red","size":"M"}`,
				},
				{
					SKU:            "TS-RED-L",
					Name:           "Red / L",
					Price:          799.0,
					MRP:            999.0,
					Stock:          15,
					AttributesJSON: `{"color":"Red","size":"L"}`,
				},
				{
					SKU:            "TS-BLU-M",
					Name:           "Blue / M",
					Price:          849.0,
					MRP:            999.0,
					Stock:          5,
					AttributesJSON: `{"color":"Blue","size":"M"}`,
				},
			},
		}

		if err := req.Validate(); err != nil {
			t.Fatalf("expected valid request, got %v", err)
		}
	})

	t.Run("reject duplicate variant SKU in request", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    storeID,
			CategoryID: catID,
			Name:       "T-Shirt",
			Price:      799.0,
			MRP:        999.0,
			Variants: []dto.CreateVariantRequest{
				{SKU: "TS-RED-M", Name: "Red / M", Price: 799.0, MRP: 999.0},
				{SKU: "TS-RED-M", Name: "Red / M Duplicate", Price: 799.0, MRP: 999.0},
			},
		}

		err := req.Validate()
		if err == nil || !strings.Contains(err.Error(), "duplicate variant SKU") {
			t.Fatalf("expected duplicate variant SKU error, got %v", err)
		}
	})

	t.Run("reject negative variant price", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    storeID,
			CategoryID: catID,
			Name:       "T-Shirt",
			Price:      799.0,
			MRP:        999.0,
			Variants: []dto.CreateVariantRequest{
				{SKU: "TS-RED-M", Name: "Red / M", Price: -10.0, MRP: 999.0},
			},
		}

		err := req.Validate()
		if err == nil || !strings.Contains(err.Error(), "variant price cannot be negative") {
			t.Fatalf("expected negative price error, got %v", err)
		}
	})

	t.Run("reject MRP less than price for variant", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    storeID,
			CategoryID: catID,
			Name:       "T-Shirt",
			Price:      799.0,
			MRP:        999.0,
			Variants: []dto.CreateVariantRequest{
				{SKU: "TS-RED-M", Name: "Red / M", Price: 799.0, MRP: 500.0},
			},
		}

		err := req.Validate()
		if err == nil || !strings.Contains(err.Error(), "variant MRP must be greater than or equal to price") {
			t.Fatalf("expected MRP error, got %v", err)
		}
	})

	t.Run("reject invalid variant attributes json", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    storeID,
			CategoryID: catID,
			Name:       "T-Shirt",
			Price:      799.0,
			MRP:        999.0,
			Variants: []dto.CreateVariantRequest{
				{SKU: "TS-RED-M", Name: "Red / M", Price: 799.0, MRP: 999.0, AttributesJSON: "{bad_json"},
			},
		}

		err := req.Validate()
		if err == nil || !strings.Contains(err.Error(), "invalid variant attributes json") {
			t.Fatalf("expected invalid attributes json error, got %v", err)
		}
	})
}

