package service

import (
	"context"
	"strings"

	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/repository"
)

type AddressService interface {
	CreateAddress(ctx context.Context, authUserID string, req dto.CreateAddressRequest) (*model.Address, error)
	ListAddresses(ctx context.Context, authUserID string) ([]*model.Address, error)
	GetAddress(ctx context.Context, authUserID string, addressID string) (*model.Address, error)
	UpdateAddress(ctx context.Context, authUserID string, addressID string, req dto.UpdateAddressRequest) (*model.Address, error)
	DeleteAddress(ctx context.Context, authUserID string, addressID string) error
	SetDefaultAddress(ctx context.Context, authUserID string, addressID string) (*model.Address, error)
}

type addressService struct {
	repo     repository.AddressRepository
	userRepo repository.UserRepository
}

func NewAddressService(repo repository.AddressRepository, userRepo repository.UserRepository) AddressService {
	return &addressService{
		repo:     repo,
		userRepo: userRepo,
	}
}

func normalizeAddressLabel(labelInput *string) *string {
	if labelInput == nil || strings.TrimSpace(*labelInput) == "" {
		defaultLabel := "Home"
		return &defaultLabel
	}
	trimmed := strings.TrimSpace(*labelInput)
	switch strings.ToLower(trimmed) {
	case "home":
		normalized := "Home"
		return &normalized
	case "work":
		normalized := "Work"
		return &normalized
	case "other":
		normalized := "Other"
		return &normalized
	default:
		return &trimmed
	}
}

func (s *addressService) CreateAddress(ctx context.Context, authUserID string, req dto.CreateAddressRequest) (*model.Address, error) {
	if strings.TrimSpace(authUserID) == "" {
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	user, err := s.userRepo.GetByID(ctx, authUserID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(user.Status, "active") {
		return nil, errors.Forbidden("user account is not active")
	}

	count, err := s.repo.CountByUserID(ctx, authUserID)
	if err != nil {
		return nil, err
	}

	isDefault := false
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
	} else if count == 0 {
		isDefault = true
	}

	if isDefault {
		if err := s.repo.ClearDefaultAddresses(ctx, authUserID); err != nil {
			return nil, err
		}
	}

	fullName := strings.TrimSpace(req.FullName)
	phoneNumber := strings.TrimSpace(req.PhoneNumber)

	var emailAddress *string
	if req.EmailAddress != nil && strings.TrimSpace(*req.EmailAddress) != "" {
		trimmed := strings.TrimSpace(*req.EmailAddress)
		emailAddress = &trimmed
	} else if strings.TrimSpace(user.Email) != "" {
		trimmedUserEmail := strings.TrimSpace(user.Email)
		emailAddress = &trimmedUserEmail
	}

	label := normalizeAddressLabel(req.Label)

	addr := &model.Address{
		UserID:       authUserID,
		Label:        label,
		FullName:     &fullName,
		PhoneNumber:  &phoneNumber,
		EmailAddress: emailAddress,
		AddressLine:  strings.TrimSpace(req.AddressLine),
		City:         strings.TrimSpace(req.City),
		State:        strings.TrimSpace(req.State),
		PostalCode:   strings.TrimSpace(req.PostalCode),
		Country:      strings.TrimSpace(req.Country),
		IsDefault:    isDefault,
	}

	if err := s.repo.Create(ctx, addr); err != nil {
		return nil, err
	}

	return addr, nil
}

func (s *addressService) ListAddresses(ctx context.Context, authUserID string) ([]*model.Address, error) {
	if strings.TrimSpace(authUserID) == "" {
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	return s.repo.ListByUserID(ctx, authUserID)
}

func (s *addressService) GetAddress(ctx context.Context, authUserID string, addressID string) (*model.Address, error) {
	if strings.TrimSpace(authUserID) == "" {
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	if strings.TrimSpace(addressID) == "" {
		return nil, errors.BadRequest("address ID is required")
	}

	addr, err := s.repo.GetByID(ctx, addressID)
	if err != nil {
		return nil, err
	}

	if addr.UserID != authUserID {
		return nil, errors.Forbidden("user cannot access another user's address")
	}

	return addr, nil
}

func (s *addressService) UpdateAddress(ctx context.Context, authUserID string, addressID string, req dto.UpdateAddressRequest) (*model.Address, error) {
	if strings.TrimSpace(authUserID) == "" {
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	if strings.TrimSpace(addressID) == "" {
		return nil, errors.BadRequest("address ID is required")
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	existing, err := s.repo.GetByID(ctx, addressID)
	if err != nil {
		return nil, err
	}

	if existing.UserID != authUserID {
		return nil, errors.Forbidden("user cannot modify another user's address")
	}

	if req.Label != nil {
		existing.Label = normalizeAddressLabel(req.Label)
	}

	if req.FullName != nil {
		trimmed := strings.TrimSpace(*req.FullName)
		existing.FullName = &trimmed
	}

	if req.PhoneNumber != nil {
		trimmed := strings.TrimSpace(*req.PhoneNumber)
		existing.PhoneNumber = &trimmed
	}

	if req.EmailAddress != nil {
		existing.EmailAddress = req.EmailAddress
	}

	if req.AddressLine != nil {
		line := strings.TrimSpace(*req.AddressLine)
		existing.AddressLine = line
	}

	if req.City != nil {
		existing.City = strings.TrimSpace(*req.City)
	}

	if req.State != nil {
		existing.State = strings.TrimSpace(*req.State)
	}

	if req.PostalCode != nil {
		existing.PostalCode = strings.TrimSpace(*req.PostalCode)
	}

	if req.Country != nil {
		existing.Country = strings.TrimSpace(*req.Country)
	}

	if req.IsDefault != nil {
		if *req.IsDefault && !existing.IsDefault {
			if err := s.repo.ClearDefaultAddresses(ctx, authUserID); err != nil {
				return nil, err
			}
		}
		existing.IsDefault = *req.IsDefault
	}

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	return existing, nil
}

func (s *addressService) DeleteAddress(ctx context.Context, authUserID string, addressID string) error {
	if strings.TrimSpace(authUserID) == "" {
		return errors.Unauthorized("authenticated user context is required")
	}

	if strings.TrimSpace(addressID) == "" {
		return errors.BadRequest("address ID is required")
	}

	existing, err := s.repo.GetByID(ctx, addressID)
	if err != nil {
		return err
	}

	if existing.UserID != authUserID {
		return errors.Forbidden("user cannot delete another user's address")
	}

	return s.repo.Delete(ctx, addressID, authUserID)
}

func (s *addressService) SetDefaultAddress(ctx context.Context, authUserID string, addressID string) (*model.Address, error) {
	if strings.TrimSpace(authUserID) == "" {
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	if strings.TrimSpace(addressID) == "" {
		return nil, errors.BadRequest("address ID is required")
	}

	existing, err := s.repo.GetByID(ctx, addressID)
	if err != nil {
		return nil, err
	}

	if existing.UserID != authUserID {
		return nil, errors.Forbidden("user cannot modify another user's address")
	}

	if err := s.repo.SetDefaultAddress(ctx, addressID, authUserID); err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, addressID)
}
