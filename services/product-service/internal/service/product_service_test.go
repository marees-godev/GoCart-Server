package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	categorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/category"
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
			validStoreID: {Id: validStoreID, MerchantId: merchantID, Name: "Test Store"},
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
			validStoreID: {Id: validStoreID, MerchantId: merchantID, Name: "Test Store"},
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
