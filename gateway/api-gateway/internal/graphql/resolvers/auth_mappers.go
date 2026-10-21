package resolvers

import (
	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
)

func authRoleToAuthPBRole(r model.AuthRole) authpb.Role {
	switch r {
	case model.AuthRoleUser:
		return authpb.Role_USER
	case model.AuthRoleMerchant:
		return authpb.Role_MERCHANT
	case model.AuthRoleDeliveryPartner:
		return authpb.Role_DELIVERY_PARTNER
	default:
		return authpb.Role_ROLE_UNSPECIFIED
	}
}

func fromAuthPBRole(r authpb.Role) model.Role {
	switch r {
	case authpb.Role_USER:
		return model.RoleUser
	case authpb.Role_MERCHANT:
		return model.RoleMerchant
	case authpb.Role_DELIVERY_PARTNER:
		return model.RoleDeliveryPartner
	case authpb.Role_ADMIN:
		return model.RoleAdmin
	default:
		return ""
	}
}
