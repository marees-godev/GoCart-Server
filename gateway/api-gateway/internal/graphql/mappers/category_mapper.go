package maps

import (
	categorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/category"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
)

func MapCategory(c *categorypb.Category) *model.Category {
	if c == nil {
		return nil
	}

	var parentID *string
	pID := c.ParentCategoryId
	if pID != "" {
		parentID = &pID
	}

	var desc *string
	if c.Description != "" {
		desc = &c.Description
	}

	var createdAt *string
	if c.CreatedAt != "" {
		createdAt = &c.CreatedAt
	}

	var updatedAt *string
	if c.UpdatedAt != "" {
		updatedAt = &c.UpdatedAt
	}

	return &model.Category{
		ID:               c.Id,
		Name:             c.Name,
		ParentCategoryID: parentID,
		Description:      desc,
		IsActive:         c.IsActive,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}
}

func MapCategories(categories []*categorypb.Category) []*model.Category {
	if categories == nil {
		return []*model.Category{}
	}
	res := make([]*model.Category, 0, len(categories))
	for _, c := range categories {
		if c != nil {
			res = append(res, MapCategory(c))
		}
	}
	return res
}
