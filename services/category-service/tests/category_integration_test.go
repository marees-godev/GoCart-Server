package tests_test

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	categorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/category"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type inMemoryCategoryRepo struct {
	categories map[string]*model.Category
	counter    int
}

func newInMemoryRepo() *inMemoryCategoryRepo {
	return &inMemoryCategoryRepo{
		categories: make(map[string]*model.Category),
	}
}

func (m *inMemoryCategoryRepo) CreateCategory(ctx context.Context, cat *model.Category) error {
	if cat.ID == "" {
		m.counter++
		cat.ID = fmt.Sprintf("cat-%d", m.counter)
	}
	cat.CreatedAt = time.Now()
	cat.UpdatedAt = time.Now()
	cp := *cat
	m.categories[cat.ID] = &cp
	return nil
}

func (m *inMemoryCategoryRepo) GetByID(ctx context.Context, id string) (*model.Category, error) {
	cat, ok := m.categories[id]
	if !ok || cat.DeletedAt != nil {
		return nil, appErrors.NotFound("category not found")
	}
	cp := *cat
	return &cp, nil
}

func (m *inMemoryCategoryRepo) GetByNameAndParent(ctx context.Context, name string, parentCategoryID *string) (*model.Category, error) {
	for _, c := range m.categories {
		if c.DeletedAt != nil {
			continue
		}
		if strings.EqualFold(c.Name, name) {
			if parentCategoryID == nil && c.ParentCategoryID == nil {
				cp := *c
				return &cp, nil
			}
			if parentCategoryID != nil && c.ParentCategoryID != nil && *parentCategoryID == *c.ParentCategoryID {
				cp := *c
				return &cp, nil
			}
		}
	}
	return nil, appErrors.NotFound("category not found")
}

func (m *inMemoryCategoryRepo) ListCategory(ctx context.Context, filter dto.ListCategoriesRequest) ([]*model.Category, int32, error) {
	var result []*model.Category
	for _, c := range m.categories {
		if c.DeletedAt != nil {
			continue
		}
		if filter.RootOnly && c.ParentCategoryID != nil {
			continue
		}
		if filter.ParentCategoryID != nil {
			if c.ParentCategoryID == nil || *c.ParentCategoryID != *filter.ParentCategoryID {
				continue
			}
		}
		cp := *c
		result = append(result, &cp)
	}

	total := int32(len(result))
	start := filter.Offset
	if start > total {
		start = total
	}
	end := start + filter.Limit
	if filter.Limit <= 0 || end > total {
		end = total
	}

	return result[start:end], total, nil
}

func (m *inMemoryCategoryRepo) GetChildren(ctx context.Context, parentCategoryID string, limit, offset int32) ([]*model.Category, int32, error) {
	var children []*model.Category
	for _, c := range m.categories {
		if c.DeletedAt != nil {
			continue
		}
		if c.ParentCategoryID != nil && *c.ParentCategoryID == parentCategoryID {
			cp := *c
			children = append(children, &cp)
		}
	}

	total := int32(len(children))
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if limit <= 0 || end > total {
		end = total
	}

	return children[start:end], total, nil
}

func (m *inMemoryCategoryRepo) UpdateCategory(ctx context.Context, cat *model.Category) error {
	existing, ok := m.categories[cat.ID]
	if !ok || existing.DeletedAt != nil {
		return appErrors.NotFound("category not found")
	}
	cat.UpdatedAt = time.Now()
	cp := *cat
	m.categories[cat.ID] = &cp
	return nil
}

func (m *inMemoryCategoryRepo) DeleteCategory(ctx context.Context, id string) error {
	cat, ok := m.categories[id]
	if !ok || cat.DeletedAt != nil {
		return appErrors.NotFound("category not found")
	}
	now := time.Now()
	cat.DeletedAt = &now
	cat.IsActive = false
	cat.UpdatedAt = now
	return nil
}

func (m *inMemoryCategoryRepo) HasChildren(ctx context.Context, parentCategoryID string) (bool, error) {
	for _, c := range m.categories {
		if c.DeletedAt == nil && c.ParentCategoryID != nil && *c.ParentCategoryID == parentCategoryID {
			return true, nil
		}
	}
	return false, nil
}

func (m *inMemoryCategoryRepo) IsDescendant(ctx context.Context, candidateDescendantID, ancestorID string) (bool, error) {
	currID := candidateDescendantID
	for currID != "" {
		if currID == ancestorID {
			return true, nil
		}
		cat, ok := m.categories[currID]
		if !ok || cat.DeletedAt != nil || cat.ParentCategoryID == nil {
			break
		}
		currID = *cat.ParentCategoryID
	}
	return false, nil
}

