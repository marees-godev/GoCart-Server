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

	var desc *string
	if s.Description != "" {
		desc = &s.Description
	}

	var logo *string
	if s.LogoUrl != "" {
		logo = &s.LogoUrl
	}

	var banner *string
	if s.BannerUrl != "" {
		banner = &s.BannerUrl
	}

	var addr *string
	if s.Address != "" {
		addr = &s.Address
	}

	var rej *string
	if s.RejectionReason != "" {
		rej = &s.RejectionReason
	}

	var bankRaw *string
	var bankAccount *model.BankAccount
	if s.BankAccountDetails != "" {
		bankRaw = &s.BankAccountDetails
		var parsed model.BankAccount
		if err := json.Unmarshal([]byte(s.BankAccountDetails), &parsed); err == nil {
			bankAccount = &parsed
		}
	}

	return &model.Store{
		ID:                 s.Id,
		MerchantID:         s.MerchantId,
		Name:               s.Name,
		Slug:               s.Slug,
		Description:        desc,
		LogoURL:            logo,
		BannerURL:          banner,
		Address:            addr,
		ApprovalStatus:     s.ApprovalStatus,
		PublishStatus:      s.PublishStatus,
		RejectionReason:    rej,
		KycStatus:          s.KycStatus,
		BankAccount:        bankAccount,
		BankAccountDetails: bankRaw,
		AvgStoreRating:     s.AvgStoreRating,
		CreatedAt:          s.CreatedAt,
		UpdatedAt:          s.UpdatedAt,
	}
}

func SerializeBankAccount(account *model.BankAccountInput, rawDetails *string) string {
	if account != nil {
		bytes, err := json.Marshal(account)
		if err == nil {
			return string(bytes)
		}
	}
	if rawDetails != nil {
		return *rawDetails
	}
	return ""
}

