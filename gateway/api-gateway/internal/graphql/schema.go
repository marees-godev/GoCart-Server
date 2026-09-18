package graphql

import (
	"context"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/generated"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	resolvers "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func NewSchema(res *resolvers.Resolver, cfg ...*config.Config) (graphql.ExecutableSchema, error) {
	if res == nil {
		res = resolvers.NewResolver(nil)
	}

	execConfig := generated.Config{
		Resolvers: res,
		Directives: generated.DirectiveRoot{
			Auth: func(ctx context.Context, obj interface{}, next graphql.Resolver, requires []model.Role) (interface{}, error) {
				user, ok := auth.FromContext(ctx)
				if !ok || user == nil || user.UserID == "" {
					return nil, appErrors.Unauthorized("authentication required")
				}

				if len(requires) > 0 {
					userRole := strings.ToUpper(strings.TrimSpace(user.Role))
					hasRole := false
					for _, reqRole := range requires {
						if userRole == string(reqRole) || strings.EqualFold(userRole, string(reqRole)) {
							hasRole = true
							break
						}
					}
					if !hasRole {
						return nil, appErrors.Forbidden("insufficient permissions for this operation")
					}
				}

				return next(ctx)
			},
		},
	}

	return generated.NewExecutableSchema(execConfig), nil
}
