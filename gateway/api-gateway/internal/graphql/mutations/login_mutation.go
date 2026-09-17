package mutation

import (
	"context"

	maps "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/userpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (m *MutationResolver) Login(ctx context.Context, email, password string) (interface{}, error) {
	if m.Clients == nil || m.Clients.UserClient == nil {
		return nil, appErrors.Internal(nil, "user client unavailable")
	}
	if email == "" || password == "" {
		return nil, appErrors.BadRequest("email and password are required")
	}

	res, err := m.Clients.UserClient.Login(ctx, &userpb.LoginRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"token": res.Token,
		"user":  maps.MapUser(res.User),
	}, nil
}
