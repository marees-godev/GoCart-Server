package service_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	categorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/category"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	notificationpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/notification"
	storepb "github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/service"
	"google.golang.org/grpc"
)

type mockNotificationClient struct {
	sentRequests []*notificationpb.SendNotificationRequest
	mu           sync.Mutex
}

func (m *mockNotificationClient) SendNotification(ctx context.Context, in *notificationpb.SendNotificationRequest, opts ...grpc.CallOption) (*notificationpb.SendNotificationResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentRequests = append(m.sentRequests, in)
	return &notificationpb.SendNotificationResponse{
		NotificationId: "notif-123",
		Success:        true,
	}, nil
}

type mockRepo struct {
	counter  int64
	products map[string]*model.Product
	skus     map[string]*model.Product // key: storeID + ":" + sku
	reports  map[string]*model.ProductReport
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		products: make(map[string]*model.Product),
		skus:     make(map[string]*model.Product),
		reports:  make(map[string]*model.ProductReport),
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

func (m *mockRepo) CreateProductReport(ctx context.Context, report *model.ProductReport, outboxEvt ...*outbox.Event) error {
	for _, r := range m.reports {
		if r.ProductID == report.ProductID && r.ReporterID == report.ReporterID {
			return appErrors.Conflict("you have already reported this product")
		}
	}
	idNum := atomic.AddInt64(&m.counter, 1)
	report.ID = fmt.Sprintf("44444444-4444-4444-4444-%012d", idNum)
	report.CreatedAt = time.Now()
	report.UpdatedAt = time.Now()
	m.reports[report.ID] = report
	return nil
}

func (m *mockRepo) GetProductReportByID(ctx context.Context, id string) (*model.ProductReport, error) {
	r, ok := m.reports[id]
	if !ok {
		return nil, appErrors.NotFound("product report not found")
	}
	return r, nil
}

func (m *mockRepo) GetProductReportByUserAndProduct(ctx context.Context, productID, reporterID string) (*model.ProductReport, error) {
	for _, r := range m.reports {
		if r.ProductID == productID && r.ReporterID == reporterID {
			return r, nil
		}
	}
	return nil, nil
}

func (m *mockRepo) EnsureProductReportsTable(ctx context.Context) error {
	return nil
}

func (m *mockRepo) ListProductReports(ctx context.Context, req dto.ListProductReportsRequest) ([]*model.ProductReport, int32, error) {
	res := []*model.ProductReport{}
	for _, r := range m.reports {
		if req.ProductID != "" && r.ProductID != req.ProductID {
			continue
		}
		if req.Status != "" && string(r.Status) != req.Status {
			continue
		}
		if len(req.StoreIDs) > 0 {
			prod, ok := m.products[r.ProductID]
			if !ok {
				continue
			}
			matched := false
			for _, sID := range req.StoreIDs {
				if prod.StoreID == sID {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		res = append(res, r)
	}
	return res, int32(len(res)), nil
}

type mockStoreClient struct {
	stores map[string]*storepb.Store
}

func (m *mockStoreClient) GetStore(ctx context.Context, in *storepb.GetStoreRequest, opts ...grpc.CallOption) (*storepb.GetStoreResponse, error) {
	if in.Id != "" {
		st, ok := m.stores[in.Id]
		if !ok {
			return nil, errors.New("store not found")
		}
		return &storepb.GetStoreResponse{Store: st}, nil
	}
	if in.MerchantId != "" {
		for _, st := range m.stores {
			if st.MerchantId == in.MerchantId {
				return &storepb.GetStoreResponse{Store: st}, nil
			}
		}
	}
	return nil, errors.New("store not found")
}

func (m *mockStoreClient) ListStores(ctx context.Context, in *storepb.ListStoresRequest, opts ...grpc.CallOption) (*storepb.ListStoresResponse, error) {
	var matched []*storepb.Store
	for _, s := range m.stores {
		if in.MerchantId == "" || s.MerchantId == in.MerchantId {
			matched = append(matched, s)
		}
	}
	return &storepb.ListStoresResponse{Stores: matched, Total: int32(len(matched))}, nil
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

	t.Run("allow state transition DRAFT -> PUBLISHED directly on update without pending review", func(t *testing.T) {
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

		// Transition DRAFT -> PUBLISHED directly without verification
		pubStatus := "PUBLISHED"
		pub, err := svc.UpdateProduct(merchantCtx, dto.UpdateProductRequest{
			ID:     prod.ID,
			Status: &pubStatus,
		})
		if err != nil {
			t.Fatalf("expected DRAFT -> PUBLISHED to succeed directly without verification, got %v", err)
		}
		if pub.Status != model.StatusPublished {
			t.Errorf("expected status PUBLISHED, got %s", pub.Status)
		}
	})
}

func TestDirectProductPublishingWithoutVerification(t *testing.T) {
	repo := newMockRepo()
	validStoreID := "11111111-1111-1111-1111-111111111111"
	validCategoryID := "22222222-2222-2222-2222-222222222222"
	merchantID := "00000000-0000-0000-0000-000000000001"

	storeClient := &mockStoreClient{
		stores: map[string]*storepb.Store{
			validStoreID: {Id: validStoreID, MerchantId: merchantID, Name: "Test Store", ApprovalStatus: "APPROVED"},
		},
	}
	catClient := &mockCategoryClient{
		categories: map[string]*categorypb.Category{
			validCategoryID: {Id: validCategoryID, Name: "Electronics", IsActive: true},
		},
	}
	merchantClient := &mockMerchantClient{
		merchants: map[string]*merchantpb.MerchantResponseData{
			merchantID: {Id: merchantID, Status: merchantpb.MerchantStatus_APPROVED},
		},
	}
	notifClient := &mockNotificationClient{}

	svc := service.NewProductServiceWithAllClients(repo, storeClient, catClient, merchantClient, notifClient, nil)
	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: merchantID,
		Role:   auth.RoleMerchant,
	})

	t.Run("merchant creates product directly with PUBLISHED status", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    validStoreID,
			CategoryID: validCategoryID,
			SKU:        "DIRECT-PUB-SKU",
			Name:       "Direct Published Product",
			Price:      250.0,
			MRP:        300.0,
			Status:     "PUBLISHED",
			Images:     []string{"https://img.com/p1.jpg"},
		}
		prod, err := svc.CreateProduct(merchantCtx, req)
		if err != nil {
			t.Fatalf("expected direct publishing during creation to succeed, got %v", err)
		}
		if prod.Status != model.StatusPublished {
			t.Errorf("expected status PUBLISHED, got %s", prod.Status)
		}
	})

	t.Run("merchant transitions DRAFT to PUBLISHED directly", func(t *testing.T) {
		req := dto.CreateProductRequest{
			StoreID:    validStoreID,
			CategoryID: validCategoryID,
			SKU:        "DRAFT-TO-PUB-SKU",
			Name:       "Draft To Published",
			Price:      150.0,
			MRP:        200.0,
			Status:     "DRAFT",
			Images:     []string{"https://img.com/p2.jpg"},
		}
		prod, err := svc.CreateProduct(merchantCtx, req)
		if err != nil {
			t.Fatalf("failed to create draft product: %v", err)
		}
		if prod.Status != model.StatusDraft {
			t.Fatalf("expected DRAFT status, got %s", prod.Status)
		}

		pubStatus := "PUBLISHED"
		updated, err := svc.UpdateProduct(merchantCtx, dto.UpdateProductRequest{
			ID:     prod.ID,
			Status: &pubStatus,
		})
		if err != nil {
			t.Fatalf("expected transition from DRAFT to PUBLISHED without admin verification, got %v", err)
		}
		if updated.Status != model.StatusPublished {
			t.Errorf("expected status PUBLISHED, got %s", updated.Status)
		}
	})
}

