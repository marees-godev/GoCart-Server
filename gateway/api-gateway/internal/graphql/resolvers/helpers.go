package resolvers

import (
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/cartpb"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/orderpb"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/productpb"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/userpb"
)

func toModelUser(u *userpb.User) *model.User {
	if u == nil {
		return nil
	}
	fn := u.FirstName
	ln := u.LastName
	role := u.Role
	ca := u.CreatedAt
	return &model.User{
		ID:        u.Id,
		Email:     u.Email,
		FirstName: &fn,
		LastName:  &ln,
		Role:      &role,
		CreatedAt: &ca,
	}
}

func toModelProduct(p *productpb.Product) *model.Product {
	if p == nil {
		return nil
	}
	desc := p.Description
	catID := p.CategoryId
	ca := p.CreatedAt
	return &model.Product{
		ID:            p.Id,
		Name:          p.Name,
		Description:   &desc,
		Price:         p.Price,
		CategoryID:    &catID,
		StockQuantity: int(p.StockQuantity),
		CreatedAt:     &ca,
	}
}

func toModelCart(c *cartpb.Cart) *model.Cart {
	if c == nil {
		return nil
	}
	items := make([]*model.CartItem, len(c.Items))
	for i, item := range c.Items {
		items[i] = &model.CartItem{
			ID:        item.Id,
			ProductID: item.ProductId,
			Quantity:  int(item.Quantity),
			UnitPrice: item.UnitPrice,
		}
	}
	return &model.Cart{
		ID:          c.Id,
		UserID:      c.UserId,
		Items:       items,
		TotalAmount: c.TotalAmount,
	}
}

func toModelOrder(o *orderpb.Order) *model.Order {
	if o == nil {
		return nil
	}
	items := make([]*model.OrderItem, len(o.Items))
	for i, item := range o.Items {
		items[i] = &model.OrderItem{
			ID:        item.Id,
			ProductID: item.ProductId,
			Quantity:  int(item.Quantity),
			Price:     item.Price,
		}
	}
	ca := o.CreatedAt
	return &model.Order{
		ID:          o.Id,
		UserID:      o.UserId,
		Status:      o.Status,
		Items:       items,
		TotalAmount: o.TotalAmount,
		CreatedAt:   &ca,
	}
}
