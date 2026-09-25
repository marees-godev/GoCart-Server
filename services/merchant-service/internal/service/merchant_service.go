package service

import (
	"context"
	"log/slog"
	"regexp"
	"strings"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/repository"
)

var (
	// E.164 phone pattern: optionally starts with +, followed by 7-15 digits starting with 1-9
	phoneRegex = regexp.MustCompile(`^\+?[1-9]\d{6,14}$`)
	// Tax ID format: 3-50 alphanumeric and hyphen characters
	taxIDRegex = regexp.MustCompile(`^[A-Za-z0-9\-]{3,50}$`)
)

func validateBusinessName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", appErrors.BadRequest("business_name is required")
	}
	if len(trimmed) < 2 || len(trimmed) > 100 {
		return "", appErrors.BadRequest("business_name must be between 2 and 100 characters")
	}
	return trimmed, nil
}

func validateBusinessPhone(phone string) (string, error) {
	trimmed := strings.TrimSpace(phone)
	if trimmed == "" {
		return "", appErrors.BadRequest("business_phone is required")
	}
	if !phoneRegex.MatchString(trimmed) {
		return "", appErrors.BadRequest("business_phone must be in valid format (e.g., E.164 format)")
	}
	return trimmed, nil
}

func validateTaxID(taxID string) (string, error) {
	trimmed := strings.TrimSpace(taxID)
	if trimmed == "" {
		return "", appErrors.BadRequest("tax_id is required")
	}
	if !taxIDRegex.MatchString(trimmed) {
		return "", appErrors.BadRequest("tax_id must match valid tax identifier format")
	}
	return trimmed, nil
}

type MerchantService interface {
	GetMerchantByID(ctx context.Context, id uuid.UUID) (*model.Merchant, error)
	CreateMerchant(ctx context.Context, req dto.CreateMerchantRequest) (*model.Merchant, error)
	ListMerchants(ctx context.Context, limit, offset int, status string) ([]*model.Merchant, int, error)
	UpdateMerchant(ctx context.Context, id uuid.UUID, req dto.UpdateMerchantRequest) (*model.Merchant, error)
	UpdateMerchantStatus(ctx context.Context, id uuid.UUID, req dto.UpdateMerchantStatusRequest) (*model.Merchant, error)
	DeleteMerchant(ctx context.Context, id uuid.UUID) error
}

type merchantService struct {
	repo   repository.MerchantRepository
	logger *slog.Logger
}

func NewMerchantService(repo repository.MerchantRepository, log *slog.Logger) MerchantService {
	if log == nil {
		log = slog.Default()
	}
	return &merchantService{
		repo:   repo,
		logger: log,
	}
}

func (s *merchantService) GetMerchantByID(ctx context.Context, id uuid.UUID) (*model.Merchant, error) {
	if id == uuid.Nil {
		s.logger.Warn("GetMerchantByID failed: invalid nil UUID")
		return nil, appErrors.BadRequest("valid merchant ID is required")
	}
	s.logger.Info("Retrieving merchant profile", slog.String("merchant_id", id.String()))
	merchant, err := s.repo.GetByID(ctx, id)
	if err != nil {
		appErr := appErrors.AsAppError(err)
		if appErr != nil && appErr.Code == appErrors.CodeNotFound {
			s.logger.Warn("Merchant profile not found", slog.String("merchant_id", id.String()))
		} else {
			s.logger.Error("Failed to retrieve merchant profile", slog.String("merchant_id", id.String()), slog.Any("error", err))
		}
		return nil, err
	}
	s.logger.Info("Merchant profile retrieved successfully", slog.String("merchant_id", id.String()), slog.String("status", merchant.Status))
	return merchant, nil
}

func (s *merchantService) CreateMerchant(ctx context.Context, req dto.CreateMerchantRequest) (*model.Merchant, error) {
	idStr := strings.TrimSpace(req.ID)
	var merchantID uuid.UUID
	if idStr != "" {
		var err error
		merchantID, err = uuid.Parse(idStr)
		if err != nil {
			s.logger.Warn("CreateMerchant failed: invalid merchant ID format", slog.String("provided_id", idStr), slog.Any("error", err))
			return nil, appErrors.BadRequest("invalid merchant id format")
		}
		existing, err := s.repo.GetByID(ctx, merchantID)
		if err == nil && existing != nil {
			s.logger.Warn("CreateMerchant failed: merchant already exists", slog.String("merchant_id", merchantID.String()))
			return nil, appErrors.Conflict("merchant already exists")
		}
	} else {
		merchantID = uuid.New()
	}

	s.logger.Info("Creating merchant profile",
		slog.String("merchant_id", merchantID.String()),
		slog.String("business_email", req.BusinessEmail),
		slog.String("first_name", req.FirstName),
		slog.String("last_name", req.LastName),
	)

	merchant := &model.Merchant{
		ID:            merchantID,
		BusinessName:  "",
		FirstName:     strings.TrimSpace(req.FirstName),
		LastName:      strings.TrimSpace(req.LastName),
		BusinessEmail: strings.TrimSpace(req.BusinessEmail),
		BusinessPhone: "",
		TaxID:         "",
		Status:        string(model.MerchantStatusPending),
	}

	if err := s.repo.Create(ctx, merchant); err != nil {
		s.logger.Error("Failed to persist new merchant profile",
			slog.String("merchant_id", merchantID.String()),
			slog.Any("error", err),
		)
		return nil, err
	}

	s.logger.Info("Merchant profile created successfully",
		slog.String("merchant_id", merchantID.String()),
		slog.String("business_email", merchant.BusinessEmail),
		slog.String("status", merchant.Status),
	)
	return merchant, nil
}

