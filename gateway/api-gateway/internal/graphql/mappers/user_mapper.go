package maps

import userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"

func MapUser(u *userpb.User) map[string]interface{} {
	if u == nil {
		return nil
	}
	return map[string]interface{}{
		"id":        u.Id,
		"email":     u.Email,
		"firstName": u.FirstName,
		"lastName":  u.LastName,
		"createdAt": u.CreatedAt,
	}
}
