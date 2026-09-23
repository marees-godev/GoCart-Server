package service

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/repository"
)

type MerchantService interface {
	GetMerchantByID(ctx context.Context, id uuid.UUID) (*model.Merchant, error)
	GetMerchantByUserID(ctx context.Context, userID uuid.UUID) (*model.Merchant, error)
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
		return nil, appErrors.BadRequest("valid merchant ID is required")
	}
	return s.repo.GetByID(ctx, id)
}

func (s *merchantService) GetMerchantByUserID(ctx context.Context, userID uuid.UUID) (*model.Merchant, error) {
	if userID == uuid.Nil {
		return nil, appErrors.BadRequest("valid user ID is required")
	}
	return s.repo.GetByUserID(ctx, userID)
}

func (s *merchantService) CreateMerchant(ctx context.Context, req dto.CreateMerchantRequest) (*model.Merchant, error) {
	businessName := strings.TrimSpace(req.BusinessName)
	if businessName == "" {
		businessName = strings.TrimSpace(req.FirstName + " " + req.LastName)
	}
	if businessName == "" {
		businessName = strings.TrimSpace(req.BusinessEmail)
	}
	if businessName == "" {
		return nil, appErrors.BadRequest("business name is required")
	}

	userUIDStr := strings.TrimSpace(req.UserID)
	var userUUID uuid.UUID
	if userUIDStr != "" {
		var err error
		userUUID, err = uuid.Parse(userUIDStr)
		if err != nil {
			return nil, appErrors.BadRequest("invalid user id format")
		}
		existing, err := s.repo.GetByUserID(ctx, userUUID)
		if err == nil && existing != nil {
			return existing, nil
		}
	}

	idStr := strings.TrimSpace(req.ID)
	var merchantID uuid.UUID
	if idStr != "" {
		var err error
		merchantID, err = uuid.Parse(idStr)
		if err != nil {
			return nil, appErrors.BadRequest("invalid merchant id format")
		}
		existing, err := s.repo.GetByID(ctx, merchantID)
		if err == nil && existing != nil {
			return nil, appErrors.Conflict("merchant already exists")
		}
	} else {
		merchantID = uuid.New()
	}

	if userUUID == uuid.Nil {
		userUUID = merchantID
	}

	merchant := &model.Merchant{
		ID:            merchantID,
		UserID:        userUUID,
		BusinessName:  businessName,
		FirstName:     strings.TrimSpace(req.FirstName),
		LastName:      strings.TrimSpace(req.LastName),
		BusinessEmail: strings.TrimSpace(req.BusinessEmail),
		BusinessPhone: strings.TrimSpace(req.BusinessPhone),
		TaxID:         strings.TrimSpace(req.TaxID),
		Status:        string(model.MerchantStatusPending),
	}

	if err := s.repo.Create(ctx, merchant); err != nil {
		return nil, err
	}

	return merchant, nil
}

func (s *merchantService) ListMerchants(ctx context.Context, limit, offset int, status string) ([]*model.Merchant, int, error) {
	return s.repo.List(ctx, limit, offset, strings.TrimSpace(status))
}

func (s *merchantService) UpdateMerchant(ctx context.Context, id uuid.UUID, req dto.UpdateMerchantRequest) (*model.Merchant, error) {
	if id == uuid.Nil {
		return nil, appErrors.BadRequest("valid merchant ID is required")
	}

	merchant, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.BusinessName) != "" {
		merchant.BusinessName = strings.TrimSpace(req.BusinessName)
	}
	if strings.TrimSpace(req.FirstName) != "" {
		merchant.FirstName = strings.TrimSpace(req.FirstName)
	}
	if strings.TrimSpace(req.LastName) != "" {
		merchant.LastName = strings.TrimSpace(req.LastName)
	}
	if strings.TrimSpace(req.BusinessEmail) != "" {
		merchant.BusinessEmail = strings.TrimSpace(req.BusinessEmail)
	}
	if strings.TrimSpace(req.BusinessPhone) != "" {
		merchant.BusinessPhone = strings.TrimSpace(req.BusinessPhone)
	}
	if strings.TrimSpace(req.TaxID) != "" {
		merchant.TaxID = strings.TrimSpace(req.TaxID)
	}

	if err := s.repo.Update(ctx, merchant); err != nil {
		return nil, err
	}

	return merchant, nil
}

func (s *merchantService) UpdateMerchantStatus(ctx context.Context, id uuid.UUID, req dto.UpdateMerchantStatusRequest) (*model.Merchant, error) {
	if id == uuid.Nil {
		return nil, appErrors.BadRequest("valid merchant ID is required")
	}

	status := strings.ToUpper(strings.TrimSpace(req.Status))
	switch status {
	case string(model.MerchantStatusPending),
		string(model.MerchantStatusApproved),
		string(model.MerchantStatusRejected):
	default:
		return nil, appErrors.BadRequest("invalid merchant status, must be PENDING, APPROVED, or REJECTED")
	}

	reason := strings.TrimSpace(req.RejectionReason)
	return s.repo.UpdateStatus(ctx, id, status, reason)
}

func (s *merchantService) DeleteMerchant(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return appErrors.BadRequest("valid merchant ID is required")
	}
	return s.repo.Delete(ctx, id)
}
