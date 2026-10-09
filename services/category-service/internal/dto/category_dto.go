package dto

import (
	"time"

	categorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/category"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/model"
)

type CreateCategoryRequest struct {
	Name             string  `json:"name"`
	ParentCategoryID *string `json:"parent_category_id,omitempty"`
	Description      string  `json:"description"`
	IsActive         *bool   `json:"is_active,omitempty"`
}

type UpdateCategoryRequest struct {
	ID               string  `json:"id"`
	Name             *string `json:"name,omitempty"`
	ParentCategoryID *string `json:"parent_category_id,omitempty"`
	ClearParent      bool    `json:"clear_parent,omitempty"`
	Description      *string `json:"description,omitempty"`
	IsActive         *bool   `json:"is_active,omitempty"`
}

type ListCategoriesRequest struct {
	Limit            int32   `json:"limit"`
	Offset           int32   `json:"offset"`
	ParentCategoryID *string `json:"parent_category_id,omitempty"`
	RootOnly         bool    `json:"root_only"`
}

type GetChildCategoriesRequest struct {
	ParentCategoryID string `json:"parent_category_id"`
	Limit            int32  `json:"limit"`
	Offset           int32  `json:"offset"`
}

func ToCategoryPB(cat *model.Category) *categorypb.Category {
	if cat == nil {
		return nil
	}

	var parentID string
	if cat.ParentCategoryID != nil {
		parentID = *cat.ParentCategoryID
	}

	return &categorypb.Category{
		Id:               cat.ID,
		Name:             cat.Name,
		ParentCategoryId: parentID,
		Description:      cat.Description,
		IsActive:         cat.IsActive,
		CreatedAt:        cat.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        cat.UpdatedAt.Format(time.RFC3339),
	}
}

func ToCategoryListPB(cats []*model.Category) []*categorypb.Category {
	res := make([]*categorypb.Category, len(cats))
	for i, c := range cats {
		res[i] = ToCategoryPB(c)
	}
	return res
}
