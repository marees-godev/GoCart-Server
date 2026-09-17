package maps

import "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/cartpb"

func MapCart(c *cartpb.Cart) map[string]interface{} {
	if c == nil {
		return nil
	}
	items := make([]map[string]interface{}, len(c.Items))
	for i, item := range c.Items {
		items[i] = map[string]interface{}{
			"id":        item.Id,
			"productId": item.ProductId,
			"quantity":  item.Quantity,
			"unitPrice": item.UnitPrice,
		}
	}
	return map[string]interface{}{
		"id":          c.Id,
		"userId":      c.UserId,
		"items":       items,
		"totalAmount": c.TotalAmount,
	}
}