func TestProductReporting(t *testing.T) {
	repo := newMockRepo()
	validStoreID := "11111111-1111-1111-1111-111111111111"
	validCategoryID := "22222222-2222-2222-2222-222222222222"
	merchantID := "00000000-0000-0000-0000-000000000001"
	customerID := "99999999-9999-9999-9999-999999999999"

	storeClient := &mockStoreClient{
		stores: map[string]*storepb.Store{
			validStoreID: {Id: validStoreID, MerchantId: merchantID, Name: "Test Store", ApprovalStatus: "APPROVED"},
		},
	}
	catClient := &mockCategoryClient{
		categories: map[string]*categorypb.Category{
			validCategoryID: {Id: validCategoryID, Name: "Electronics", IsActive: true},
		},
	}
	merchantClient := &mockMerchantClient{
		merchants: map[string]*merchantpb.MerchantResponseData{
			merchantID: {Id: merchantID, Status: merchantpb.MerchantStatus_APPROVED},
		},
	}
	notifClient := &mockNotificationClient{}

	svc := service.NewProductServiceWithAllClients(repo, storeClient, catClient, merchantClient, notifClient, nil)
	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: merchantID,
		Role:   auth.RoleMerchant,
	})
	customerCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: customerID,
		Role:   auth.RoleCustomer,
	})

	// Create a published product
	pubReq := dto.CreateProductRequest{
		StoreID:    validStoreID,
		CategoryID: validCategoryID,
		SKU:        "REPORTABLE-PROD-SKU",
		Name:       "Reportable Product",
		Price:      100.0,
		MRP:        150.0,
		Status:     "PUBLISHED",
		Images:     []string{"https://img.com/p.jpg"},
	}
	pubProd, err := svc.CreateProduct(merchantCtx, pubReq)
	if err != nil {
		t.Fatalf("failed to create published product: %v", err)
	}

	// Create a draft product
	draftReq := dto.CreateProductRequest{
		StoreID:    validStoreID,
		CategoryID: validCategoryID,
		SKU:        "DRAFT-PROD-SKU",
		Name:       "Draft Product",
		Price:      100.0,
		MRP:        150.0,
		Status:     "DRAFT",
		Images:     []string{"https://img.com/d.jpg"},
	}
	draftProd, err := svc.CreateProduct(merchantCtx, draftReq)
	if err != nil {
		t.Fatalf("failed to create draft product: %v", err)
	}

	t.Run("successfully report published product", func(t *testing.T) {
		reportReq := dto.ReportProductRequest{
			ProductID:   pubProd.ID,
			ReporterID:  customerID,
			Reason:      "INAPPROPRIATE_CONTENT",
			Description: "This product contains inappropriate imagery",
		}
		rep, err := svc.ReportProduct(customerCtx, reportReq)
		if err != nil {
			t.Fatalf("expected successful report, got %v", err)
		}
		if rep.ID == "" {
			t.Errorf("expected report ID to be generated")
		}
		if rep.ProductID != pubProd.ID {
			t.Errorf("expected product ID %s, got %s", pubProd.ID, rep.ProductID)
		}
		if rep.ReporterID != customerID {
			t.Errorf("expected reporter ID %s, got %s", customerID, rep.ReporterID)
		}
		if rep.Reason != model.ReportReasonInappropriateContent {
			t.Errorf("expected reason INAPPROPRIATE_CONTENT, got %s", rep.Reason)
		}
		if rep.Status != model.ReportStatusPending {
			t.Errorf("expected status PENDING, got %s", rep.Status)
		}

		// Wait briefly for notification goroutine to complete
		time.Sleep(50 * time.Millisecond)
		notifClient.mu.Lock()
		sentCount := len(notifClient.sentRequests)
		notifClient.mu.Unlock()
		if sentCount < 2 {
			t.Errorf("expected at least 2 notifications (merchant + reporter), got %d", sentCount)
		}

		// Verify product is NOT automatically removed or unpublished
		p, err := svc.GetProduct(customerCtx, pubProd.ID)
		if err != nil {
			t.Fatalf("expected product to still exist, got %v", err)
		}
		if p.Status != model.StatusPublished {
			t.Errorf("expected product status to remain PUBLISHED, got %s", p.Status)
		}
	})

	t.Run("prevent duplicate report from same user for same product", func(t *testing.T) {
		reportReq := dto.ReportProductRequest{
			ProductID:   pubProd.ID,
			ReporterID:  customerID,
			Reason:      "MISLEADING_INFORMATION",
			Description: "Duplicate report",
		}
		_, err := svc.ReportProduct(customerCtx, reportReq)
		if err == nil {
			t.Fatal("expected conflict error for duplicate report by same user")
		}
	})

	t.Run("fail reporting non-existent product", func(t *testing.T) {
		reportReq := dto.ReportProductRequest{
			ProductID:   "00000000-0000-0000-0000-000000000999",
			ReporterID:  customerID,
			Reason:      "SPAM_OR_DUPLICATE",
			Description: "Non-existent",
		}
		_, err := svc.ReportProduct(customerCtx, reportReq)
		if err == nil {
			t.Fatal("expected not found error for non-existent product")
		}
	})

	t.Run("fail reporting draft product", func(t *testing.T) {
		reportReq := dto.ReportProductRequest{
			ProductID:   draftProd.ID,
			ReporterID:  customerID,
			Reason:      "PROHIBITED_PRODUCT",
			Description: "Draft cannot be reported",
		}
		_, err := svc.ReportProduct(customerCtx, reportReq)
		if err == nil {
			t.Fatal("expected bad request error for reporting draft product")
		}
	})

	t.Run("fail reporting with invalid reason", func(t *testing.T) {
		reportReq := dto.ReportProductRequest{
			ProductID:   pubProd.ID,
			ReporterID:  "88888888-8888-8888-8888-888888888888",
			Reason:      "I_JUST_DONT_LIKE_IT",
			Description: "Invalid reason",
		}
		_, err := svc.ReportProduct(customerCtx, reportReq)
		if err == nil {
			t.Fatal("expected error for invalid report reason")
		}
	})

	t.Run("list product reports for moderation", func(t *testing.T) {
		reports, total, err := svc.ListProductReports(customerCtx, dto.ListProductReportsRequest{
			ProductID: pubProd.ID,
		})
		if err != nil {
			t.Fatalf("failed to list product reports: %v", err)
		}
		if total < 1 || len(reports) < 1 {
			t.Fatalf("expected at least 1 report, got %d", total)
		}
		if reports[0].ID == "" {
			t.Fatal("expected report ID to be populated, got empty")
		}
		if reports[0].CreatedAt.IsZero() {
			t.Fatal("expected report CreatedAt to be non-zero")
		}
	})

	t.Run("merchant scopes reports to own products without productId", func(t *testing.T) {
		ownerCtx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: merchantID,
			Role:   auth.RoleMerchant,
		})
		reports, total, err := svc.ListProductReports(ownerCtx, dto.ListProductReportsRequest{})
		if err != nil {
			t.Fatalf("failed to list product reports for merchant: %v", err)
		}
		if total < 1 || len(reports) < 1 {
			t.Fatalf("expected at least 1 report for owner merchant, got %d", total)
		}
	})

	t.Run("merchant cannot access other merchant product reports with productId", func(t *testing.T) {
		otherMerchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: "other-merchant-999",
			Role:   auth.RoleMerchant,
		})
		_, _, err := svc.ListProductReports(otherMerchantCtx, dto.ListProductReportsRequest{
			ProductID: pubProd.ID,
		})
		if err == nil {
			t.Fatal("expected forbidden error for other merchant querying product reports")
		}
	})

	t.Run("merchant with no stores receives empty report list", func(t *testing.T) {
		emptyMerchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: "storeless-merchant-999",
			Role:   auth.RoleMerchant,
		})
		reports, total, err := svc.ListProductReports(emptyMerchantCtx, dto.ListProductReportsRequest{})
		if err != nil {
			t.Fatalf("expected success for merchant with no stores, got error: %v", err)
		}
		if total != 0 || len(reports) != 0 {
			t.Fatalf("expected 0 reports for merchant with no stores, got %d", total)
		}
	})
}

