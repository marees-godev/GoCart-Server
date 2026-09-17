package mutation

import (
	"github.com/graphql-go/graphql"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/cartpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (m *MutationResolver) AddToCart(p graphql.ResolveParams) (interface{}, error) {
	if m.Clients == nil || m.Clients.CartClient == nil {
		return nil, appErrors.Internal(nil, "cart client unavailable")
	}
	input, _ := p.Args["input"].(map[string]interface{})
	userID, _ := input["userId"].(string)
	productID, _ := input["productId"].(string)
	qty, _ := input["quantity"].(int)

	if userID == "" || productID == "" || qty <= 0 {
		return nil, appErrors.BadRequest("userId, productId, and positive quantity are required")
	}

	res, err := m.Clients.CartClient.AddToCart(p.Context, &cartpb.AddToCartRequest{
		UserId:    userID,
		ProductId: productID,
		Quantity:  int32(qty),
	})
	if err != nil {
		return nil, err
	}
	return maps.MapCart(res.Cart), nil
}
