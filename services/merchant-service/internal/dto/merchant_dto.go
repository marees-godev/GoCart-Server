package dto

import (
	"time"

	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
)

type CreateMerchantRequest struct {
	ID            string `json:"id,omitempty"`
	UserID        string `json:"user_id,omitempty"`
	BusinessName  string `json:"business_name"`
	FirstName     string `json:"first_name,omitempty"`
	LastName      string `json:"last_name,omitempty"`
	BusinessEmail string `json:"business_email,omitempty"`
	BusinessPhone string `json:"business_phone,omitempty"`
	TaxID         string `json:"tax_id,omitempty"`
}

type UpdateMerchantRequest struct {
	BusinessName  string `json:"business_name,omitempty"`
	FirstName     string `json:"first_name,omitempty"`
	LastName      string `json:"last_name,omitempty"`
	BusinessEmail string `json:"business_email,omitempty"`
	BusinessPhone string `json:"business_phone,omitempty"`
	TaxID         string `json:"tax_id,omitempty"`
}

type UpdateMerchantStatusRequest struct {
	Status          string `json:"status"`
	RejectionReason string `json:"rejection_reason,omitempty"`
}

type MerchantResponse struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id,omitempty"`
	BusinessName    string `json:"business_name"`
	FirstName       string `json:"first_name,omitempty"`
	LastName        string `json:"last_name,omitempty"`
	BusinessEmail   string `json:"business_email,omitempty"`
	BusinessPhone   string `json:"business_phone,omitempty"`
	TaxID           string `json:"tax_id,omitempty"`
	Status          string `json:"status"`
	RejectionReason string `json:"rejection_reason,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type MerchantListResponse struct {
	Merchants []MerchantResponse `json:"merchants"`
	Total     int                `json:"total"`
	Limit     int                `json:"limit"`
	Offset    int                `json:"offset"`
}

func ToMerchantResponse(m *model.Merchant) MerchantResponse {
	if m == nil {
		return MerchantResponse{}
	}
	return MerchantResponse{
		ID:              m.ID.String(),
		UserID:          m.UserID.String(),
		BusinessName:    m.BusinessName,
		FirstName:       m.FirstName,
		LastName:        m.LastName,
		BusinessEmail:   m.BusinessEmail,
		BusinessPhone:   m.BusinessPhone,
		TaxID:           m.TaxID,
		Status:          m.Status,
		RejectionReason: m.RejectionReason,
		CreatedAt:       m.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       m.UpdatedAt.Format(time.RFC3339),
	}
}

func ToMerchantListResponse(merchants []*model.Merchant, total, limit, offset int) MerchantListResponse {
	list := make([]MerchantResponse, len(merchants))
	for i, m := range merchants {
		list[i] = ToMerchantResponse(m)
	}
	return MerchantListResponse{
		Merchants: list,
		Total:     total,
		Limit:     limit,
		Offset:    offset,
	}
}
