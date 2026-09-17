package resolvers

import (
	"github.com/graphql-go/graphql"
	maps "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/userpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (r *Resolver) User(p graphql.ResolveParams) (interface{}, error) {
	if r.Clients == nil || r.Clients.UserClient == nil {
		return nil, appErrors.Internal(nil, "user client unavailable")
	}
	id, _ := p.Args["id"].(string)
	if id == "" {
		return nil, appErrors.BadRequest("user id is required")
	}

	res, err := r.Clients.UserClient.GetUser(p.Context, &userpb.GetUserRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return maps.MapUser(res.User), nil
}
