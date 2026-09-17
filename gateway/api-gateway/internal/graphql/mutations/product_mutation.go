package mutation

import (
	"github.com/graphql-go/graphql"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/productpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (m *MutationResolver) CreateProduct(p graphql.ResolveParams) (interface{}, error) {
	if m.Clients == nil || m.Clients.ProductClient == nil {
		return nil, appErrors.Internal(nil, "product client unavailable")
	}
	input, _ := p.Args["input"].(map[string]interface{})
	name, _ := input["name"].(string)
	desc, _ := input["description"].(string)
	price, _ := input["price"].(float64)
	categoryID, _ := input["categoryId"].(string)
	stockQty, _ := input["stockQuantity"].(int)

	if name == "" || price <= 0 {
		return nil, appErrors.BadRequest("valid product name and positive price are required")
	}

	res, err := m.Clients.ProductClient.CreateProduct(p.Context, &productpb.CreateProductRequest{
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
