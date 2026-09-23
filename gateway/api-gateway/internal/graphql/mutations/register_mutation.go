package mutation

import (
	"context"

	"github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (m *MutationResolver) Register(ctx context.Context, email, password, firstName, lastName string) (interface{}, error) {
	if m.Clients == nil || m.Clients.AuthClient == nil {
		return nil, appErrors.Internal(nil, "auth client unavailable")
	}
	if email == "" || password == "" {
		return nil, appErrors.BadRequest("email and password are required")
	}

	res, err := m.Clients.AuthClient.Register(ctx, &auth.RegisterRequest{
		Email:     email,
		Password:  password,
		FirstName: firstName,
		LastName:  lastName,
	})
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"token": res.AccessToken,
		"user": map[string]interface{}{
			"id":        res.UserId,
			"email":     email,
			"firstName": firstName,
			"lastName":  lastName,
		},
	}, nil
}
