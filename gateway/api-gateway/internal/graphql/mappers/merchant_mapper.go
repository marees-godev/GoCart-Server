package maps

import (
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
)

func ToGraphQLMerchant(m *merchantpb.Merchant) *model.Merchant {
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
		createdAt := m.CreatedAt
		res.CreatedAt = &createdAt
	}
	if m.UpdatedAt != "" {
		updatedAt := m.UpdatedAt
		res.UpdatedAt = &updatedAt
	}
	if m.TaxId != "" {
		taxId := m.TaxId
		res.TaxID = &taxId
	}
	if m.BusinessEmail != "" {
		email := m.BusinessEmail
		res.BusinessEmail = &email
	}
	if m.BusinessPhone != "" {
		phone := m.BusinessPhone
		res.BusinessPhone = &phone
	}
	if m.RejectionReason != "" {
		reason := m.RejectionReason
		res.RejectionReason = &reason
	}
	return res
}
