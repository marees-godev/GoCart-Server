package service

import (
	"context"
	"log/slog"
	"strings"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/repository"
)

type CategoryService interface {
	CreateCategory(ctx context.Context, req dto.CreateCategoryRequest) (*model.Category, error)
	GetCategory(ctx context.Context, id string) (*model.Category, error)
	ListCategories(ctx context.Context, req dto.ListCategoriesRequest) ([]*model.Category, int32, error)
	GetChildCategories(ctx context.Context, req dto.GetChildCategoriesRequest) ([]*model.Category, int32, error)
	UpdateCategory(ctx context.Context, req dto.UpdateCategoryRequest) (*model.Category, error)
	DeleteCategory(ctx context.Context, id string) error
}

type categoryService struct {
	repo   repository.CategoryRepository
	logger *slog.Logger
}

func NewCategoryService(repo repository.CategoryRepository, log ...*slog.Logger) CategoryService {
	var l *slog.Logger
	if len(log) > 0 {
		l = log[0]
	}
	return &categoryService{repo: repo, logger: l}
}

func (s *categoryService) CreateCategory(ctx context.Context, req dto.CreateCategoryRequest) (*model.Category, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		if s.logger != nil {
			s.logger.Warn("Category name is required")
		}
		return nil, appErrors.BadRequest("category name is required")
	}

	var parentID *string
	if req.ParentCategoryID != nil && strings.TrimSpace(*req.ParentCategoryID) != "" {
		cleanParentID := strings.TrimSpace(*req.ParentCategoryID)
		// Verify parent exists
		_, err := s.repo.GetByID(ctx, cleanParentID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("Parent category not found during creation", "parent_id", cleanParentID)
			}
			return nil, appErrors.NotFound("parent category not found")
		}
		parentID = &cleanParentID
	}

	// Verify name uniqueness within the same parent scope
	existing, err := s.repo.GetByNameAndParent(ctx, name, parentID)
	if err == nil && existing != nil {
		if s.logger != nil {
			s.logger.Warn("Category name already exists in parent scope", "name", name, "parent_id", parentID)
		}
		return nil, appErrors.Conflict("category name already exists in this parent scope")
	}

	cat := &model.Category{
		Name:             name,
		ParentCategoryID: parentID,
		Description:      strings.TrimSpace(req.Description),
		IsActive:         true,
	}

	if err := s.repo.CreateCategory(ctx, cat); err != nil {
		if s.logger != nil {
			s.logger.Error("Failed to persist category", "error", err, "name", name)
		}
		return nil, err
	}

	if s.logger != nil {
		s.logger.Info("Category created successfully", "id", cat.ID, "name", cat.Name)
	}

	return cat, nil
}

func (s *categoryService) GetCategory(ctx context.Context, id string) (*model.Category, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		if s.logger != nil {
			s.logger.Warn("Category ID is required")
		}
		return nil, appErrors.BadRequest("category ID is required")
	}

	cat, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Category not found", "id", id, "error", err)
		}
		return nil, err
	}

	if s.logger != nil {
		s.logger.Debug("Category retrieved", "id", cat.ID, "name", cat.Name)
	}

	return cat, nil
}

func (s *categoryService) ListCategories(ctx context.Context, req dto.ListCategoriesRequest) ([]*model.Category, int32, error) {
	if req.ParentCategoryID != nil && strings.TrimSpace(*req.ParentCategoryID) != "" {
		clean := strings.TrimSpace(*req.ParentCategoryID)
		req.ParentCategoryID = &clean
	} else {
		req.ParentCategoryID = nil
	}

	categories, total, err := s.repo.ListCategory(ctx, req)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("Failed to list categories", "error", err, "parent_id", req.ParentCategoryID, "root_only", req.RootOnly)
		}
		return nil, 0, err
	}

	if s.logger != nil {
		s.logger.Debug("Listed categories", "count", len(categories), "total", total)
	}

	return categories, total, nil
}

func (s *categoryService) GetChildCategories(ctx context.Context, req dto.GetChildCategoriesRequest) ([]*model.Category, int32, error) {
	parentID := strings.TrimSpace(req.ParentCategoryID)
	if parentID == "" {
		if s.logger != nil {
			s.logger.Warn("Parent category ID is required")
		}
		return nil, 0, appErrors.BadRequest("parent category ID is required")
	}

	// Check if parent category exists
	if _, err := s.repo.GetByID(ctx, parentID); err != nil {
		if s.logger != nil {
			s.logger.Warn("Parent category not found for child categories", "parent_id", parentID, "error", err)
		}
		return nil, 0, appErrors.NotFound("parent category not found")
	}

	categories, total, err := s.repo.GetChildren(ctx, parentID, req.Limit, req.Offset)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("Failed to get child categories", "parent_id", parentID, "error", err)
		}
		return nil, 0, err
	}

	if s.logger != nil {
		s.logger.Debug("Retrieved child categories", "parent_id", parentID, "count", len(categories), "total", total)
	}

	return categories, total, nil
}

