package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	categorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/category"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	storepb "github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/service"
	"google.golang.org/grpc"
)

type mockRepo struct {
	counter  int64
	products map[string]*model.Product
	skus     map[string]*model.Product // key: storeID + ":" + sku
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		products: make(map[string]*model.Product),
		skus:     make(map[string]*model.Product),
	}
}

func (m *mockRepo) CreateProduct(ctx context.Context, p *model.Product) error {
	idNum := atomic.AddInt64(&m.counter, 1)
	p.ID = fmt.Sprintf("33333333-3333-3333-3333-%012d", idNum)
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()

	key := p.StoreID + ":" + p.SKU
	if existing, exists := m.skus[key]; exists && existing.Status != model.StatusDiscontinued && existing.DeletedAt == nil {
		return appErrors.AlreadyExists("product SKU already exists in this store")
	}

	m.products[p.ID] = p
	m.skus[key] = p
	return nil
}

func (m *mockRepo) GetProductByID(ctx context.Context, id string) (*model.Product, error) {
	p, ok := m.products[id]
	if !ok || p.Status == model.StatusDiscontinued || p.DeletedAt != nil {
		return nil, appErrors.NotFound("product not found")
	}
	return p, nil
}

func (m *mockRepo) GetProductBySKU(ctx context.Context, storeID, sku string) (*model.Product, error) {
	key := storeID + ":" + sku
	p, ok := m.skus[key]
	if !ok || p.Status == model.StatusDiscontinued || p.DeletedAt != nil {
		return nil, appErrors.NotFound("product not found")
	}
	return p, nil
}

func (m *mockRepo) ListProducts(ctx context.Context, filter dto.ListProductsRequest) ([]*model.Product, int32, error) {
	res := []*model.Product{}
	for _, p := range m.products {
		if p.Status == model.StatusDiscontinued || p.DeletedAt != nil {
			continue
		}
		if filter.StoreID != "" && p.StoreID != filter.StoreID {
			continue
		}
		if filter.CategoryID != "" && p.CategoryID != filter.CategoryID {
			continue
		}
		res = append(res, p)
	}
	return res, int32(len(res)), nil
}

func (m *mockRepo) UpdateProduct(ctx context.Context, p *model.Product) error {
	if p.Status == model.StatusDiscontinued || p.DeletedAt != nil {
		return appErrors.NotFound("product not found")
	}
	p.UpdatedAt = time.Now()
	m.products[p.ID] = p
	return nil
}

func (m *mockRepo) DeleteProduct(ctx context.Context, id string) error {
	p, ok := m.products[id]
	if !ok || p.Status == model.StatusDiscontinued || p.DeletedAt != nil {
		return appErrors.NotFound("product not found")
	}
	now := time.Now()
	p.Status = model.StatusDiscontinued
	p.DeletedAt = &now
	return nil
}

func (m *mockRepo) DeleteProductVariant(ctx context.Context, productID, variantID string) error {
	p, ok := m.products[productID]
	if !ok {
		return appErrors.NotFound("product not found")
	}
	for i, v := range p.Variants {
		if v.ID == variantID {
			now := time.Now()
			p.Variants[i].Status = model.StatusDiscontinued
			p.Variants[i].DeletedAt = &now
			return nil
		}
	}
	return appErrors.NotFound("product variant not found for this product")
}

type mockStoreClient struct {
	stores map[string]*storepb.Store
}

func (m *mockStoreClient) GetStore(ctx context.Context, in *storepb.GetStoreRequest, opts ...grpc.CallOption) (*storepb.GetStoreResponse, error) {
	st, ok := m.stores[in.Id]
	if !ok {
		return nil, errors.New("store not found")
	}
	return &storepb.GetStoreResponse{Store: st}, nil
}

type mockCategoryClient struct {
	categories map[string]*categorypb.Category
}

