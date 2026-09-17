package resolvers

import (
	"github.com/graphql-go/graphql"
	maps "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/cartpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (r *Resolver) Cart(p graphql.ResolveParams) (interface{}, error) {
	if r.Clients == nil || r.Clients.CartClient == nil {
		return nil, appErrors.Internal(nil, "cart client unavailable")
	}
	userId, _ := p.Args["userId"].(string)
	if userId == "" {
		return nil, appErrors.BadRequest("userId is required")
	}

	res, err := r.Clients.CartClient.GetCart(p.Context, &cartpb.GetCartRequest{UserId: userId})
	if err != nil {
		return nil, err
	}
	return maps.MapCart(res.Cart), nil
}
