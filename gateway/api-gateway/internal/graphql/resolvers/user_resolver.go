package resolvers

import (
	"context"

	maps "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/userpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (r *Resolver) User(ctx context.Context, id string) (interface{}, error) {
	if r.Clients == nil || r.Clients.UserClient == nil {
		return nil, appErrors.Internal(nil, "user client unavailable")
	}
	if id == "" {
		return nil, appErrors.BadRequest("user id is required")
	}

	res, err := r.Clients.UserClient.GetUser(ctx, &userpb.GetUserRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return maps.MapUser(res.User), nil
}
