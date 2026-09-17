package mutation

import (
	"github.com/graphql-go/graphql"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/orderpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (m *MutationResolver) CreateOrder(p graphql.ResolveParams) (interface{}, error) {
	if m.Clients == nil || m.Clients.OrderClient == nil {
		return nil, appErrors.Internal(nil, "order client unavailable")
	}
	input, _ := p.Args["input"].(map[string]interface{})
	userID, _ := input["userId"].(string)
	cartID, _ := input["cartId"].(string)
	shippingAddress, _ := input["shippingAddress"].(string)

	if userID == "" || cartID == "" {
		return nil, appErrors.BadRequest("userId and cartId are required")
	}

	res, err := m.Clients.OrderClient.CreateOrder(p.Context, &orderpb.CreateOrderRequest{
		UserId:          userID,
		CartId:          cartID,
		ShippingAddress: shippingAddress,
	})
	if err != nil {
		return nil, err
	}
	return maps.MapOrder(res.Order), nil
}