func (s *categoryService) UpdateCategory(ctx context.Context, req dto.UpdateCategoryRequest) (*model.Category, error) {
	id := strings.TrimSpace(req.ID)
	if id == "" {
		if s.logger != nil {
			s.logger.Warn("Category ID is required for update")
		}
		return nil, appErrors.BadRequest("category ID is required")
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Category not found for update", "id", id, "error", err)
		}
		return nil, err
	}

	// Determine new name
	targetName := existing.Name
	if req.Name != nil {
		trimmedName := strings.TrimSpace(*req.Name)
		if trimmedName == "" {
			if s.logger != nil {
				s.logger.Warn("Category name cannot be empty for update", "id", id)
			}
			return nil, appErrors.BadRequest("category name cannot be empty")
		}
		targetName = trimmedName
	}

	// Determine target parent ID
	var targetParentID *string
	if req.ClearParent {
		targetParentID = nil
	} else if req.ParentCategoryID != nil {
		cleanParent := strings.TrimSpace(*req.ParentCategoryID)
		if cleanParent == "" {
			targetParentID = nil
		} else {
			targetParentID = &cleanParent
		}
	} else {
		targetParentID = existing.ParentCategoryID
	}

	// If parent is changed to a non-nil parent:
	if targetParentID != nil && (existing.ParentCategoryID == nil || *existing.ParentCategoryID != *targetParentID) {
		if *targetParentID == existing.ID {
			if s.logger != nil {
				s.logger.Warn("Category cannot be its own parent", "id", id)
			}
			return nil, appErrors.BadRequest("category cannot be its own parent")
		}

		// Verify target parent exists
		if _, err := s.repo.GetByID(ctx, *targetParentID); err != nil {
			if s.logger != nil {
				s.logger.Warn("Target parent category not found for update", "target_parent_id", *targetParentID, "error", err)
			}
			return nil, appErrors.NotFound("target parent category not found")
		}

		// Prevent circular hierarchy (target parent must not be a descendant of existing)
		isDescendant, err := s.repo.IsDescendant(ctx, *targetParentID, existing.ID)
		if err != nil {
			if s.logger != nil {
				s.logger.Error("Failed to check circular hierarchy", "id", id, "target_parent_id", *targetParentID, "error", err)
			}
			return nil, err
		}
		if isDescendant {
			if s.logger != nil {
				s.logger.Warn("Circular reference detected in category update", "id", id, "target_parent_id", *targetParentID)
			}
			return nil, appErrors.BadRequest("cannot set parent to a descendant category (circular reference detected)")
		}
	}

	// Check name uniqueness if name or parent changed
	nameChanged := targetName != existing.Name
	parentChanged := (existing.ParentCategoryID == nil && targetParentID != nil) ||
		(existing.ParentCategoryID != nil && targetParentID == nil) ||
		(existing.ParentCategoryID != nil && targetParentID != nil && *existing.ParentCategoryID != *targetParentID)

	if nameChanged || parentChanged {
		sameNameCat, err := s.repo.GetByNameAndParent(ctx, targetName, targetParentID)
		if err == nil && sameNameCat != nil && sameNameCat.ID != existing.ID {
			if s.logger != nil {
				s.logger.Warn("Category name conflict during update", "name", targetName, "parent_id", targetParentID)
			}
			return nil, appErrors.Conflict("category name already exists in this parent scope")
		}
	}

	existing.Name = targetName
	existing.ParentCategoryID = targetParentID
	if req.Description != nil {
		existing.Description = strings.TrimSpace(*req.Description)
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	if err := s.repo.UpdateCategory(ctx, existing); err != nil {
		if s.logger != nil {
			s.logger.Error("Failed to update category in repository", "id", id, "error", err)
		}
		return nil, err
	}

	if s.logger != nil {
		s.logger.Info("Category updated successfully", "id", existing.ID, "name", existing.Name)
	}

	return existing, nil
}

func (s *categoryService) DeleteCategory(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		if s.logger != nil {
			s.logger.Warn("Category ID is required for delete")
		}
		return appErrors.BadRequest("category ID is required")
	}

	// Ensure category exists before deleting
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		if s.logger != nil {
			s.logger.Warn("Category not found for deletion", "id", id, "error", err)
		}
		return err
	}

	// Check if category has any child categories
	hasChildren, err := s.repo.HasChildren(ctx, id)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("Failed to check child categories before deletion", "id", id, "error", err)
		}
		return err
	}
	if hasChildren {
		if s.logger != nil {
			s.logger.Warn("Cannot delete category with child categories", "id", id)
		}
		return appErrors.BadRequest("Please delete all child categories before deleting the parent category.")
	}

	if err := s.repo.DeleteCategory(ctx, id); err != nil {
		if s.logger != nil {
			s.logger.Error("Failed to delete category in repository", "id", id, "error", err)
		}
		return err
	}

	if s.logger != nil {
		s.logger.Info("Category deleted successfully", "id", id)
	}

	return nil
}
