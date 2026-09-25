package resolvers

import (
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
)

func toModelUser(u *userpb.User) *model.User {
	if u == nil {
		return nil
	}
	fn := u.FirstName
	ln := u.LastName
	ca := u.CreatedAt
	st := u.Status
	var ph *string
	if u.Phone != "" {
		p := u.Phone
		ph = &p
	}
	var gender *model.Gender
	if u.Gender != nil && *u.Gender != "" {
		g := model.Gender(*u.Gender)
		if g.IsValid() {
			gender = &g
		}
	}
	return &model.User{
		ID:             u.Id,
		Email:          u.Email,
		FirstName:      &fn,
		LastName:       &ln,
		Phone:          ph,
		Username:       u.Username,
		AlternatePhone: u.AlternatePhone,
		DateOfBirth:    u.DateOfBirth,
		Gender:         gender,
		Bio:            u.Bio,
		AvatarURL:      u.AvatarUrl,
		Status:         &st,
		CreatedAt:      &ca,
		UpdatedAt:      u.UpdatedAt,
	}
}

func toModelMerchant(m *merchantpb.Merchant) *model.Merchant {
	if m == nil {
		return nil
	}
	res := &model.Merchant{
		ID:           m.Id,
		MerchantID:   m.Id,
		BusinessName: m.BusinessName,
		Status:       m.Status,
	}
	if m.UserId != "" {
		uid := m.UserId
		res.UserID = &uid
	}
	if m.FirstName != "" {
		fn := m.FirstName
		res.FirstName = &fn
	}
	if m.LastName != "" {
		ln := m.LastName
		res.LastName = &ln
	}
	if m.CreatedAt != "" {
		ca := m.CreatedAt
		res.CreatedAt = &ca
	}
	if m.UpdatedAt != "" {
		ua := m.UpdatedAt
		res.UpdatedAt = &ua
	}
	if m.TaxId != "" {
		tid := m.TaxId
		res.TaxID = &tid
	}
	if m.BusinessEmail != "" {
		be := m.BusinessEmail
		res.BusinessEmail = &be
	}
	if m.BusinessPhone != "" {
		bp := m.BusinessPhone
		res.BusinessPhone = &bp
	}
	if m.RejectionReason != "" {
		rr := m.RejectionReason
		res.RejectionReason = &rr
	}
	return res
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
