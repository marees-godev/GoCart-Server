package dto

import (
	"regexp"
	"strings"
	"time"

	storepb "github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/model"
)

var (
	urlRegex            = regexp.MustCompile(`^(https?://|data:image/|/).+`)
	slugNonAlphaNumeric = regexp.MustCompile(`[^a-z0-9]+`)
)

type CreateStoreRequest struct {
	MerchantID         string  `json:"merchant_id,omitempty"`
	Name               string  `json:"name,omitempty"`
	Description        string  `json:"description,omitempty"`
	LogoURL            string  `json:"logo_url,omitempty"`
	BannerURL          string  `json:"banner_url,omitempty"`
	Address            string  `json:"address,omitempty"`
	BankAccountDetails *string `json:"bank_account_details,omitempty"`
	Slug               *string `json:"slug,omitempty"`
}

func (r *CreateStoreRequest) GetName() string {
	return strings.TrimSpace(r.Name)
}

func (r *CreateStoreRequest) GetDescription() string {
	return strings.TrimSpace(r.Description)
}

func (r *CreateStoreRequest) GetLogo() string {
	return strings.TrimSpace(r.LogoURL)
}

func (r *CreateStoreRequest) GetBanner() string {
	return strings.TrimSpace(r.BannerURL)
}

func (r *CreateStoreRequest) Validate() error {
	name := r.GetName()
	if name == "" {
		return errors.BadRequest("store name is required")
	}
	if len(name) < 2 || len(name) > 255 {
		return errors.BadRequest("store name must be between 2 and 255 characters")
	}
	if len(r.GetDescription()) > 2000 {
		return errors.BadRequest("store description must not exceed 2000 characters")
	}
	return nil
}

type UpdateStoreRequest struct {
	StoreID            *string `json:"store_id,omitempty"`
	ID                 *string `json:"id,omitempty"`
	MerchantID         *string `json:"merchant_id,omitempty"`
	Name               *string `json:"name,omitempty"`
	Description        *string `json:"description,omitempty"`
	LogoURL            *string `json:"logo_url,omitempty"`
	BannerURL          *string `json:"banner_url,omitempty"`
	Address            *string `json:"address,omitempty"`
	PublishStatus      *bool   `json:"publish_status,omitempty"`
	BankAccountDetails *string `json:"bank_account_details,omitempty"`
}

func (r *UpdateStoreRequest) GetName() *string {
	if r.Name != nil && strings.TrimSpace(*r.Name) != "" {
		trimmed := strings.TrimSpace(*r.Name)
		return &trimmed
	}
	return nil
}

func (r *UpdateStoreRequest) GetDescription() *string {
	return r.Description
}

func (r *UpdateStoreRequest) GetLogo() *string {
	return r.LogoURL
}

func (r *UpdateStoreRequest) GetBanner() *string {
	return r.BannerURL
}

func (r *UpdateStoreRequest) Validate() error {
	if name := r.GetName(); name != nil {
		if len(*name) < 2 || len(*name) > 255 {
			return errors.BadRequest("store name must be between 2 and 255 characters")
		}
	}
	if desc := r.GetDescription(); desc != nil && len(*desc) > 2000 {
		return errors.BadRequest("store description must not exceed 2000 characters")
	}
	return nil
}

type StoreResponse struct {
	ID                 string  `json:"id"`
	MerchantID         string  `json:"merchant_id"`
	Name               string  `json:"name"`
	Slug               string  `json:"slug"`
	Description        string  `json:"description"`
	LogoURL            string  `json:"logo_url"`
	BannerURL          string  `json:"banner_url"`
	Address            string  `json:"address"`
	ApprovalStatus     string  `json:"approval_status"`
	PublishStatus      bool    `json:"publish_status"`
	RejectionReason    *string `json:"rejection_reason,omitempty"`
	KYCStatus          string  `json:"kyc_status"`
	BankAccountDetails *string `json:"bank_account_details,omitempty"`
	AvgStoreRating     float64 `json:"avg_store_rating"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

func ToStoreResponse(s *model.Store) *StoreResponse {
	if s == nil {
		return nil
	}

	return &StoreResponse{
		ID:                 s.ID,
		MerchantID:         s.MerchantID,
		Name:               s.Name,
		Slug:               s.Slug,
		Description:        s.Description,
		LogoURL:            s.LogoURL,
		BannerURL:          s.BannerURL,
		Address:            s.Address,
		ApprovalStatus:     s.ApprovalStatus,
		PublishStatus:      s.PublishStatus,
		RejectionReason:    s.RejectionReason,
		KYCStatus:          s.KYCStatus,
		BankAccountDetails: s.BankAccountDetails,
		AvgStoreRating:     s.AvgStoreRating,
		CreatedAt:          s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          s.UpdatedAt.Format(time.RFC3339),
	}
}

func ToStorePB(s *model.Store) *storepb.Store {
	if s == nil {
		return nil
	}

	var rejectionReason string
	if s.RejectionReason != nil {
		rejectionReason = *s.RejectionReason
	}

	var bankDetails string
	if s.BankAccountDetails != nil {
		bankDetails = *s.BankAccountDetails
	}

	return &storepb.Store{
		Id:                 s.ID,
		MerchantId:         s.MerchantID,
		Name:               s.Name,
		Slug:               s.Slug,
		Description:        s.Description,
		LogoUrl:            s.LogoURL,
		BannerUrl:          s.BannerURL,
		Address:            s.Address,
		ApprovalStatus:     s.ApprovalStatus,
		PublishStatus:      s.PublishStatus,
		RejectionReason:    rejectionReason,
		KycStatus:          s.KYCStatus,
		BankAccountDetails: bankDetails,
		AvgStoreRating:     s.AvgStoreRating,
		CreatedAt:          s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          s.UpdatedAt.Format(time.RFC3339),
	}
}

func GenerateSlug(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = slugNonAlphaNumeric.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "store"
	}
	return slug
}

type BankAccount struct {
	AccountHolderName string  `json:"account_holder_name"`
	AccountNumber     string  `json:"account_number"`
	RoutingNumber     *string `json:"routing_number,omitempty"`
	BankName          string  `json:"bank_name"`
	AccountType       *string `json:"account_type,omitempty"`
	BranchCode        *string `json:"branch_code,omitempty"`
}

type GetUploadURLRequest struct {
	MerchantID  string `json:"merchant_id"`
	ImageType   string `json:"image_type"` // "logo" or "banner"
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
}

type GetUploadURLResponse struct {
	UploadURL        string `json:"upload_url"`
	PublicURL        string `json:"public_url"`
	Key              string `json:"key"`
	ExpiresInSeconds int32  `json:"expires_in_seconds"`
}
