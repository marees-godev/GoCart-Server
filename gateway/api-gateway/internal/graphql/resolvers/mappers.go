package resolvers

import (
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/userpb"
)

func toModelUser(u *userpb.User) *model.User {
	if u == nil {
		return nil
	}
	fn := u.FirstName
	ln := u.LastName
	role := u.Role
	ca := u.CreatedAt
	return &model.User{
		ID:        u.Id,
		Email:     u.Email,
		FirstName: &fn,
		LastName:  &ln,
		Role:      &role,
		CreatedAt: &ca,
	}
}

