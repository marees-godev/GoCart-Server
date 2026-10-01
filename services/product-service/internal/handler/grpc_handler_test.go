package handler_test

import (
	"context"
	"testing"

	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/model"
)

type mockProductService struct {
	products map[string]*model.Product
}

func newMockProductService() *mockProductService {
	return &mockProductService{
		products: make(map[string]*model.Product),
	}
}

func (m *mockProductService) CreateProduct(ctx context.Context, req dto.CreateProductRequest) (*model.Product, error) {
	if req.Name == "" {
		return nil, appErrors.InvalidArgument("product name is required")
	}
	p := &model.Product{
		ID:          "33333333-3333-3333-3333-333333333333",
		StoreID:     req.StoreID,
		CategoryID:  req.CategoryID,
		SKU:         req.SKU,
		Name:        req.Name,
		Description: req.Description,
		Price:       req.Price,
		MRP:         req.MRP,
		Status:      model.StatusStockIn,
	}
	m.products[p.ID] = p
	return p, nil
}

func (m *mockProductService) GetProduct(ctx context.Context, id string) (*model.Product, error) {
	p, ok := m.products[id]
	if !ok {
		return nil, appErrors.NotFound("product not found")
	}
	return p, nil
}

func (m *mockProductService) ListProducts(ctx context.Context, req dto.ListProductsRequest) ([]*model.Product, int32, error) {
	res := []*model.Product{}
	for _, p := range m.products {
		res = append(res, p)
	}
	return res, int32(len(res)), nil
}

func (m *mockProductService) UpdateProduct(ctx context.Context, req dto.UpdateProductRequest) (*model.Product, error) {
	p, ok := m.products[req.ID]
	if !ok {
		return nil, appErrors.NotFound("product not found")
	}
	if req.Name != nil {
		p.Name = *req.Name
	}
	return p, nil
}

func (m *mockProductService) DeleteProduct(ctx context.Context, id string) error {
	_, ok := m.products[id]
	if !ok {
		return appErrors.NotFound("product not found")
	}
	delete(m.products, id)
	return nil
}

func TestProductGRPCHandler(t *testing.T) {
	svc := newMockProductService()
	h := handler.NewProductGRPCHandler(svc)
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-123",
		Role:   auth.RoleMerchant,
	})

	t.Run("CreateProduct RPC", func(t *testing.T) {
		req := &productpb.CreateProductRequest{
			StoreId:    "11111111-1111-1111-1111-111111111111",
			CategoryId: "22222222-2222-2222-2222-222222222222",
			Sku:        "TEST-SKU",
			Name:       "Test Product",
			Price:      100.0,
			Mrp:        120.0,
		}

		resp, err := h.CreateProduct(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Product == nil || resp.Product.Id != "33333333-3333-3333-3333-333333333333" {
			t.Errorf("expected product ID 33333333-3333-3333-3333-333333333333, got %v", resp.Product)
		}
	})

	t.Run("GetProduct RPC", func(t *testing.T) {
		resp, err := h.GetProduct(ctx, &productpb.GetProductRequest{Id: "33333333-3333-3333-3333-333333333333"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Product.Name != "Test Product" {
			t.Errorf("expected Test Product, got %s", resp.Product.Name)
		}
	})

	t.Run("DeleteProduct RPC", func(t *testing.T) {
		resp, err := h.DeleteProduct(ctx, &productpb.DeleteProductRequest{Id: "33333333-3333-3333-3333-333333333333"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !resp.Success {
			t.Errorf("expected success true")
		}
	})
}
