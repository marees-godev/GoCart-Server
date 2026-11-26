package resolvers

import (
	"context"
	"testing"

	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
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
	reportResp *productpb.ReportProductResponse
	reportErr  error
	listRepResp *productpb.ListProductReportsResponse
	listRepErr  error
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

func (m *mockProductClient) ReportProduct(ctx context.Context, in *productpb.ReportProductRequest, opts ...grpcPkg.CallOption) (*productpb.ReportProductResponse, error) {
	if m.reportErr != nil {
		return nil, m.reportErr
	}
	return m.reportResp, nil
}

func (m *mockProductClient) ListProductReports(ctx context.Context, in *productpb.ListProductReportsRequest, opts ...grpcPkg.CallOption) (*productpb.ListProductReportsResponse, error) {
	if m.listRepErr != nil {
		return nil, m.listRepErr
	}
	return m.listRepResp, nil
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

func TestProductResolvers_PublishWithoutVerification(t *testing.T) {
	mockClient := &mockProductClient{
		createResp: &productpb.CreateProductResponse{
			Product: &productpb.Product{
				Id:     "prod-pub-1",
				Name:   "Direct Published",
				Price:  100.0,
				Status: "PUBLISHED",
			},
		},
		updateResp: &productpb.UpdateProductResponse{
			Product: &productpb.Product{
				Id:     "prod-pub-1",
				Name:   "Direct Published",
				Price:  100.0,
				Status: "PUBLISHED",
			},
		},
	}

	clients := &grpc.Clients{ProductClient: mockClient}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}

	pubStatus := model.ProductStatusPublished
	createRes, err := r.CreateProduct(context.Background(), model.CreateProductInput{
		StoreID:    "store-1",
		CategoryID: "cat-1",
		Sku:        "DIRECT-PUB",
		Name:       "Direct Published",
		Price:      100.0,
		Mrp:        120.0,
		Status:     &pubStatus,
	})
	if err != nil {
		t.Fatalf("expected direct publishing during creation without verification, got %v", err)
	}
	if createRes.Status != model.ProductStatusPublished {
		t.Errorf("expected PUBLISHED status, got %s", createRes.Status)
	}

	updateRes, err := r.UpdateProduct(context.Background(), "prod-pub-1", model.UpdateProductInput{
		Status: &pubStatus,
	})
	if err != nil {
		t.Fatalf("expected direct update to PUBLISHED without verification, got %v", err)
	}
	if updateRes.Status != model.ProductStatusPublished {
		t.Errorf("expected PUBLISHED status, got %s", updateRes.Status)
	}
}

func TestProductResolvers_ReportProduct_Success(t *testing.T) {
	mockClient := &mockProductClient{
		reportResp: &productpb.ReportProductResponse{
			Success: true,
			Message: "Product reported successfully",
			Report: &productpb.ProductReport{
				Id:          "rep-1",
				ProductId:   "prod-1",
				ReporterId:  "user-123",
				Reason:      "INAPPROPRIATE_CONTENT",
				Description: "Inappropriate image",
				Status:      "PENDING",
			},
		},
	}

	clients := &grpc.Clients{ProductClient: mockClient}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleCustomer,
	})

	desc := "Inappropriate image"
	res, err := r.ReportProduct(ctx, model.ReportProductInput{
		ProductID:   "prod-1",
		Reason:      model.ProductReportReasonInappropriateContent,
		Description: &desc,
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !res.Success || res.Report == nil {
		t.Fatalf("expected report response, got %+v", res)
	}
	if res.Report.ID != "rep-1" || res.Report.Reason != model.ProductReportReasonInappropriateContent {
		t.Errorf("unexpected report details: %+v", res.Report)
	}
}

func TestProductResolvers_ReportProduct_Unauthenticated(t *testing.T) {
	clients := &grpc.Clients{ProductClient: &mockProductClient{}}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}

	_, err := r.ReportProduct(context.Background(), model.ReportProductInput{
		ProductID: "prod-1",
		Reason:    model.ProductReportReasonInappropriateContent,
	})
	if err == nil {
		t.Fatal("expected unauthorized error for unauthenticated user, got nil")
	}
}

func TestProductResolvers_ProductReports_Success(t *testing.T) {
	mockClient := &mockProductClient{
		listRepResp: &productpb.ListProductReportsResponse{
			Reports: []*productpb.ProductReport{
				{
					Id:         "rep-1",
					ProductId:  "prod-1",
					ReporterId: "user-123",
					Reason:     "INAPPROPRIATE_CONTENT",
					Status:     "PENDING",
				},
			},
			Total: 1,
		},
	}

	clients := &grpc.Clients{ProductClient: mockClient}
	r := &queryResolver{Resolver: &Resolver{Clients: clients}}

	prodID := "prod-1"
	limit := 10
	offset := 0
	res, err := r.ProductReports(context.Background(), &prodID, &limit, &offset, nil)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if res.Total != 1 || len(res.Reports) != 1 {
		t.Errorf("unexpected reports count: %d", res.Total)
	}
	if res.Reports[0].ID != "rep-1" {
		t.Errorf("expected report ID rep-1, got %s", res.Reports[0].ID)
	}
}
