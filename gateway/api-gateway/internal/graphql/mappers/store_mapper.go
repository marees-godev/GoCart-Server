package maps

import (
	"encoding/json"

	storepb "github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
)

func MapStore(s *storepb.Store) *model.Store {
	if s == nil {
		return nil
	}

	var email *string
	if s.BusinessEmail != "" {
		email = &s.BusinessEmail
	}

	var phone *string
	if s.BusinessPhone != "" {
		phone = &s.BusinessPhone
	}

	var desc *string
	if s.Description != "" {
		desc = &s.Description
	}

	var logo *string
	if s.LogoUrl != "" {
		logo = &s.LogoUrl
	}

	var addr *string
	if s.Address != "" {
		addr = &s.Address
	}

	var rej *string
	if s.RejectionReason != "" {
		rej = &s.RejectionReason
	}

	var bankAccount *model.BankAccount
	if s.BankAccountDetails != "" {
		var parsed model.BankAccount
		if err := json.Unmarshal([]byte(s.BankAccountDetails), &parsed); err == nil {
			bankAccount = &parsed
		}
	}

	var kycStat *string
	if s.KycStatus != "" {
		kycStat = &s.KycStatus
	}

	var gstin *string
	if s.Gstin != "" {
		gstin = &s.Gstin
	}

	return &model.Store{
		ID:              s.Id,
		MerchantID:      s.MerchantId,
		Name:            s.Name,
		Slug:            s.Slug,
		BusinessEmail:   email,
		BusinessPhone:   phone,
		Description:     desc,
		LogoURL:         logo,
		Address:         addr,
		IsVacationMode:  s.IsVacationMode,
		ApprovalStatus:  s.ApprovalStatus,
		RejectionReason: rej,
		IsPublished:     s.IsPublished,
		KycStatus:       kycStat,
		Gstin:           gstin,
		BankAccount:          bankAccount,
		AvgStoreRating:       s.AvgStoreRating,
		CreatedAt:            s.CreatedAt,
		UpdatedAt:            s.UpdatedAt,
	}
}

func SerializeBankAccount(account *model.BankAccountInput) string {
	if account != nil {
		bytes, err := json.Marshal(account)
		if err == nil {
			return string(bytes)
		}
	}
	return ""
}

func MapStoreAppeal(a *storepb.StoreAppeal) *model.StoreAppeal {
	if a == nil {
		return nil
	}
	var adminComment *string
	if a.AdminComment != "" {
		adminComment = &a.AdminComment
	}
	var reviewedAt *string
	if a.ReviewedAt != "" {
		reviewedAt = &a.ReviewedAt
	}

	return &model.StoreAppeal{
		ID:           a.Id,
		StoreID:      a.StoreId,
		MerchantID:   a.MerchantId,
		Reason:       a.Reason,
		Status:       a.Status,
		AdminComment: adminComment,
		ReviewedAt:   reviewedAt,
		CreatedAt:    a.CreatedAt,
		UpdatedAt:    a.UpdatedAt,
	}
}
