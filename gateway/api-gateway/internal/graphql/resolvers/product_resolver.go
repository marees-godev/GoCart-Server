package resolvers

import (
	"github.com/graphql-go/graphql"
	maps "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/productpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (r *Resolver) Product(p graphql.ResolveParams) (interface{}, error) {
	if r.Clients == nil || r.Clients.ProductClient == nil {
		return nil, appErrors.Internal(nil, "product client unavailable")
	}
	id, _ := p.Args["id"].(string)
	if id == "" {
		return nil, appErrors.BadRequest("product id is required")
	}

	res, err := r.Clients.ProductClient.GetProduct(p.Context, &productpb.GetProductRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return maps.MapProduct(res.Product), nil
}
