package mutation

import (
	"github.com/graphql-go/graphql"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mappers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/userpb"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (m *MutationResolver) Register(p graphql.ResolveParams) (interface{}, error) {
	if m.Clients == nil || m.Clients.UserClient == nil {
		return nil, appErrors.Internal(nil, "user client unavailable")
	}
	input, _ := p.Args["input"].(map[string]interface{})
	email, _ := input["email"].(string)
	password, _ := input["password"].(string)
	firstName, _ := input["firstName"].(string)
	lastName, _ := input["lastName"].(string)

	if email == "" || password == "" {
		return nil, appErrors.BadRequest("email and password are required")
	}

	res, err := m.Clients.UserClient.Register(p.Context, &userpb.RegisterRequest{
		Email:     email,
		Password:  password,
		FirstName: firstName,
		LastName:  lastName,
	})
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"token": res.Token,
		"user":  maps.MapUser(res.User),
	}, nil
}
