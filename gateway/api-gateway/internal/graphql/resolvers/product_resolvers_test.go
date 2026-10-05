package resolvers

import (
	"context"
	"testing"

	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	grpcPkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockProductClient struct {
	productpb.ProductServiceClient
	createResp *productpb.CreateProductResponse
	createErr  error
	getResp    *productpb.GetProductResponse
	getErr     error
	listResp   *productpb.ListProductsResponse
	listErr    error
	updateResp *productpb.UpdateProductResponse
	updateErr  error
	deleteResp *productpb.DeleteProductResponse
	deleteErr  error
}

func (m *mockProductClient) CreateProduct(ctx context.Context, in *productpb.CreateProductRequest, opts ...grpcPkg.CallOption) (*productpb.CreateProductResponse, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.createResp, nil
}

func (m *mockProductClient) GetProduct(ctx context.Context, in *productpb.GetProductRequest, opts ...grpcPkg.CallOption) (*productpb.GetProductResponse, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.getResp, nil
}

func (m *mockProductClient) ListProducts(ctx context.Context, in *productpb.ListProductsRequest, opts ...grpcPkg.CallOption) (*productpb.ListProductsResponse, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listResp, nil
}

func (m *mockProductClient) UpdateProduct(ctx context.Context, in *productpb.UpdateProductRequest, opts ...grpcPkg.CallOption) (*productpb.UpdateProductResponse, error) {
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	return m.updateResp, nil
}

func (m *mockProductClient) DeleteProduct(ctx context.Context, in *productpb.DeleteProductRequest, opts ...grpcPkg.CallOption) (*productpb.DeleteProductResponse, error) {
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}
	return m.deleteResp, nil
}

func TestProductResolvers_CreateProduct_Success(t *testing.T) {
	mockClient := &mockProductClient{
		createResp: &productpb.CreateProductResponse{
			Product: &productpb.Product{
				Id:          "prod-1",
				StoreId:     "store-1",
				CategoryId:  "cat-1",
				Sku:         "IPHONE-15",
				Name:        "iPhone 15",
				Productname: "iPhone 15",
				Price:       999.0,
				Mrp:         1099.0,
				Status:      "IN_STOCK",
			},
		},
	}

	clients := &grpc.Clients{ProductClient: mockClient}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}

	res, err := r.CreateProduct(context.Background(), model.CreateProductInput{
		StoreID:    "store-1",
		CategoryID: "cat-1",
		Sku:        "IPHONE-15",
		Name:       "iPhone 15",
		Price:      999.0,
		Mrp:        1099.0,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.ID != "prod-1" || res.Name != "iPhone 15" {
		t.Errorf("unexpected product result: %+v", res)
	}
}

func TestProductResolvers_GetProduct_NotFound(t *testing.T) {
	mockClient := &mockProductClient{
		getErr: status.Error(codes.NotFound, "product not found"),
	}

	clients := &grpc.Clients{ProductClient: mockClient}
	r := &queryResolver{Resolver: &Resolver{Clients: clients}}

	res, err := r.Product(context.Background(), "non-existent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result, got %+v", res)
	}
}

func TestProductResolvers_ListProducts_Success(t *testing.T) {
	mockClient := &mockProductClient{
		listResp: &productpb.ListProductsResponse{
			Products: []*productpb.Product{
				{Id: "prod-1", Name: "iPhone 15", Price: 999.0},
				{Id: "prod-2", Name: "MacBook Pro", Price: 2499.0},
			},
			Total: 2,
		},
	}

	clients := &grpc.Clients{ProductClient: mockClient}
	r := &queryResolver{Resolver: &Resolver{Clients: clients}}

	limit := 10
	offset := 0
	res, err := r.Products(context.Background(), &limit, &offset, nil, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.Total != 2 || len(res.Products) != 2 {
		t.Errorf("unexpected product list: %+v", res)
	}
}

func TestProductResolvers_UpdateProduct_Success(t *testing.T) {
	mockClient := &mockProductClient{
		updateResp: &productpb.UpdateProductResponse{
			Product: &productpb.Product{
				Id:    "prod-1",
				Name:  "iPhone 15 Pro",
				Price: 1099.0,
			},
		},
	}

	clients := &grpc.Clients{ProductClient: mockClient}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}

	newName := "iPhone 15 Pro"
	newPrice := 1099.0
	res, err := r.UpdateProduct(context.Background(), "prod-1", model.UpdateProductInput{
		Name:  &newName,
		Price: &newPrice,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.Name != "iPhone 15 Pro" || res.Price != 1099.0 {
		t.Errorf("expected updated product, got %+v", res)
	}
}

func TestProductResolvers_DeleteProduct_Success(t *testing.T) {
	mockClient := &mockProductClient{
		deleteResp: &productpb.DeleteProductResponse{
			Success: true,
			Message: "Product deleted successfully",
		},
	}

	clients := &grpc.Clients{ProductClient: mockClient}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}

	res, err := r.DeleteProduct(context.Background(), "prod-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !res.Success {
		t.Errorf("expected success true, got %v", res.Success)
	}
}
