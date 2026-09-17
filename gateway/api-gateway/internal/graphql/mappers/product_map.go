package maps

import "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/productpb"

func MapProduct(p *productpb.Product) map[string]interface{} {
	if p == nil {
		return nil
	}
	return map[string]interface{}{
		"id":            p.Id,
		"name":          p.Name,
		"description":   p.Description,
		"price":         p.Price,
		"categoryId":    p.CategoryId,
		"stockQuantity": p.StockQuantity,
		"createdAt":     p.CreatedAt,
	}
}