func (m *mockCategoryClient) GetCategory(ctx context.Context, in *categorypb.GetCategoryRequest, opts ...grpc.CallOption) (*categorypb.GetCategoryResponse, error) {
	cat, ok := m.categories[in.Id]
	if !ok {
		return nil, errors.New("category not found")
	}
	return &categorypb.GetCategoryResponse{Category: cat}, nil
}

func TestProductService_CreateProduct(t *testing.T) {
	validStoreID := "11111111-1111-1111-1111-111111111111"
	validCatID := "22222222-2222-2222-2222-222222222222"
	merchantID := "merchant-123"

	repo := newMockRepo()
	storeClient := &mockStoreClient{
		stores: map[string]*storepb.Store{
			validStoreID: {Id: validStoreID, MerchantId: merchantID, Name: "Test Store", ApprovalStatus: "APPROVED"},
		},
	}
	catClient := &mockCategoryClient{
		categories: map[string]*categorypb.Category{
			validCatID: {Id: validCatID, Name: "Electronics", IsActive: true},
		},
	}

	svc := service.NewProductService(repo, storeClient, catClient, nil)

	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: merchantID,
		Role:   auth.RoleMerchant,
	})

	t.Run("successful product creation by store merchant", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:     validStoreID,
			CategoryID:  validCatID,
			SKU:         "IPHONE15-BLK",
			Name:        "iPhone 15",
			Description: "Latest Apple Smartphone",
			Price:       999.00,
			MRP:         1099.00,
			Tax:         5.00,
			Status:      "IN_STOCK",
			Images:      []string{"https://img.com/iphone15.jpg"},
		}

		prod, err := svc.CreateProduct(merchantCtx, req)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if prod.ID == "" {
			t.Errorf("expected product ID to be generated")
		}
		if prod.Name != "iPhone 15" {
			t.Errorf("expected name iPhone 15, got %s", prod.Name)
		}
	})

	t.Run("failure when merchant does not own store", func(t *testing.T) {
		otherMerchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: "other-merchant-456",
			Role:   auth.RoleMerchant,
		})

		req := dto.CreateProductRequest{
			StoreID:    validStoreID,
			CategoryID: validCatID,
			SKU:        "IPHONE15-WHT",
			Name:       "iPhone 15 White",
			Price:      999.00,
			MRP:        1099.00,
		}

		_, err := svc.CreateProduct(otherMerchantCtx, req)
		if err == nil {
			t.Fatalf("expected forbidden error, got nil")
		}
		var appErr *appErrors.AppError
		if errors.As(err, &appErr) && appErr.Code != appErrors.CodeForbidden {
			t.Errorf("expected CodeForbidden, got %s", appErr.Code)
		}
	})

	t.Run("failure when category is invalid", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    validStoreID,
			CategoryID: "33333333-3333-3333-3333-333333333333",
			SKU:        "IPHONE15-RED",
			Name:       "iPhone 15 Red",
			Price:      999.00,
			MRP:        1099.00,
		}

		_, err := svc.CreateProduct(merchantCtx, req)
		if err == nil {
			t.Fatalf("expected invalid category error, got nil")
		}
	})

	t.Run("failure when price is negative", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    validStoreID,
			CategoryID: validCatID,
			SKU:        "IPHONE15-NEG",
			Name:       "iPhone 15 Negative",
			Price:      -100.00,
			MRP:        1099.00,
		}

		_, err := svc.CreateProduct(merchantCtx, req)
		if err == nil {
			t.Fatalf("expected validation error for negative price, got nil")
		}
	})
}

