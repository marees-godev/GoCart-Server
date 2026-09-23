package resolvers

import (
	"context"

	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	maps "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (r *Resolver) Me(ctx context.Context) (interface{}, error) {
	if r.Clients == nil || r.Clients.UserClient == nil {
		return nil, appErrors.Internal(nil, "user client unavailable")
	}
	userId, ok := ctx.Value("userID").(string)
	if !ok || userId == "" {
		return nil, appErrors.Unauthorized("authentication required")
	}

	res, err := r.Clients.UserClient.GetUser(ctx, &userpb.GetUserRequest{Id: userId})
	if err != nil {
		return nil, err
	}
	return maps.MapUser(res.User), nil
}