func (s *merchantService) ListMerchants(ctx context.Context, limit, offset int, status string) ([]*model.Merchant, int, error) {
	statusFilter := strings.TrimSpace(status)
	s.logger.Info("Listing merchants",
		slog.Int("limit", limit),
		slog.Int("offset", offset),
		slog.String("status_filter", statusFilter),
	)

	merchants, total, err := s.repo.List(ctx, limit, offset, statusFilter)
	if err != nil {
		s.logger.Error("Failed to list merchants from repository",
			slog.Int("limit", limit),
			slog.Int("offset", offset),
			slog.String("status_filter", statusFilter),
			slog.Any("error", err),
		)
		return nil, 0, err
	}

	s.logger.Info("Merchants listed successfully",
		slog.Int("count", len(merchants)),
		slog.Int("total", total),
	)
	return merchants, total, nil
}

func (s *merchantService) UpdateMerchant(ctx context.Context, id uuid.UUID, req dto.UpdateMerchantRequest) (*model.Merchant, error) {
	if id == uuid.Nil {
		s.logger.Warn("UpdateMerchant failed: invalid nil UUID")
		return nil, appErrors.BadRequest("valid merchant ID is required")
	}

	s.logger.Info("Updating merchant business details",
		slog.String("merchant_id", id.String()),
		slog.String("business_name", req.BusinessName),
		slog.String("business_phone", req.BusinessPhone),
		slog.String("tax_id", req.TaxID),
	)

	validBusinessName, err := validateBusinessName(req.BusinessName)
	if err != nil {
		s.logger.Warn("UpdateMerchant validation failed for business_name",
			slog.String("merchant_id", id.String()),
			slog.Any("error", err),
		)
		return nil, err
	}

	validBusinessPhone, err := validateBusinessPhone(req.BusinessPhone)
	if err != nil {
		s.logger.Warn("UpdateMerchant validation failed for business_phone",
			slog.String("merchant_id", id.String()),
			slog.Any("error", err),
		)
		return nil, err
	}

	validTaxID, err := validateTaxID(req.TaxID)
	if err != nil {
		s.logger.Warn("UpdateMerchant validation failed for tax_id",
			slog.String("merchant_id", id.String()),
			slog.Any("error", err),
		)
		return nil, err
	}

	merchant, err := s.repo.GetByID(ctx, id)
	if err != nil {
		appErr := appErrors.AsAppError(err)
		if appErr != nil && appErr.Code == appErrors.CodeNotFound {
			s.logger.Warn("UpdateMerchant failed: merchant not found", slog.String("merchant_id", id.String()))
		} else {
			s.logger.Error("UpdateMerchant failed to load existing merchant",
				slog.String("merchant_id", id.String()),
				slog.Any("error", err),
			)
		}
		return nil, err
	}

	merchant.BusinessName = validBusinessName
	merchant.BusinessPhone = validBusinessPhone
	merchant.TaxID = validTaxID

	if err := s.repo.Update(ctx, merchant); err != nil {
		s.logger.Error("UpdateMerchant failed to save updates",
			slog.String("merchant_id", id.String()),
			slog.Any("error", err),
		)
		return nil, err
	}

	s.logger.Info("Merchant business details updated successfully",
		slog.String("merchant_id", id.String()),
		slog.String("business_name", merchant.BusinessName),
	)
	return merchant, nil
}

func (s *merchantService) UpdateMerchantStatus(ctx context.Context, id uuid.UUID, req dto.UpdateMerchantStatusRequest) (*model.Merchant, error) {
	if id == uuid.Nil {
		s.logger.Warn("UpdateMerchantStatus failed: invalid nil UUID")
		return nil, appErrors.BadRequest("valid merchant ID is required")
	}

	status := strings.ToUpper(strings.TrimSpace(req.Status))
	switch status {
	case string(model.MerchantStatusPending),
		string(model.MerchantStatusApproved),
		string(model.MerchantStatusRejected),
		string(model.MerchantStatusSuspended):
	default:
		s.logger.Warn("UpdateMerchantStatus failed: invalid status",
			slog.String("merchant_id", id.String()),
			slog.String("provided_status", req.Status),
		)
		return nil, appErrors.BadRequest("invalid merchant status, must be PENDING, APPROVED, REJECTED, or SUSPENDED")
	}

	reason := strings.TrimSpace(req.RejectionReason)
	s.logger.Info("Updating merchant status",
		slog.String("merchant_id", id.String()),
		slog.String("new_status", status),
		slog.String("rejection_reason", reason),
	)

	updated, err := s.repo.UpdateStatus(ctx, id, status, reason)
	if err != nil {
		s.logger.Error("UpdateMerchantStatus failed in repository",
			slog.String("merchant_id", id.String()),
			slog.String("status", status),
			slog.Any("error", err),
		)
		return nil, err
	}

	s.logger.Info("Merchant status updated successfully",
		slog.String("merchant_id", id.String()),
		slog.String("status", updated.Status),
	)
	return updated, nil
}

func (s *merchantService) DeleteMerchant(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		s.logger.Warn("DeleteMerchant failed: invalid nil UUID")
		return appErrors.BadRequest("valid merchant ID is required")
	}

	s.logger.Info("Deleting merchant profile", slog.String("merchant_id", id.String()))
	if err := s.repo.Delete(ctx, id); err != nil {
		s.logger.Error("DeleteMerchant failed in repository",
			slog.String("merchant_id", id.String()),
			slog.Any("error", err),
		)
		return err
	}

	s.logger.Info("Merchant profile deleted successfully", slog.String("merchant_id", id.String()))
	return nil
}