func TestProductService_UpdateAndDeleteProduct(t *testing.T) {
	validStoreID := "11111111-1111-1111-1111-111111111111"
	validCatID := "22222222-2222-2222-2222-222222222222"
	merchantID := "merchant-123"

	repo := newMockRepo()
	storeClient := &mockStoreClient{
		stores: map[string]*storepb.Store{
			validStoreID: {Id: validStoreID, MerchantId: merchantID, Name: "Test Store", ApprovalStatus: "APPROVED"},
		},
	}
	catClient := &mockCategoryClient{
		categories: map[string]*categorypb.Category{
			validCatID: {Id: validCatID, Name: "Electronics", IsActive: true},
		},
	}

	svc := service.NewProductService(repo, storeClient, catClient, nil)

	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: merchantID,
		Role:   auth.RoleMerchant,
	})

	prod, err := svc.CreateProduct(merchantCtx, dto.CreateProductRequest{
		StoreID:    validStoreID,
		CategoryID: validCatID,
		SKU:        "MACBOOK-PRO",
		Name:       "MacBook Pro 16",
		Price:      2499.00,
		MRP:        2699.00,
	})
	if err != nil {
		t.Fatalf("failed to create product: %v", err)
	}

	t.Run("merchant can update own product", func(t *testing.T) {
		newName := "MacBook Pro M3 Max"
		newPrice := 2599.00

		updated, err := svc.UpdateProduct(merchantCtx, dto.UpdateProductRequest{
			ID:    prod.ID,
			Name:  &newName,
			Price: &newPrice,
		})
		if err != nil {
			t.Fatalf("expected update success, got %v", err)
		}
		if updated.Name != newName || updated.Price != newPrice {
			t.Errorf("expected updated fields, got name=%s price=%f", updated.Name, updated.Price)
		}
	})

	t.Run("other merchant cannot update product", func(t *testing.T) {
		otherCtx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: "hacker-merchant",
			Role:   auth.RoleMerchant,
		})
		newName := "Hacked MacBook"

		_, err := svc.UpdateProduct(otherCtx, dto.UpdateProductRequest{
			ID:   prod.ID,
			Name: &newName,
		})
		if err == nil {
			t.Fatalf("expected forbidden error, got nil")
		}
	})

	t.Run("merchant can soft-delete own product", func(t *testing.T) {
		err := svc.DeleteProduct(merchantCtx, prod.ID)
		if err != nil {
			t.Fatalf("expected delete success, got %v", err)
		}

		_, err = svc.GetProduct(merchantCtx, prod.ID)
		if err == nil {
			t.Fatalf("expected product to be not found after deletion")
		}
	})
}

