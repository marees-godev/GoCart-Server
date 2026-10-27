package resolvers

import (
	"math"

	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (r *Resolver) getCartClient() (cartpb.CartServiceClient, error) {
	if r == nil {
		return nil, appErrors.Internal(nil, "cart service client unavailable")
	}
	if r.Clients != nil && r.Clients.CartClient != nil {
		return r.Clients.CartClient, nil
	}
	if r.ClientMgr != nil && r.ClientMgr.CartClient != nil {
		return r.ClientMgr.CartClient, nil
	}
	return nil, appErrors.Internal(nil, "cart service client unavailable")
}

func toModelCart(c *cartpb.Cart) *model.Cart {
	if c == nil {
		return nil
	}
	var totalCost float64
	var totalItems int
	items := make([]*model.CartItem, len(c.Items))
	for i, it := range c.Items {
		itemPrice := math.Round(it.UnitPrice*float64(it.Quantity)*100) / 100
		totalCost += itemPrice
		totalItems += int(it.Quantity)
		items[i] = &model.CartItem{
			ID:         it.Id,
			ProductID:  it.ProductId,
			Quantity:   int(it.Quantity),
			UnitPrice:  it.UnitPrice,
			TotalPrice: itemPrice,
		}
	}
	if c.TotalAmount > 0 {
		totalCost = math.Round(c.TotalAmount*100) / 100
	} else {
		totalCost = math.Round(totalCost*100) / 100
	}
	return &model.Cart{
		ID:          c.Id,
		UserID:      c.UserId,
		Items:       items,
		TotalAmount: totalCost,
		TotalItems:  totalItems,
	}
}