func setupTestGRPCServer(t *testing.T) (categorypb.CategoryServiceClient, func()) {
	repo := newInMemoryRepo()
	svc := service.NewCategoryService(repo)
	h := handler.NewCategoryGRPCHandler(svc)

	categoryMethodRoles := map[string][]string{
		"/gocart.category.v1.CategoryService/CreateCategory": {auth.RoleAdmin},
		"/gocart.category.v1.CategoryService/UpdateCategory": {auth.RoleAdmin},
		"/gocart.category.v1.CategoryService/DeleteCategory": {auth.RoleAdmin},
	}

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpcclient.UnaryServerInterceptor(),
			grpcclient.UnaryRoleAuthInterceptor(categoryMethodRoles),
		),
	)
	categorypb.RegisterCategoryServiceServer(server, h)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	go func() {
		_ = server.Serve(lis)
	}()

	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	client := categorypb.NewCategoryServiceClient(conn)
	cleanup := func() {
		_ = conn.Close()
		server.Stop()
		_ = lis.Close()
	}

	return client, cleanup
}

func withRole(ctx context.Context, role string) context.Context {
	md := metadata.Pairs("x-user-role", role, "x-user-id", "test-user-1")
	return metadata.NewOutgoingContext(ctx, md)
}

func TestCategoryService_EndToEndHierarchyAndPermissions(t *testing.T) {
	client, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()
	adminCtx := withRole(ctx, auth.RoleAdmin)
	custCtx := withRole(ctx, auth.RoleCustomer)
	merchCtx := withRole(ctx, auth.RoleMerchant)

	// 1. Permission checks on CreateCategory
	// Unauthenticated request should fail with Unauthenticated
	_, err := client.CreateCategory(ctx, &categorypb.CreateCategoryRequest{Name: "Electronics"})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated for unauth call, got %v", status.Code(err))
	}

	// Customer should fail with PermissionDenied
	_, err = client.CreateCategory(custCtx, &categorypb.CreateCategoryRequest{Name: "Electronics"})
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied for Customer role, got %v", status.Code(err))
	}

	// Merchant should fail with PermissionDenied
	_, err = client.CreateCategory(merchCtx, &categorypb.CreateCategoryRequest{Name: "Electronics"})
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied for Merchant role, got %v", status.Code(err))
	}

	// 2. Admin successfully creates root category "Electronics"
	elecResp, err := client.CreateCategory(adminCtx, &categorypb.CreateCategoryRequest{
		Name:        "Electronics",
		Description: "All electronics products",
	})
	if err != nil {
		t.Fatalf("admin failed creating Electronics root category: %v", err)
	}
	elecID := elecResp.Category.Id
	if elecID == "" || elecResp.Category.ParentCategoryId != "" {
		t.Fatalf("invalid root category created: %+v", elecResp.Category)
	}

	// 3. Duplicate root category name is rejected
	_, err = client.CreateCategory(adminCtx, &categorypb.CreateCategoryRequest{
		Name: "Electronics",
	})
	if status.Code(err) != codes.AlreadyExists {
		t.Errorf("expected AlreadyExists for duplicate root category, got %v", status.Code(err))
	}

	// 4. Admin creates child category "Mobiles" under "Electronics"
	mobilesResp, err := client.CreateCategory(adminCtx, &categorypb.CreateCategoryRequest{
		Name:             "Mobiles",
		ParentCategoryId: elecID,
		Description:      "Mobile phones",
	})
	if err != nil {
		t.Fatalf("admin failed creating Mobiles: %v", err)
	}
	mobilesID := mobilesResp.Category.Id
	if mobilesResp.Category.ParentCategoryId != elecID {
		t.Errorf("expected parent ID to be %s, got %s", elecID, mobilesResp.Category.ParentCategoryId)
	}

	// 5. Admin creates child category "Laptops" under "Electronics"
	laptopsResp, err := client.CreateCategory(adminCtx, &categorypb.CreateCategoryRequest{
		Name:             "Laptops",
		ParentCategoryId: elecID,
		Description:      "Laptops and notebooks",
	})
	if err != nil {
		t.Fatalf("admin failed creating Laptops: %v", err)
	}
	laptopsID := laptopsResp.Category.Id

	// 6. Duplicate subcategory name under SAME parent is rejected
	_, err = client.CreateCategory(adminCtx, &categorypb.CreateCategoryRequest{
		Name:             "Mobiles",
		ParentCategoryId: elecID,
	})
	if status.Code(err) != codes.AlreadyExists {
		t.Errorf("expected AlreadyExists for duplicate category under same parent, got %v", status.Code(err))
	}

	// 7. Admin creates grandchildren under "Mobiles": "Android Phones" and "iPhones"
	androidResp, err := client.CreateCategory(adminCtx, &categorypb.CreateCategoryRequest{
		Name:             "Android Phones",
		ParentCategoryId: mobilesID,
	})
	if err != nil {
		t.Fatalf("failed creating Android Phones: %v", err)
	}

	iphoneResp, err := client.CreateCategory(adminCtx, &categorypb.CreateCategoryRequest{
		Name:             "iPhones",
		ParentCategoryId: mobilesID,
	})
	if err != nil {
		t.Fatalf("failed creating iPhones: %v", err)
	}

	// 8. Same category name under DIFFERENT parents IS SUPPORTED
	// e.g. "Accessories" under Mobiles AND "Accessories" under Laptops
	mobAccResp, err := client.CreateCategory(adminCtx, &categorypb.CreateCategoryRequest{
		Name:             "Accessories",
		ParentCategoryId: mobilesID,
	})
	if err != nil {
		t.Fatalf("failed creating Accessories under Mobiles: %v", err)
	}

	lapAccResp, err := client.CreateCategory(adminCtx, &categorypb.CreateCategoryRequest{
		Name:             "Accessories",
		ParentCategoryId: laptopsID,
	})
	if err != nil {
		t.Fatalf("failed creating Accessories under Laptops: %v", err)
	}

	if mobAccResp.Category.Id == lapAccResp.Category.Id {
		t.Errorf("expected distinct categories for same name under different parents")
	}

	// 9. Read operations: Customer / unauthenticated user can retrieve categories
	getResp, err := client.GetCategory(ctx, &categorypb.GetCategoryRequest{Id: androidResp.Category.Id})
	if err != nil {
		t.Fatalf("public GetCategory failed: %v", err)
	}
	if getResp.Category.Name != "Android Phones" {
		t.Errorf("expected Android Phones, got %s", getResp.Category.Name)
	}

	// 10. GetChildCategories returns immediate children
	mobChildren, err := client.GetChildCategories(custCtx, &categorypb.GetChildCategoriesRequest{
		ParentCategoryId: mobilesID,
	})
	if err != nil {
		t.Fatalf("failed getting children of Mobiles: %v", err)
	}
	if mobChildren.Total != 3 { // Android Phones, iPhones, Accessories
		t.Errorf("expected 3 children for Mobiles, got %d", mobChildren.Total)
	}

	// 11. Customer attempting UpdateCategory is rejected with PermissionDenied
	_, err = client.UpdateCategory(custCtx, &categorypb.UpdateCategoryRequest{
		Id:   mobilesID,
		Name: "Cellphones",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied for Customer UpdateCategory, got %v", status.Code(err))
	}

	// 12. Admin updates category successfully
	updateResp, err := client.UpdateCategory(adminCtx, &categorypb.UpdateCategoryRequest{
		Id:          mobilesID,
		Name:        "Smartphones",
		Description: "Modern smartphones",
	})
	if err != nil {
		t.Fatalf("admin failed updating category: %v", err)
	}
	if updateResp.Category.Name != "Smartphones" {
		t.Errorf("expected name Smartphones, got %s", updateResp.Category.Name)
	}

	// 13. Customer attempting DeleteCategory is rejected with PermissionDenied
	_, err = client.DeleteCategory(custCtx, &categorypb.DeleteCategoryRequest{
		Id: iphoneResp.Category.Id,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied for Customer DeleteCategory, got %v", status.Code(err))
	}

	// 14. Admin deletes category successfully
	delResp, err := client.DeleteCategory(adminCtx, &categorypb.DeleteCategoryRequest{
		Id: iphoneResp.Category.Id,
	})
	if err != nil {
		t.Fatalf("admin failed deleting category: %v", err)
	}
	if !delResp.Success {
		t.Errorf("expected delete success to be true")
	}

	// Verify category no longer exists
	_, err = client.GetCategory(ctx, &categorypb.GetCategoryRequest{Id: iphoneResp.Category.Id})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound after delete, got %v", status.Code(err))
	}

	// 15. Attempting to delete parent category with child categories fails with InvalidArgument
	_, err = client.DeleteCategory(adminCtx, &categorypb.DeleteCategoryRequest{
		Id: mobilesID,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument when deleting parent with children, got %v", status.Code(err))
	}
	if !strings.Contains(err.Error(), "Please delete all child categories before deleting the parent category.") {
		t.Errorf("expected specific error message, got %v", err)
	}
}
