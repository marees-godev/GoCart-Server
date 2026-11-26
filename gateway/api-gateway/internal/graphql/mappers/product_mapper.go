package maps

import (
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
)

func MapProductVariant(v *productpb.ProductVariant) *model.ProductVariant {
	if v == nil {
		return nil
	}

	var attrJSON *string
	if v.AttributesJson != "" {
		attrJSON = &v.AttributesJson
	}

	var createdAt *string
	if v.CreatedAt != "" {
		createdAt = &v.CreatedAt
	}

	var updatedAt *string
	if v.UpdatedAt != "" {
		updatedAt = &v.UpdatedAt
	}

	var deletedAt *string
	if v.DeletedAt != "" {
		deletedAt = &v.DeletedAt
	}

	return &model.ProductVariant{
		ID:             v.Id,
		ProductID:      v.ProductId,
		Sku:            v.Sku,
		Name:           v.Name,
		Price:          v.Price,
		Mrp:            v.Mrp,
		Stock:          int(v.Stock),
		AttributesJSON: attrJSON,
		Status:         model.ProductStatus(v.Status),
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		DeletedAt:      deletedAt,
	}
}

func MapProductVariants(variants []*productpb.ProductVariant) []*model.ProductVariant {
	if variants == nil {
		return []*model.ProductVariant{}
	}
	res := make([]*model.ProductVariant, 0, len(variants))
	for _, v := range variants {
		if v != nil {
			res = append(res, MapProductVariant(v))
		}
	}
	return res
}

func MapProduct(p *productpb.Product) *model.Product {
	if p == nil {
		return nil
	}

	var desc *string
	if p.Description != "" {
		desc = &p.Description
	}

	var createdAt *string
	if p.CreatedAt != "" {
		createdAt = &p.CreatedAt
	}

	var updatedAt *string
	if p.UpdatedAt != "" {
		updatedAt = &p.UpdatedAt
	}

	var deletedAt *string
	if p.DeletedAt != "" {
		deletedAt = &p.DeletedAt
	}

	name := p.Name
	if name == "" && p.Productname != "" {
		name = p.Productname
	}

	var imgURL *string
	if p.ImageUrl != "" {
		imgURL = &p.ImageUrl
	}

	isExchangeEligible := p.IsExchangeEligible
	maxExchangeDiscount := p.MaxExchangeDiscount

	return &model.Product{
		ID:                  p.Id,
		StoreID:             p.StoreId,
		CategoryID:          p.CategoryId,
		Sku:                 p.Sku,
		Name:                name,
		Description:         desc,
		Price:               p.Price,
		Mrp:                 p.Mrp,
		Tax:                 p.Tax,
		Weight:              p.Weight,
		Status:              model.ProductStatus(p.Status),
		ImageURL:            imgURL,
		Images:              p.Images,
		Variants:            MapProductVariants(p.Variants),
		AvgRating:           p.AvgRating,
		IsExchangeEligible:  &isExchangeEligible,
		MaxExchangeDiscount: &maxExchangeDiscount,
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
		DeletedAt:           deletedAt,
	}
}

func MapProducts(products []*productpb.Product) []*model.Product {
	if products == nil {
		return []*model.Product{}
	}
	res := make([]*model.Product, 0, len(products))
	for _, p := range products {
		if p != nil {
			res = append(res, MapProduct(p))
		}
	}
	return res
}

func MapProductReport(r *productpb.ProductReport) *model.ProductReport {
	if r == nil {
		return nil
	}
	var desc *string
	if r.Description != "" {
		desc = &r.Description
	}
	var createdAt *string
	if r.CreatedAt != "" {
		createdAt = &r.CreatedAt
	}
	var updatedAt *string
	if r.UpdatedAt != "" {
		updatedAt = &r.UpdatedAt
	}
	return &model.ProductReport{
		ID:          r.Id,
		ProductID:   r.ProductId,
		ReporterID:  r.ReporterId,
		Reason:      model.ProductReportReason(r.Reason),
		Description: desc,
		Status:      model.ProductReportStatus(r.Status),
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

func MapProductReports(reports []*productpb.ProductReport) []*model.ProductReport {
	if reports == nil {
		return []*model.ProductReport{}
	}
	res := make([]*model.ProductReport, 0, len(reports))
	for _, r := range reports {
		if r != nil {
			res = append(res, MapProductReport(r))
		}
	}
	return res
}
