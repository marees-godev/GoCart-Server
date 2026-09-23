package mutation

import (
	"context"

	"github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (m *MutationResolver) Login(ctx context.Context, email, password string) (interface{}, error) {
	if m.Clients == nil || m.Clients.AuthClient == nil {
		return nil, appErrors.Internal(nil, "auth client unavailable")
	}
	if email == "" || password == "" {
		return nil, appErrors.BadRequest("email and password are required")
	}

	res, err := m.Clients.AuthClient.Login(ctx, &auth.LoginRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"token": res.AccessToken,
		"user": map[string]interface{}{
			"id": res.UserId,
		},
	}, nil
}
