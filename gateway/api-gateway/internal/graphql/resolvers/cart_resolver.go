package resolvers

import (
	"context"

	maps "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/cartpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (r *Resolver) Cart(ctx context.Context, userId string) (interface{}, error) {
	if r.Clients == nil || r.Clients.CartClient == nil {
		return nil, appErrors.Internal(nil, "cart client unavailable")
	}
	if userId == "" {
		return nil, appErrors.BadRequest("userId is required")
	}

	res, err := r.Clients.CartClient.GetCart(ctx, &cartpb.GetCartRequest{UserId: userId})
	if err != nil {
		return nil, err
	}
	return maps.MapCart(res.Cart), nil
}