type mockProductUploader struct {
	failUpload bool
}

func (m *mockProductUploader) Upload(ctx context.Context, key string, body io.Reader, contentType string) (string, error) {
	if m.failUpload {
		return "", errors.New("supabase storage upload failed")
	}
	return "https://storage.supabase.co/storage/v1/object/public/products/" + key, nil
}

func (m *mockProductUploader) UploadBytes(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if m.failUpload {
		return "", errors.New("supabase storage upload failed")
	}
	return "https://storage.supabase.co/storage/v1/object/public/products/" + key, nil
}

func (m *mockProductUploader) Delete(ctx context.Context, key string) error {
	return nil
}

func (m *mockProductUploader) GetPublicURL(key string) string {
	return "https://storage.supabase.co/storage/v1/object/public/products/" + key
}

func (m *mockProductUploader) GenerateKey(prefix, filename string) string {
	return prefix + "/" + filename
}

func (m *mockProductUploader) GetPresignedPutURL(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error) {
	return "", nil
}

func TestProductService_ImageUpload_StorageUnavailable(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	validStoreID := "11111111-1111-1111-1111-111111111111"
	validCatID := "22222222-2222-2222-2222-222222222222"
	storeClient := &mockStoreClient{
		stores: map[string]*storepb.Store{
			validStoreID: {Id: validStoreID, MerchantId: "merchant-1", Name: "Test Store", ApprovalStatus: "APPROVED"},
		},
	}
	catClient := &mockCategoryClient{
		categories: map[string]*categorypb.Category{
			validCatID: {Id: validCatID, Name: "Electronics", IsActive: true},
		},
	}
	svc := service.NewProductService(repo, storeClient, catClient, nil)

	merchantCtx := auth.WithUser(ctx, &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	base64Img := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	_, err := svc.CreateProduct(merchantCtx, dto.CreateProductRequest{
		StoreID:    validStoreID,
		CategoryID: validCatID,
		SKU:        "SKU-TEST-1",
		Name:       "Test Product",
		Price:      100,
		MRP:        120,
		ImageURL:   base64Img,
	})
	if err == nil {
		t.Fatal("expected error when storage uploader is unavailable, got nil")
	}
	if !strings.Contains(err.Error(), "storage service is not configured or unavailable") {
		t.Fatalf("expected storage unavailable error, got: %v", err)
	}
}

func TestProductService_ImageUpload_UploadFailure(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	validStoreID := "11111111-1111-1111-1111-111111111111"
	validCatID := "22222222-2222-2222-2222-222222222222"
	storeClient := &mockStoreClient{
		stores: map[string]*storepb.Store{
			validStoreID: {Id: validStoreID, MerchantId: "merchant-1", Name: "Test Store", ApprovalStatus: "APPROVED"},
		},
	}
	catClient := &mockCategoryClient{
		categories: map[string]*categorypb.Category{
			validCatID: {Id: validCatID, Name: "Electronics", IsActive: true},
		},
	}
	failingUploader := &mockProductUploader{failUpload: true}
	svc := service.NewProductService(repo, storeClient, catClient, failingUploader)

	merchantCtx := auth.WithUser(ctx, &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	base64Img := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	_, err := svc.CreateProduct(merchantCtx, dto.CreateProductRequest{
		StoreID:    validStoreID,
		CategoryID: validCatID,
		SKU:        "SKU-TEST-2",
		Name:       "Test Product 2",
		Price:      100,
		MRP:        120,
		ImageURL:   base64Img,
	})
	if err == nil {
		t.Fatal("expected error when storage upload fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to upload image to storage") {
		t.Fatalf("expected failed to upload image error, got: %v", err)
	}

	// Verify no product was persisted with base64 url
	p, err := repo.GetProductBySKU(ctx, validStoreID, "SKU-TEST-2")
	if err == nil && p != nil {
		t.Fatalf("product should not have been saved on upload failure")
	}
}

func TestProductService_ImageUpload_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	validStoreID := "11111111-1111-1111-1111-111111111111"
	validCatID := "22222222-2222-2222-2222-222222222222"
	storeClient := &mockStoreClient{
		stores: map[string]*storepb.Store{
			validStoreID: {Id: validStoreID, MerchantId: "merchant-1", Name: "Test Store", ApprovalStatus: "APPROVED"},
		},
	}
	catClient := &mockCategoryClient{
		categories: map[string]*categorypb.Category{
			validCatID: {Id: validCatID, Name: "Electronics", IsActive: true},
		},
	}
	uploader := &mockProductUploader{failUpload: false}
	svc := service.NewProductService(repo, storeClient, catClient, uploader)

	merchantCtx := auth.WithUser(ctx, &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	base64Img := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	prod, err := svc.CreateProduct(merchantCtx, dto.CreateProductRequest{
		StoreID:    validStoreID,
		CategoryID: validCatID,
		SKU:        "SKU-TEST-3",
		Name:       "Test Product 3",
		Price:      100,
		MRP:        120,
		ImageURL:   base64Img,
		Images:     []string{base64Img},
	})
	if err != nil {
		t.Fatalf("expected successful product creation, got: %v", err)
	}
	if strings.HasPrefix(prod.ImageURL, "data:") {
		t.Fatalf("expected Supabase Storage URL, got base64 data URL: %s", prod.ImageURL)
	}
	if !strings.HasPrefix(prod.ImageURL, "https://storage.supabase.co/") {
		t.Fatalf("expected Supabase Storage URL prefix, got: %s", prod.ImageURL)
	}
	if len(prod.Images) > 0 && strings.HasPrefix(prod.Images[0].URL, "data:") {
		t.Fatalf("expected product gallery image to have Supabase Storage URL, got base64: %s", prod.Images[0].URL)
	}
}

func TestProductService_UpdateProduct_ImageUploadFailuresAndSuccess(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	validStoreID := "11111111-1111-1111-1111-111111111111"
	validCatID := "22222222-2222-2222-2222-222222222222"
	storeClient := &mockStoreClient{
		stores: map[string]*storepb.Store{
			validStoreID: {Id: validStoreID, MerchantId: "merchant-1", Name: "Test Store", ApprovalStatus: "APPROVED"},
		},
	}
	catClient := &mockCategoryClient{
		categories: map[string]*categorypb.Category{
			validCatID: {Id: validCatID, Name: "Electronics", IsActive: true},
		},
	}
	uploader := &mockProductUploader{failUpload: false}
	svc := service.NewProductService(repo, storeClient, catClient, uploader)

	merchantCtx := auth.WithUser(ctx, &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	prod, err := svc.CreateProduct(merchantCtx, dto.CreateProductRequest{
		StoreID:    validStoreID,
		CategoryID: validCatID,
		SKU:        "SKU-UPDATE-1",
		Name:       "Initial Product",
		Price:      100,
		MRP:        120,
		ImageURL:   "https://existing.cdn.com/product.png",
	})
	if err != nil {
		t.Fatalf("failed to create initial product: %v", err)
	}

	base64Img := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	// Test upload failure
	uploader.failUpload = true
	_, err = svc.UpdateProduct(merchantCtx, dto.UpdateProductRequest{
		ID:       prod.ID,
		ImageURL: &base64Img,
	})
	if err == nil {
		t.Fatal("expected error on update when upload fails, got nil")
	}

	// Verify original URL is preserved and no base64 was saved
	fetched, err := repo.GetProductByID(ctx, prod.ID)
	if err != nil {
		t.Fatalf("failed to get product: %v", err)
	}
	if fetched.ImageURL != "https://existing.cdn.com/product.png" {
		t.Fatalf("expected existing URL preserved, got: %s", fetched.ImageURL)
	}

	// Test upload success
	uploader.failUpload = false
	updated, err := svc.UpdateProduct(merchantCtx, dto.UpdateProductRequest{
		ID:       prod.ID,
		ImageURL: &base64Img,
	})
	if err != nil {
		t.Fatalf("expected successful update, got: %v", err)
	}
	if strings.HasPrefix(updated.ImageURL, "data:") {
		t.Fatalf("expected storage URL, got base64 data URL: %s", updated.ImageURL)
	}
	if !strings.HasPrefix(updated.ImageURL, "https://storage.supabase.co/") {
		t.Fatalf("expected Supabase Storage URL prefix, got: %s", updated.ImageURL)
	}
}

func TestProductService_ImageUpload_Formats(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	validStoreID := "11111111-1111-1111-1111-111111111111"
	validCatID := "22222222-2222-2222-2222-222222222222"
	storeClient := &mockStoreClient{
		stores: map[string]*storepb.Store{
			validStoreID: {Id: validStoreID, MerchantId: "merchant-1", Name: "Test Store", ApprovalStatus: "APPROVED"},
		},
	}
	catClient := &mockCategoryClient{
		categories: map[string]*categorypb.Category{
			validCatID: {Id: validCatID, Name: "Electronics", IsActive: true},
		},
	}
	uploader := &mockProductUploader{failUpload: false}
	svc := service.NewProductService(repo, storeClient, catClient, uploader)

	merchantCtx := auth.WithUser(ctx, &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	formats := []struct {
		name    string
		dataURL string
	}{
		{"PNG", "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="},
		{"JPEG", "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEASABIAAD/2wBDAP//////////////////////////////////////////////////////////////////////////////////////wgALCAABAAEBAREA/8QAFBABAAAAAAAAAAAAAAAAAAAAAP/aAAgBAQABPxA="},
		{"WebP", "data:image/webp;base64,UklGRh4AAABXRUJQVlA4TBEAAAAvAAAAAAfQ//73v/+BiOh/AAA="},
	}

	for _, tc := range formats {
		t.Run(tc.name, func(t *testing.T) {
			prod, err := svc.CreateProduct(merchantCtx, dto.CreateProductRequest{
				StoreID:    validStoreID,
				CategoryID: validCatID,
				SKU:        "SKU-FMT-" + tc.name,
				Name:       "Test Product " + tc.name,
				Price:      100,
				MRP:        120,
				ImageURL:   tc.dataURL,
			})
			if err != nil {
				t.Fatalf("expected successful image processing for %s, got: %v", tc.name, err)
			}
			if !strings.HasPrefix(prod.ImageURL, "https://storage.supabase.co/") {
				t.Errorf("expected Supabase storage URL prefix, got: %s", prod.ImageURL)
			}
			if strings.HasPrefix(prod.ImageURL, "data:") {
				t.Errorf("expected image URL to not be data URI, got: %s", prod.ImageURL)
			}
		})
	}
}

func TestProductService_ImageUpload_InvalidData(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	validStoreID := "11111111-1111-1111-1111-111111111111"
	validCatID := "22222222-2222-2222-2222-222222222222"
	storeClient := &mockStoreClient{
		stores: map[string]*storepb.Store{
			validStoreID: {Id: validStoreID, MerchantId: "merchant-1", Name: "Test Store", ApprovalStatus: "APPROVED"},
		},
	}
	catClient := &mockCategoryClient{
		categories: map[string]*categorypb.Category{
			validCatID: {Id: validCatID, Name: "Electronics", IsActive: true},
		},
	}
	uploader := &mockProductUploader{failUpload: false}
	svc := service.NewProductService(repo, storeClient, catClient, uploader)

	merchantCtx := auth.WithUser(ctx, &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	cases := []struct {
		name        string
		dataURL     string
		expectedErr string
	}{
		{"InvalidBase64Encoding", "data:image/png;base64,invalid!not-base64!***", "failed to decode base64 image data"},
		{"UnsupportedGIF", "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7", "unsupported image type"},
		{"MalformedDataURI", "data:image/png,notbase64", "invalid image data URL format"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateProduct(merchantCtx, dto.CreateProductRequest{
				StoreID:    validStoreID,
				CategoryID: validCatID,
				SKU:        "SKU-INV-" + tc.name,
				Name:       "Invalid Product",
				Price:      100,
				MRP:        120,
				ImageURL:   tc.dataURL,
			})
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.expectedErr) {
				t.Errorf("expected error containing '%s', got: %v", tc.expectedErr, err)
			}
		})
	}
}

func TestProductService_ImageUpload_NormalURLPreserved(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	validStoreID := "11111111-1111-1111-1111-111111111111"
	validCatID := "22222222-2222-2222-2222-222222222222"
	storeClient := &mockStoreClient{
		stores: map[string]*storepb.Store{
			validStoreID: {Id: validStoreID, MerchantId: "merchant-1", Name: "Test Store", ApprovalStatus: "APPROVED"},
		},
	}
	catClient := &mockCategoryClient{
		categories: map[string]*categorypb.Category{
			validCatID: {Id: validCatID, Name: "Electronics", IsActive: true},
		},
	}
	// uploader is nil: normal URL should not need uploader
	svc := service.NewProductService(repo, storeClient, catClient, nil)

	merchantCtx := auth.WithUser(ctx, &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	normalURL := "https://cdn.example.com/products/headphones.jpg"
	prod, err := svc.CreateProduct(merchantCtx, dto.CreateProductRequest{
		StoreID:    validStoreID,
		CategoryID: validCatID,
		SKU:        "SKU-NORMAL-URL",
		Name:       "Normal URL Product",
		Price:      100,
		MRP:        120,
		ImageURL:   normalURL,
		Images:     []string{normalURL},
	})
	if err != nil {
		t.Fatalf("expected success with normal URL, got: %v", err)
	}
	if prod.ImageURL != normalURL {
		t.Errorf("expected %s, got: %s", normalURL, prod.ImageURL)
	}
	if len(prod.Images) == 0 || prod.Images[0].URL != normalURL {
		t.Errorf("expected gallery image URL to be preserved, got: %+v", prod.Images)
	}
}
