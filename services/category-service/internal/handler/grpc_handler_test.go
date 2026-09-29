package handler_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	categorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/category"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockRepo struct {
	categories map[string]*model.Category
	idCounter  int
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		categories: make(map[string]*model.Category),
	}
}

func (m *mockRepo) CreateCategory(ctx context.Context, cat *model.Category) error {
	if cat.ID == "" {
		m.idCounter++
		cat.ID = fmt.Sprintf("cat-%d", m.idCounter)
	}
	cat.CreatedAt = time.Now()
	cat.UpdatedAt = time.Now()
	cp := *cat
	m.categories[cat.ID] = &cp
	return nil
}

func (m *mockRepo) GetByID(ctx context.Context, id string) (*model.Category, error) {
	cat, ok := m.categories[id]
	if !ok || cat.DeletedAt != nil {
		return nil, appErrors.NotFound("category not found")
	}
	cp := *cat
	return &cp, nil
}

func (m *mockRepo) GetByNameAndParent(ctx context.Context, name string, parentCategoryID *string) (*model.Category, error) {
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

func (m *mockRepo) ListCategory(ctx context.Context, filter dto.ListCategoriesRequest) ([]*model.Category, int32, error) {
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

func (m *mockRepo) GetChildren(ctx context.Context, parentCategoryID string, limit, offset int32) ([]*model.Category, int32, error) {
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

func (m *mockRepo) UpdateCategory(ctx context.Context, cat *model.Category) error {
	existing, ok := m.categories[cat.ID]
	if !ok || existing.DeletedAt != nil {
		return appErrors.NotFound("category not found")
	}
	cat.UpdatedAt = time.Now()
	cp := *cat
	m.categories[cat.ID] = &cp
	return nil
}

func (m *mockRepo) DeleteCategory(ctx context.Context, id string) error {
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

func (m *mockRepo) HasChildren(ctx context.Context, parentCategoryID string) (bool, error) {
	for _, c := range m.categories {
		if c.DeletedAt == nil && c.ParentCategoryID != nil && *c.ParentCategoryID == parentCategoryID {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockRepo) IsDescendant(ctx context.Context, candidateDescendantID, ancestorID string) (bool, error) {
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

func setupHandler() *handler.CategoryGRPCHandler {
	repo := newMockRepo()
	svc := service.NewCategoryService(repo)
	return handler.NewCategoryGRPCHandler(svc)
}

func TestCategoryGRPCHandler_CreateCategory(t *testing.T) {
	h := setupHandler()
	ctx := context.Background()

	// 1. Missing request
	_, err := h.CreateCategory(ctx, nil)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}

	// 2. Missing name
	_, err = h.CreateCategory(ctx, &categorypb.CreateCategoryRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}

	// 3. Create root category: Electronics
	resp, err := h.CreateCategory(ctx, &categorypb.CreateCategoryRequest{
		Name:        "Electronics",
		Description: "Electronics devices",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.Category.Id == "" || resp.Category.Name != "Electronics" {
		t.Fatalf("unexpected category response: %+v", resp.Category)
	}
	if resp.Category.ParentCategoryId != "" {
		t.Errorf("expected empty parent for root category, got %s", resp.Category.ParentCategoryId)
	}

	electronicsID := resp.Category.Id

	// 4. Create child category: Mobiles
	mobilesResp, err := h.CreateCategory(ctx, &categorypb.CreateCategoryRequest{
		Name:             "Mobiles",
		ParentCategoryId: electronicsID,
	})
	if err != nil {
		t.Fatalf("expected success creating child category, got %v", err)
	}
	if mobilesResp.Category.ParentCategoryId != electronicsID {
		t.Errorf("expected parent ID to match electronics ID, got %s", mobilesResp.Category.ParentCategoryId)
	}

	// 5. Reject duplicate under same parent
	_, err = h.CreateCategory(ctx, &categorypb.CreateCategoryRequest{
		Name:             "Mobiles",
		ParentCategoryId: electronicsID,
	})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("expected AlreadyExists for duplicate category, got %v", status.Code(err))
	}
}

func TestCategoryGRPCHandler_GetCategory(t *testing.T) {
	h := setupHandler()
	ctx := context.Background()

	// 1. Missing request / ID
	_, err := h.GetCategory(ctx, nil)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}

	_, err = h.GetCategory(ctx, &categorypb.GetCategoryRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}

	// 2. Not found
	_, err = h.GetCategory(ctx, &categorypb.GetCategoryRequest{Id: "unknown-id"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", status.Code(err))
	}

	// 3. Create and get
	created, err := h.CreateCategory(ctx, &categorypb.CreateCategoryRequest{Name: "Fashion"})
	if err != nil {
		t.Fatalf("failed creating category: %v", err)
	}

	got, err := h.GetCategory(ctx, &categorypb.GetCategoryRequest{Id: created.Category.Id})
	if err != nil {
		t.Fatalf("failed getting category: %v", err)
	}
	if got.Category.Name != "Fashion" {
		t.Errorf("expected Fashion, got %s", got.Category.Name)
	}
}

func TestCategoryGRPCHandler_ListAndChildCategories(t *testing.T) {
	h := setupHandler()
	ctx := context.Background()

	// Create root: Electronics
	elec, err := h.CreateCategory(ctx, &categorypb.CreateCategoryRequest{Name: "Electronics"})
	if err != nil {
		t.Fatalf("failed to create root category: %v", err)
	}

	// Create sub: Mobiles, Laptops
	mob, err := h.CreateCategory(ctx, &categorypb.CreateCategoryRequest{Name: "Mobiles", ParentCategoryId: elec.Category.Id})
	if err != nil {
		t.Fatalf("failed to create sub category: %v", err)
	}
	lap, err := h.CreateCategory(ctx, &categorypb.CreateCategoryRequest{Name: "Laptops", ParentCategoryId: elec.Category.Id})
	if err != nil {
		t.Fatalf("failed to create sub category: %v", err)
	}

	// Create grandchild: Android, iPhone
	_, err = h.CreateCategory(ctx, &categorypb.CreateCategoryRequest{Name: "Android Phones", ParentCategoryId: mob.Category.Id})
	if err != nil {
		t.Fatalf("failed to create grandchild category: %v", err)
	}
	_, err = h.CreateCategory(ctx, &categorypb.CreateCategoryRequest{Name: "iPhones", ParentCategoryId: mob.Category.Id})
	if err != nil {
		t.Fatalf("failed to create grandchild category: %v", err)
	}

	// List all
	allList, err := h.ListCategories(ctx, &categorypb.ListCategoriesRequest{Limit: 100})
	if err != nil {
		t.Fatalf("failed to list categories: %v", err)
	}
	if allList.Total != 5 {
		t.Errorf("expected total 5, got %d", allList.Total)
	}

	// List root only
	rootList, err := h.ListCategories(ctx, &categorypb.ListCategoriesRequest{RootOnly: true})
	if err != nil {
		t.Fatalf("failed to list root categories: %v", err)
	}
	if rootList.Total != 1 {
		t.Errorf("expected total 1 root category, got %d", rootList.Total)
	}

	// Get child categories of Electronics
	elecChildren, err := h.GetChildCategories(ctx, &categorypb.GetChildCategoriesRequest{ParentCategoryId: elec.Category.Id})
	if err != nil {
		t.Fatalf("failed to get children: %v", err)
	}
	if elecChildren.Total != 2 {
		t.Errorf("expected 2 children, got %d", elecChildren.Total)
	}

	// Get child categories of Mobiles
	mobChildren, err := h.GetChildCategories(ctx, &categorypb.GetChildCategoriesRequest{ParentCategoryId: mob.Category.Id})
	if err != nil {
		t.Fatalf("failed to get children: %v", err)
	}
	if mobChildren.Total != 2 {
		t.Errorf("expected 2 children, got %d", mobChildren.Total)
	}

	_ = lap
}

func TestCategoryGRPCHandler_UpdateAndDelete(t *testing.T) {
	h := setupHandler()
	ctx := context.Background()

	created, err := h.CreateCategory(ctx, &categorypb.CreateCategoryRequest{
		Name:        "Tablets",
		Description: "Tablet devices",
	})
	if err != nil {
		t.Fatalf("failed creating category: %v", err)
	}

	// Update name & description
	updated, err := h.UpdateCategory(ctx, &categorypb.UpdateCategoryRequest{
		Id:          created.Category.Id,
		Name:        "Smart Tablets",
		Description: "Updated description",
	})
	if err != nil {
		t.Fatalf("failed updating category: %v", err)
	}
	if updated.Category.Name != "Smart Tablets" || updated.Category.Description != "Updated description" {
		t.Errorf("update not applied properly: %+v", updated.Category)
	}

	// Delete
	delResp, err := h.DeleteCategory(ctx, &categorypb.DeleteCategoryRequest{Id: created.Category.Id})
	if err != nil {
		t.Fatalf("failed deleting category: %v", err)
	}
	if !delResp.Success {
		t.Errorf("expected delete success true, got false")
	}

	// Verify not found after deletion
	_, err = h.GetCategory(ctx, &categorypb.GetCategoryRequest{Id: created.Category.Id})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound after delete, got %v", status.Code(err))
	}

	// Test deletion restriction on parent category with child
	parent, err := h.CreateCategory(ctx, &categorypb.CreateCategoryRequest{
		Name: "Parent Category",
	})
	if err != nil {
		t.Fatalf("failed creating parent category: %v", err)
	}

	child, err := h.CreateCategory(ctx, &categorypb.CreateCategoryRequest{
		Name:             "Child Category",
		ParentCategoryId: parent.Category.Id,
	})
	if err != nil {
		t.Fatalf("failed creating child category: %v", err)
	}

	// Attempting to delete parent should fail with InvalidArgument
	_, err = h.DeleteCategory(ctx, &categorypb.DeleteCategoryRequest{Id: parent.Category.Id})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument when deleting parent with children, got %v", status.Code(err))
	}
	if !strings.Contains(err.Error(), "Please delete all child categories before deleting the parent category.") {
		t.Errorf("expected specific error message, got %v", err)
	}

	// Delete child category first
	_, err = h.DeleteCategory(ctx, &categorypb.DeleteCategoryRequest{Id: child.Category.Id})
	if err != nil {
		t.Fatalf("failed deleting child category: %v", err)
	}

	// Now parent can be deleted
	_, err = h.DeleteCategory(ctx, &categorypb.DeleteCategoryRequest{Id: parent.Category.Id})
	if err != nil {
		t.Fatalf("failed deleting parent category after child deleted: %v", err)
	}
}
