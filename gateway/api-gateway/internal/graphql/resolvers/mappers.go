package resolvers

import (
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
)

func toModelUser(u *userpb.User) *model.User {
	if u == nil {
		return nil
	}
	fn := u.FirstName
	ln := u.LastName
	ca := u.CreatedAt
	return &model.User{
		ID:        u.Id,
		Email:     u.Email,
		FirstName: &fn,
		LastName:  &ln,
		CreatedAt: &ca,
	}
}

func toModelAddress(a *userpb.Address) *model.Address {
	if a == nil {
		return nil
	}
	var label, fullName, phoneNumber, emailAddress, createdAt, updatedAt *string
	if a.Label != "" {
		l := a.Label
		label = &l
	}
	if a.FullName != "" {
		fn := a.FullName
		fullName = &fn
	}
	if a.PhoneNumber != "" {
		pn := a.PhoneNumber
		phoneNumber = &pn
	}
	if a.EmailAddress != "" {
		ea := a.EmailAddress
		emailAddress = &ea
	}
	if a.CreatedAt != "" {
		ca := a.CreatedAt
		createdAt = &ca
	}
	if a.UpdatedAt != "" {
		ua := a.UpdatedAt
		updatedAt = &ua
	}

	return &model.Address{
		ID:           a.Id,
		UserID:       a.UserId,
		Label:        label,
		FullName:     fullName,
		PhoneNumber:  phoneNumber,
		EmailAddress: emailAddress,
		AddressLine:  a.AddressLine,
		City:         a.City,
		State:        a.State,
		PostalCode:   a.PostalCode,
		Country:      a.Country,
		IsDefault:    a.IsDefault,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
}

func toModelAddressList(list []*userpb.Address) []*model.Address {
	if list == nil {
		return []*model.Address{}
	}
	res := make([]*model.Address, len(list))
	for i, a := range list {
		res[i] = toModelAddress(a)
	}
	return res
}