func TestProductService_ProductVariants(t *testing.T) {
	validStoreID := "11111111-1111-1111-1111-111111111111"
	validCatID := "22222222-2222-2222-2222-222222222222"
	merchantID := "merchant-123"

	repo := newMockRepo()
	storeClient := &mockStoreClient{
		stores: map[string]*storepb.Store{
			validStoreID: {Id: validStoreID, MerchantId: merchantID, Name: "Test Store", ApprovalStatus: "APPROVED"},
		},
	}
	catClient := &mockCategoryClient{
		categories: map[string]*categorypb.Category{
			validCatID: {Id: validCatID, Name: "Apparel", IsActive: true},
		},
	}

	svc := service.NewProductService(repo, storeClient, catClient, nil)

	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: merchantID,
		Role:   auth.RoleMerchant,
	})

	t.Run("create product with multiple size and color variants", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    validStoreID,
			CategoryID: validCatID,
			SKU:        "TSHIRT-BASE",
			Name:       "T-Shirt",
			Price:      799.00,
			MRP:        999.00,
			Variants: []dto.CreateVariantRequest{
				{
					SKU:            "TS-RED-M",
					Name:           "Red / M",
					Price:          799.00,
					MRP:            999.00,
					Stock:          20,
					AttributesJSON: `{"color":"Red","size":"M"}`,
				},
				{
					SKU:            "TS-RED-L",
					Name:           "Red / L",
					Price:          799.00,
					MRP:            999.00,
					Stock:          15,
					AttributesJSON: `{"color":"Red","size":"L"}`,
				},
				{
					SKU:            "TS-BLU-M",
					Name:           "Blue / M",
					Price:          849.00,
					MRP:            999.00,
					Stock:          10,
					AttributesJSON: `{"color":"Blue","size":"M"}`,
				},
			},
		}

		prod, err := svc.CreateProduct(merchantCtx, req)
		if err != nil {
			t.Fatalf("expected creation success, got error: %v", err)
		}

		if len(prod.Variants) != 3 {
			t.Fatalf("expected 3 variants, got %d", len(prod.Variants))
		}

		if prod.Variants[0].SKU != "TS-RED-M" || prod.Variants[0].Price != 799.00 {
			t.Errorf("variant 0 unexpected: %+v", prod.Variants[0])
		}
		if prod.Variants[2].SKU != "TS-BLU-M" || prod.Variants[2].Price != 849.00 {
			t.Errorf("variant 2 unexpected: %+v", prod.Variants[2])
		}
	})

	t.Run("create product with zero variants", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    validStoreID,
			CategoryID: validCatID,
			SKU:        "CAP-SINGLE",
			Name:       "Baseball Cap",
			Price:      299.00,
			MRP:        399.00,
			Variants:   []dto.CreateVariantRequest{},
		}

		prod, err := svc.CreateProduct(merchantCtx, req)
		if err != nil {
			t.Fatalf("expected creation success, got error: %v", err)
		}
		if len(prod.Variants) != 0 {
			t.Errorf("expected 0 variants, got %d", len(prod.Variants))
		}
	})

	t.Run("delete product variant", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    validStoreID,
			CategoryID: validCatID,
			SKU:        "PANTS-BASE",
			Name:       "Jeans",
			Price:      1499.00,
			MRP:        1999.00,
			Variants: []dto.CreateVariantRequest{
				{
					SKU:   "PANTS-30",
					Name:  "Size 30",
					Price: 1499.00,
					MRP:   1999.00,
				},
			},
		}

		prod, err := svc.CreateProduct(merchantCtx, req)
		if err != nil {
			t.Fatalf("expected creation success, got error: %v", err)
		}

		// Manually assign ID for test
		prod.Variants[0].ID = "var-1111-2222"

		err = svc.DeleteProductVariant(merchantCtx, prod.ID, prod.Variants[0].ID)
		if err != nil {
			t.Fatalf("expected delete variant success, got %v", err)
		}
	})
}

type mockMerchantClient struct {
	merchants map[string]*merchantpb.MerchantResponseData
}

func (m *mockMerchantClient) GetMerchant(ctx context.Context, in *merchantpb.GetMerchantRequest, opts ...grpc.CallOption) (*merchantpb.GetMerchantResponse, error) {
	merch, ok := m.merchants[in.Id]
	if !ok {
		return nil, errors.New("merchant not found")
	}
	return &merchantpb.GetMerchantResponse{Merchant: merch}, nil
}

