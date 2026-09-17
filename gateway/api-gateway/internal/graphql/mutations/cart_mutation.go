package mutation

import (
	"context"

	maps "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/cartpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (m *MutationResolver) AddToCart(ctx context.Context, userID, productID string, qty int) (interface{}, error) {
	if m.Clients == nil || m.Clients.CartClient == nil {
		return nil, appErrors.Internal(nil, "cart client unavailable")
	}
	if userID == "" || productID == "" || qty <= 0 {
		return nil, appErrors.BadRequest("userId, productId, and positive quantity are required")
	}

	res, err := m.Clients.CartClient.AddToCart(ctx, &cartpb.AddToCartRequest{
		UserId:    userID,
		ProductId: productID,
		Quantity:  int32(qty),
	})
	if err != nil {
		return nil, err
	}
	return maps.MapCart(res.Cart), nil
}
