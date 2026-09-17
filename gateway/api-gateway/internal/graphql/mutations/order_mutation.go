package mutation

import (
	"context"

	maps "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/orderpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (m *MutationResolver) CreateOrder(ctx context.Context, userID, cartID, shippingAddress string) (interface{}, error) {
	if m.Clients == nil || m.Clients.OrderClient == nil {
		return nil, appErrors.Internal(nil, "order client unavailable")
	}
	if userID == "" || cartID == "" {
		return nil, appErrors.BadRequest("userId and cartId are required")
	}

	res, err := m.Clients.OrderClient.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		UserId:          userID,
		CartId:          cartID,
		ShippingAddress: shippingAddress,
	})
	if err != nil {
		return nil, err
	}
	return maps.MapOrder(res.Order), nil
}