func TestProductService_PublishingRulesAndPrerequisites(t *testing.T) {
	validStoreID := "11111111-1111-1111-1111-111111111111"
	unapprovedStoreID := "22222222-2222-2222-2222-222222222222"
	activeMerchantID := "33333333-3333-3333-3333-333333333333"
	suspendedMerchantID := "44444444-4444-4444-4444-444444444444"
	validCatID := "55555555-5555-5555-5555-555555555555"

	repo := newMockRepo()
	storeClient := &mockStoreClient{
		stores: map[string]*storepb.Store{
			validStoreID:      {Id: validStoreID, MerchantId: activeMerchantID, Name: "Approved Store", ApprovalStatus: "APPROVED"},
			unapprovedStoreID: {Id: unapprovedStoreID, MerchantId: activeMerchantID, Name: "Pending Store", ApprovalStatus: "PENDING_APPROVAL"},
		},
	}
	catClient := &mockCategoryClient{
		categories: map[string]*categorypb.Category{
			validCatID: {Id: validCatID, Name: "Electronics", IsActive: true},
		},
	}
	merchClient := &mockMerchantClient{
		merchants: map[string]*merchantpb.MerchantResponseData{
			activeMerchantID:    {Id: activeMerchantID, Status: merchantpb.MerchantStatus_APPROVED},
			suspendedMerchantID: {Id: suspendedMerchantID, Status: merchantpb.MerchantStatus_SUSPENDED},
		},
	}

	svc := service.NewProductServiceWithClients(repo, storeClient, catClient, merchClient, nil)
	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: activeMerchantID,
		Role:   auth.RoleMerchant,
	})

	t.Run("cannot publish product with missing required fields (no image)", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    validStoreID,
			CategoryID: validCatID,
			SKU:        "NO-IMG",
			Name:       "No Image Product",
			Price:      100.0,
			MRP:        120.0,
			Status:     "PUBLISHED",
		}
		_, err := svc.CreateProduct(merchantCtx, req)
		if err == nil {
			t.Fatal("expected error publishing product with no images")
		}
	})

	t.Run("cannot publish product if store is not APPROVED", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    unapprovedStoreID,
			CategoryID: validCatID,
			SKU:        "UNAPP-STORE",
			Name:       "Unapproved Store Product",
			Price:      100.0,
			MRP:        120.0,
			Status:     "PUBLISHED",
			Images:     []string{"https://img.com/p.jpg"},
		}
		_, err := svc.CreateProduct(merchantCtx, req)
		if err == nil {
			t.Fatal("expected error publishing product for unapproved store")
		}
	})

	t.Run("successful publication when prerequisites met", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    validStoreID,
			CategoryID: validCatID,
			SKU:        "VALID-PUB",
			Name:       "Valid Product",
			Price:      100.0,
			MRP:        120.0,
			Status:     "PUBLISHED",
			Images:     []string{"https://img.com/p.jpg"},
		}
		p, err := svc.CreateProduct(merchantCtx, req)
		if err != nil {
			t.Fatalf("expected successful publication, got %v", err)
		}
		if p.Status != model.StatusPublished {
			t.Errorf("expected status PUBLISHED, got %s", p.Status)
		}
	})

	t.Run("reject invalid state transition DRAFT -> PUBLISHED directly on update without pending review", func(t *testing.T) {
		// Create DRAFT
		draftReq := dto.CreateProductRequest{
			StoreID:    validStoreID,
			CategoryID: validCatID,
			SKU:        "DRAFT-PROD",
			Name:       "Draft Product",
			Price:      100.0,
			MRP:        120.0,
			Status:     "DRAFT",
			Images:     []string{"https://img.com/p.jpg"},
		}
		prod, err := svc.CreateProduct(merchantCtx, draftReq)
		if err != nil {
			t.Fatalf("failed to create draft product: %v", err)
		}

		// Try transitioning DRAFT -> PUBLISHED directly
		pubStatus := "PUBLISHED"
		_, err = svc.UpdateProduct(merchantCtx, dto.UpdateProductRequest{
			ID:     prod.ID,
			Status: &pubStatus,
		})
		if err == nil {
			t.Fatal("expected conflict error transitioning DRAFT directly to PUBLISHED")
		}

		// Transition DRAFT -> PENDING_REVIEW -> PUBLISHED
		pendingStatus := "PENDING_REVIEW"
		updated, err := svc.UpdateProduct(merchantCtx, dto.UpdateProductRequest{
			ID:     prod.ID,
			Status: &pendingStatus,
		})
		if err != nil {
			t.Fatalf("expected DRAFT -> PENDING_REVIEW to succeed, got %v", err)
		}
		if updated.Status != model.StatusPendingReview {
			t.Errorf("expected status PENDING_REVIEW, got %s", updated.Status)
		}

		// Transition PENDING_REVIEW -> PUBLISHED
		pub, err := svc.UpdateProduct(merchantCtx, dto.UpdateProductRequest{
			ID:     prod.ID,
			Status: &pubStatus,
		})
		if err != nil {
			t.Fatalf("expected PENDING_REVIEW -> PUBLISHED to succeed, got %v", err)
		}
		if pub.Status != model.StatusPublished {
			t.Errorf("expected status PUBLISHED, got %s", pub.Status)
		}
	})
}

