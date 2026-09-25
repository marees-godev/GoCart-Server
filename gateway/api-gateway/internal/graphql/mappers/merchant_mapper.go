package maps

import (
	"time"

	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
)

func ToGraphQLMerchant(m *merchantpb.MerchantResponseData) *model.Merchant {
	if m == nil {
		return nil
	}
	res := &model.Merchant{
		ID:           m.Id,
		BusinessName: m.BusinessName,
		Status:       m.Status,
	}
	if m.FirstName != "" {
		fn := m.FirstName
		res.FirstName = &fn
	}
	if m.LastName != "" {
		ln := m.LastName
		res.LastName = &ln
	}
	if m.CreatedAt != nil {
		createdAt := m.CreatedAt.AsTime().Format(time.RFC3339)
		res.CreatedAt = &createdAt
	}
	if m.UpdatedAt != nil {
		updatedAt := m.UpdatedAt.AsTime().Format(time.RFC3339)
		res.UpdatedAt = &updatedAt
	}
	if m.DeletedAt != nil {
		deletedAt := m.DeletedAt.AsTime().Format(time.RFC3339)
		res.DeletedAt = &deletedAt
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
