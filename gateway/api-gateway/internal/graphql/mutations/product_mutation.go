package mutation

import (
	"context"

	maps "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/productpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (m *MutationResolver) CreateProduct(ctx context.Context, name, desc string, price float64, categoryID string, stockQty int) (interface{}, error) {
	if m.Clients == nil || m.Clients.ProductClient == nil {
		return nil, appErrors.Internal(nil, "product client unavailable")
	}
	if name == "" || price <= 0 {
		return nil, appErrors.BadRequest("valid product name and positive price are required")
	}

	res, err := m.Clients.ProductClient.CreateProduct(ctx, &productpb.CreateProductRequest{
		Name:          name,
		Description:   desc,
		Price:         price,
		CategoryId:    categoryID,
		StockQuantity: int32(stockQty),
	})
	if err != nil {
		return nil, err
	}
	return maps.MapProduct(res.Product), nil
}
