package resolvers

import (
	"github.com/graphql-go/graphql"
	maps "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/userpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (r *Resolver) Me(p graphql.ResolveParams) (interface{}, error) {
	if r.Clients == nil || r.Clients.UserClient == nil {
		return nil, appErrors.Internal(nil, "user client unavailable")
	}
	userId, ok := p.Context.Value("userID").(string)
	if !ok || userId == "" {
		return nil, appErrors.Unauthorized("authentication required")
	}

	res, err := r.Clients.UserClient.GetUser(p.Context, &userpb.GetUserRequest{Id: userId})
	if err != nil {
		return nil, err
	}
	return maps.MapUser(res.User), nil
}
