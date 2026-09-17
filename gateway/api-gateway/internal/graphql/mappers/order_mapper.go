package maps

import "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/orderpb"

func MapOrder(o *orderpb.Order) map[string]interface{} {
	if o == nil {
		return nil
	}
	items := make([]map[string]interface{}, len(o.Items))
	for i, item := range o.Items {
		items[i] = map[string]interface{}{
			"id":        item.Id,
			"productId": item.ProductId,
			"quantity":  item.Quantity,
			"price":     item.Price,
		}
	}
	return map[string]interface{}{
		"id":          o.Id,
		"userId":      o.UserId,
		"status":      o.Status,
		"items":       items,
		"totalAmount": o.TotalAmount,
		"createdAt":   o.CreatedAt,
	}
}
