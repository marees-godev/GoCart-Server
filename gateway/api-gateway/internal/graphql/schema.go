package graphql

import (
	"github.com/99designs/gqlgen/graphql"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/generated"
	resolvers "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
)

func NewSchema(res *resolvers.Resolver) (graphql.ExecutableSchema, error) {
	if res == nil {
		res = resolvers.NewResolver(nil)
	}
	return generated.NewExecutableSchema(generated.Config{
		Resolvers: res,
	}), nil
}
