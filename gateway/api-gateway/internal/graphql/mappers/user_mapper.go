package maps

import "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/userpb"

func MapUser(u *userpb.User) map[string]interface{} {
	if u == nil {
		return nil
	}
	return map[string]interface{}{
		"id":        u.Id,
		"email":     u.Email,
		"firstName": u.FirstName,
		"lastName":  u.LastName,
		"role":      u.Role,
		"createdAt": u.CreatedAt,
	}
}
