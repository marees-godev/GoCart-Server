package resolvers

import (
	"context"
	"testing"

	categorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/category"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	grpcPkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockCategoryClient struct {
	categorypb.CategoryServiceClient
	createResp   *categorypb.CreateCategoryResponse
	createErr    error
	getResp      *categorypb.GetCategoryResponse
	getErr       error
	listResp     *categorypb.ListCategoriesResponse
	listErr      error
	childrenResp *categorypb.GetChildCategoriesResponse
	childrenErr  error
	updateResp   *categorypb.UpdateCategoryResponse
	updateErr    error
	deleteResp   *categorypb.DeleteCategoryResponse
	deleteErr    error
	validateResp *categorypb.ValidateCategoryForAssignmentResponse
	validateErr  error
}

func (m *mockCategoryClient) CreateCategory(ctx context.Context, in *categorypb.CreateCategoryRequest, opts ...grpcPkg.CallOption) (*categorypb.CreateCategoryResponse, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.createResp, nil
}

func (m *mockCategoryClient) GetCategory(ctx context.Context, in *categorypb.GetCategoryRequest, opts ...grpcPkg.CallOption) (*categorypb.GetCategoryResponse, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.getResp, nil
}

func (m *mockCategoryClient) ListCategories(ctx context.Context, in *categorypb.ListCategoriesRequest, opts ...grpcPkg.CallOption) (*categorypb.ListCategoriesResponse, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listResp, nil
}

func (m *mockCategoryClient) GetChildCategories(ctx context.Context, in *categorypb.GetChildCategoriesRequest, opts ...grpcPkg.CallOption) (*categorypb.GetChildCategoriesResponse, error) {
	if m.childrenErr != nil {
		return nil, m.childrenErr
	}
	return m.childrenResp, nil
}

func (m *mockCategoryClient) UpdateCategory(ctx context.Context, in *categorypb.UpdateCategoryRequest, opts ...grpcPkg.CallOption) (*categorypb.UpdateCategoryResponse, error) {
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	return m.updateResp, nil
}

func (m *mockCategoryClient) DeleteCategory(ctx context.Context, in *categorypb.DeleteCategoryRequest, opts ...grpcPkg.CallOption) (*categorypb.DeleteCategoryResponse, error) {
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}
	return m.deleteResp, nil
}

func (m *mockCategoryClient) ValidateCategoryForAssignment(ctx context.Context, in *categorypb.ValidateCategoryForAssignmentRequest, opts ...grpcPkg.CallOption) (*categorypb.ValidateCategoryForAssignmentResponse, error) {
	if m.validateErr != nil {
		return nil, m.validateErr
	}
	return m.validateResp, nil
}

func TestCategoryResolvers_CreateCategory_Success(t *testing.T) {
	mockClient := &mockCategoryClient{
		createResp: &categorypb.CreateCategoryResponse{
			Category: &categorypb.Category{
				Id:               "cat-1",
				Name:             "Electronics",
				ParentCategoryId: "parent-1",
				Description:      "Electronic goods",
				IsActive:         true,
			},
		},
	}

	clients := &grpc.Clients{CategoryClient: mockClient}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}

	pID := "parent-1"
	desc := "Electronic goods"
	res, err := r.CreateCategory(context.Background(), model.CreateCategoryInput{
		Name:             "Electronics",
		ParentCategoryID: &pID,
		Description:      &desc,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.ID != "cat-1" || res.Name != "Electronics" {
		t.Errorf("unexpected category result: %+v", res)
	}
	if res.ParentCategoryID == nil || *res.ParentCategoryID != "parent-1" {
		t.Errorf("expected parent category id parent-1, got %v", res.ParentCategoryID)
	}
}

func TestCategoryResolvers_GetCategory_NotFound(t *testing.T) {
	mockClient := &mockCategoryClient{
		getErr: status.Error(codes.NotFound, "category not found"),
	}

	clients := &grpc.Clients{CategoryClient: mockClient}
	r := &queryResolver{Resolver: &Resolver{Clients: clients}}

	res, err := r.Category(context.Background(), "non-existent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result, got %+v", res)
	}
}

func TestCategoryResolvers_ListCategories_Success(t *testing.T) {
	mockClient := &mockCategoryClient{

		listResp: &categorypb.ListCategoriesResponse{
			Categories: []*categorypb.Category{
				{Id: "cat-1", Name: "Electronics", IsActive: true},
				{Id: "cat-2", Name: "Fashion", IsActive: true},
			},
			Total: 2,
		},
	}

	clients := &grpc.Clients{CategoryClient: mockClient}
	r := &queryResolver{Resolver: &Resolver{Clients: clients}}

	limit := 10
	offset := 0
	res, err := r.Categories(context.Background(), &limit, &offset, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.Total != 2 || len(res.Categories) != 2 {
		t.Errorf("unexpected categories list: %+v", res)
	}
}

func TestCategoryResolvers_ChildCategories_Success(t *testing.T) {
	mockClient := &mockCategoryClient{
		childrenResp: &categorypb.GetChildCategoriesResponse{
			Categories: []*categorypb.Category{
				{Id: "cat-sub-1", Name: "Laptops", ParentCategoryId: "cat-1", IsActive: true},
			},
			Total: 1,
		},
	}

	clients := &grpc.Clients{CategoryClient: mockClient}
	r := &queryResolver{Resolver: &Resolver{Clients: clients}}

	res, err := r.ChildCategories(context.Background(), "cat-1", nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.Total != 1 || len(res.Categories) != 1 {
		t.Errorf("unexpected child categories: %+v", res)
	}
}

func TestCategoryResolvers_UpdateCategory_Success(t *testing.T) {
	mockClient := &mockCategoryClient{
		updateResp: &categorypb.UpdateCategoryResponse{
			Category: &categorypb.Category{
				Id:       "cat-1",
				Name:     "Consumer Electronics",
				IsActive: true,
			},
		},
	}

	clients := &grpc.Clients{CategoryClient: mockClient}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}

	newName := "Consumer Electronics"
	res, err := r.UpdateCategory(context.Background(), "cat-1", model.UpdateCategoryInput{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.Name != "Consumer Electronics" {
		t.Errorf("expected updated name, got %s", res.Name)
	}
}

func TestCategoryResolvers_DeleteCategory_Success(t *testing.T) {
	mockClient := &mockCategoryClient{
		deleteResp: &categorypb.DeleteCategoryResponse{
			Success: true,
			Message: "Category deleted successfully",
		},
	}

	clients := &grpc.Clients{CategoryClient: mockClient}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}

	res, err := r.DeleteCategory(context.Background(), "cat-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !res.Success {
		t.Errorf("expected success true, got %v", res.Success)
	}
}

func TestCategoryResolvers_ValidateCategoryForAssignment_Success(t *testing.T) {
	mockClient := &mockCategoryClient{
		validateResp: &categorypb.ValidateCategoryForAssignmentResponse{
			IsValid: true,
			Message: "category is active and assignable",
			Category: &categorypb.Category{
				Id:       "cat-1",
				Name:     "Electronics",
				IsActive: true,
			},
		},
	}

	clients := &grpc.Clients{CategoryClient: mockClient}
	r := &queryResolver{Resolver: &Resolver{Clients: clients}}

	res, err := r.ValidateCategoryForAssignment(context.Background(), "cat-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !res.IsValid {
		t.Errorf("expected IsValid true")
	}
	if res.Category.ID != "cat-1" {
		t.Errorf("expected category ID cat-1, got %s", res.Category.ID)
	}
}
