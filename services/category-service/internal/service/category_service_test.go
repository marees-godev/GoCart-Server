package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/service"
)

type mockCategoryRepo struct {
	categories map[string]*model.Category
	idCounter  int
}

func newMockCategoryRepo() *mockCategoryRepo {
	return &mockCategoryRepo{
		categories: make(map[string]*model.Category),
	}
}

func (m *mockCategoryRepo) CreateCategory(ctx context.Context, cat *model.Category) error {
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

func (m *mockCategoryRepo) GetByID(ctx context.Context, id string) (*model.Category, error) {
	cat, ok := m.categories[id]
	if !ok || cat.DeletedAt != nil {
		return nil, appErrors.NotFound("category not found")
	}
	cp := *cat
	return &cp, nil
}

func (m *mockCategoryRepo) GetByNameAndParent(ctx context.Context, name string, parentCategoryID *string) (*model.Category, error) {
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

func (m *mockCategoryRepo) ListCategory(ctx context.Context, filter dto.ListCategoriesRequest) ([]*model.Category, int32, error) {
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

func (m *mockCategoryRepo) GetChildren(ctx context.Context, parentCategoryID string, limit, offset int32) ([]*model.Category, int32, error) {
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

func (m *mockCategoryRepo) UpdateCategory(ctx context.Context, cat *model.Category) error {
	existing, ok := m.categories[cat.ID]
	if !ok || existing.DeletedAt != nil {
		return appErrors.NotFound("category not found")
	}
	cat.UpdatedAt = time.Now()
	cp := *cat
	m.categories[cat.ID] = &cp
	return nil
}

func (m *mockCategoryRepo) DeleteCategory(ctx context.Context, id string) error {
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

func (m *mockCategoryRepo) HasChildren(ctx context.Context, parentCategoryID string) (bool, error) {
	for _, c := range m.categories {
		if c.DeletedAt == nil && c.ParentCategoryID != nil && *c.ParentCategoryID == parentCategoryID {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockCategoryRepo) IsDescendant(ctx context.Context, candidateDescendantID, ancestorID string) (bool, error) {
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

func TestCategoryService_HierarchyAndUniqueness(t *testing.T) {
	ctx := context.Background()
	repo := newMockCategoryRepo()
	svc := service.NewCategoryService(repo)

	// 1. Create root category: Electronics
	electronics, err := svc.CreateCategory(ctx, dto.CreateCategoryRequest{
		Name:        "Electronics",
		Description: "Electronic devices and gadgets",
	})
	if err != nil {
		t.Fatalf("failed to create root category: %v", err)
	}
	if electronics.ParentCategoryID != nil {
		t.Errorf("expected root category parent to be nil, got %v", *electronics.ParentCategoryID)
	}

	// 2. Reject duplicate root category with same name
	_, err = svc.CreateCategory(ctx, dto.CreateCategoryRequest{
		Name: "Electronics",
	})
	if err == nil {
		t.Fatal("expected duplicate root category name to be rejected")
	}

	// 3. Create subcategories under Electronics: Mobiles, Laptops
	mobiles, err := svc.CreateCategory(ctx, dto.CreateCategoryRequest{
		Name:             "Mobiles",
		ParentCategoryID: &electronics.ID,
	})
	if err != nil {
		t.Fatalf("failed to create Mobiles category: %v", err)
	}
	if mobiles.ParentCategoryID == nil || *mobiles.ParentCategoryID != electronics.ID {
		t.Errorf("expected parent to be %s, got %v", electronics.ID, mobiles.ParentCategoryID)
	}

	laptops, err := svc.CreateCategory(ctx, dto.CreateCategoryRequest{
		Name:             "Laptops",
		ParentCategoryID: &electronics.ID,
	})
	if err != nil {
		t.Fatalf("failed to create Laptops category: %v", err)
	}

	// 4. Reject duplicate subcategory under same parent
	_, err = svc.CreateCategory(ctx, dto.CreateCategoryRequest{
		Name:             "Mobiles",
		ParentCategoryID: &electronics.ID,
	})
	if err == nil {
		t.Fatal("expected duplicate category name under same parent to be rejected")
	}

	// 5. Create subcategory under Mobiles: Android Phones, iPhones
	androidPhones, err := svc.CreateCategory(ctx, dto.CreateCategoryRequest{
		Name:             "Android Phones",
		ParentCategoryID: &mobiles.ID,
	})
	if err != nil {
		t.Fatalf("failed to create Android Phones: %v", err)
	}

	iPhones, err := svc.CreateCategory(ctx, dto.CreateCategoryRequest{
		Name:             "iPhones",
		ParentCategoryID: &mobiles.ID,
	})
	if err != nil {
		t.Fatalf("failed to create iPhones: %v", err)
	}

	// 6. Support same category name under DIFFERENT parents
	// e.g. "Accessories" under Mobiles AND "Accessories" under Laptops
	mobileAccessories, err := svc.CreateCategory(ctx, dto.CreateCategoryRequest{
		Name:             "Accessories",
		ParentCategoryID: &mobiles.ID,
	})
	if err != nil {
		t.Fatalf("failed to create Accessories under Mobiles: %v", err)
	}

	laptopAccessories, err := svc.CreateCategory(ctx, dto.CreateCategoryRequest{
		Name:             "Accessories",
		ParentCategoryID: &laptops.ID,
	})
	if err != nil {
		t.Fatalf("failed to create Accessories under Laptops: %v", err)
	}
	if mobileAccessories.ID == laptopAccessories.ID {
		t.Errorf("categories under different parents should have different IDs")
	}

	// 7. Reject creation with non-existent parent
	nonExistentParent := "non-existent-parent-id"
	_, err = svc.CreateCategory(ctx, dto.CreateCategoryRequest{
		Name:             "Smartwatches",
		ParentCategoryID: &nonExistentParent,
	})
	if err == nil {
		t.Fatal("expected creation with non-existent parent to fail")
	}

	// 8. Retrieve child categories of Electronics
	children, total, err := svc.GetChildCategories(ctx, dto.GetChildCategoriesRequest{
		ParentCategoryID: electronics.ID,
	})
	if err != nil {
		t.Fatalf("failed to get child categories: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 children for Electronics, got %d", total)
	}
	childNames := make(map[string]bool)
	for _, c := range children {
		childNames[c.Name] = true
	}
	if !childNames["Mobiles"] || !childNames["Laptops"] {
		t.Errorf("expected children Mobiles and Laptops, got %+v", children)
	}

	// 9. Retrieve child categories of Mobiles
	mobileChildren, totalMobile, err := svc.GetChildCategories(ctx, dto.GetChildCategoriesRequest{
		ParentCategoryID: mobiles.ID,
	})
	if err != nil {
		t.Fatalf("failed to get child categories of Mobiles: %v", err)
	}
	if totalMobile != 3 || len(mobileChildren) != 3 { // Android Phones, iPhones, Accessories
		t.Errorf("expected 3 children for Mobiles, got %d (len %d)", totalMobile, len(mobileChildren))
	}

	// 10. Update category: change name and prevent duplicate under same parent
	newName := "Laptops"
	_, err = svc.UpdateCategory(ctx, dto.UpdateCategoryRequest{
		ID:   mobiles.ID,
		Name: &newName,
	})
	if err == nil {
		t.Fatal("expected renaming Mobiles to Laptops under same parent to fail with conflict")
	}

	// 11. Circular reference prevention:
	// Trying to set Electronics parent to its descendant (e.g. Mobiles or iPhones)
	_, err = svc.UpdateCategory(ctx, dto.UpdateCategoryRequest{
		ID:               electronics.ID,
		ParentCategoryID: &mobiles.ID,
	})
	if err == nil {
		t.Fatal("expected circular reference to be rejected when setting parent to direct child")
	}

	_, err = svc.UpdateCategory(ctx, dto.UpdateCategoryRequest{
		ID:               electronics.ID,
		ParentCategoryID: &androidPhones.ID,
	})
	if err == nil {
		t.Fatal("expected circular reference to be rejected when setting parent to grandchild")
	}

	// 12. Cannot set category as its own parent
	_, err = svc.UpdateCategory(ctx, dto.UpdateCategoryRequest{
		ID:               electronics.ID,
		ParentCategoryID: &electronics.ID,
	})
	if err == nil {
		t.Fatal("expected setting category as its own parent to fail")
	}

	// 13. Soft delete leaf category succeeds
	err = svc.DeleteCategory(ctx, iPhones.ID)
	if err != nil {
		t.Fatalf("failed to delete category: %v", err)
	}

	_, err = svc.GetCategory(ctx, iPhones.ID)
	if err == nil {
		t.Fatal("expected deleted category to return NotFound")
	}

	// 14. Cannot delete parent category if it has active child categories
	err = svc.DeleteCategory(ctx, mobiles.ID)
	if err == nil {
		t.Fatal("expected deleting parent with children to fail")
	}
	if !strings.Contains(err.Error(), "Please delete all child categories before deleting the parent category.") {
		t.Errorf("expected error message about deleting child categories first, got %v", err)
	}

	// Cannot delete root Electronics because Mobiles and Laptops are under it
	err = svc.DeleteCategory(ctx, electronics.ID)
	if err == nil {
		t.Fatal("expected deleting root with children to fail")
	}
	if !strings.Contains(err.Error(), "Please delete all child categories before deleting the parent category.") {
		t.Errorf("expected error message about deleting child categories first, got %v", err)
	}

	// Delete remaining children under Mobiles: androidPhones, mobileAccessories
	if err := svc.DeleteCategory(ctx, androidPhones.ID); err != nil {
		t.Fatalf("failed deleting androidPhones: %v", err)
	}
	if err := svc.DeleteCategory(ctx, mobileAccessories.ID); err != nil {
		t.Fatalf("failed deleting mobileAccessories: %v", err)
	}

	// Now Mobiles has no more children, so deleting Mobiles succeeds
	if err := svc.DeleteCategory(ctx, mobiles.ID); err != nil {
		t.Fatalf("failed deleting mobiles after children deleted: %v", err)
	}
}

func TestCategoryService_AvailabilityControls(t *testing.T) {
	ctx := context.Background()
	repo := newMockCategoryRepo()
	svc := service.NewCategoryService(repo)

	// 1. Create category (default active)
	cat, err := svc.CreateCategory(ctx, dto.CreateCategoryRequest{
		Name:        "Books",
		Description: "Books and Literature",
	})
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}
	if !cat.IsActive {
		t.Fatalf("expected newly created category to be active by default")
	}

	// 2. Validate category for product assignment (should succeed when active)
	validCat, err := svc.ValidateCategoryForAssignment(ctx, cat.ID)
	if err != nil {
		t.Fatalf("expected active category to be valid for assignment, got: %v", err)
	}
	if validCat.ID != cat.ID {
		t.Errorf("expected category ID %s, got %s", cat.ID, validCat.ID)
	}

	// 3. Disable the category
	disableFlag := false
	updatedCat, err := svc.UpdateCategory(ctx, dto.UpdateCategoryRequest{
		ID:       cat.ID,
		IsActive: &disableFlag,
	})
	if err != nil {
		t.Fatalf("failed to disable category: %v", err)
	}
	if updatedCat.IsActive {
		t.Fatalf("expected category to be disabled (is_active=false)")
	}

	// 4. Validate category for product assignment (should fail when disabled)
	_, err = svc.ValidateCategoryForAssignment(ctx, cat.ID)
	if err == nil {
		t.Fatalf("expected assignment validation for disabled category to fail")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("expected error message to mention 'disabled', got: %v", err)
	}

	// 5. Existing products referencing disabled category remain valid (GetCategory succeeds)
	existingRefCat, err := svc.GetCategory(ctx, cat.ID)
	if err != nil {
		t.Fatalf("expected existing product reference lookup (GetCategory) to succeed for disabled category, got: %v", err)
	}
	if existingRefCat.IsActive {
		t.Errorf("expected retrieved category to have IsActive=false")
	}

	// 6. Re-enable category
	enableFlag := true
	reEnabledCat, err := svc.UpdateCategory(ctx, dto.UpdateCategoryRequest{
		ID:       cat.ID,
		IsActive: &enableFlag,
	})
	if err != nil {
		t.Fatalf("failed to re-enable category: %v", err)
	}
	if !reEnabledCat.IsActive {
		t.Fatalf("expected category to be re-enabled (is_active=true)")
	}

	// 7. Validate category for product assignment after re-enabling (should succeed)
	validCat, err = svc.ValidateCategoryForAssignment(ctx, cat.ID)
	if err != nil {
		t.Fatalf("expected re-enabled category to be valid for assignment, got: %v", err)
	}
	if !validCat.IsActive {
		t.Errorf("expected validated category to be active")
	}
}
