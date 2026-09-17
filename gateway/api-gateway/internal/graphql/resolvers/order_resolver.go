package resolvers

import (
	"context"

	maps "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/orderpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (r *Resolver) Order(ctx context.Context, id string) (interface{}, error) {
	if r.Clients == nil || r.Clients.OrderClient == nil {
		return nil, appErrors.Internal(nil, "order client unavailable")
	}
	if id == "" {
		return nil, appErrors.BadRequest("order id is required")
	}

	res, err := r.Clients.OrderClient.GetOrder(ctx, &orderpb.GetOrderRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return maps.MapOrder(res.Order), nil
}
