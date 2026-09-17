package resolvers

import (
	"github.com/graphql-go/graphql"
	maps "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/productpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (r *Resolver) Products(p graphql.ResolveParams) (interface{}, error) {
	if r.Clients == nil || r.Clients.ProductClient == nil {
		return nil, appErrors.Internal(nil, "product client unavailable")
	}
	limit, _ := p.Args["limit"].(int)
	offset, _ := p.Args["offset"].(int)
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	res, err := r.Clients.ProductClient.ListProducts(p.Context, &productpb.ListProductsRequest{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, err
	}

	products := make([]map[string]interface{}, len(res.Products))
	for i, prod := range res.Products {
		products[i] = maps.MapProduct(prod)
	}
	return products, nil
}
