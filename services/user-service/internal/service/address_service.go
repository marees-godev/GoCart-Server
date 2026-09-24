package service

import (
	"context"
	"log/slog"
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

func (s *addressService) getActiveUser(ctx context.Context, userID string) (*model.User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get user for status check", "user_id", userID, "error", err)
		return nil, err
	}
	if !strings.EqualFold(user.Status, "active") {
		slog.WarnContext(ctx, "user account is not active", "user_id", userID, "status", user.Status)
		return nil, errors.Forbidden("user account is not active")
	}
	return user, nil
}

func (s *addressService) CreateAddress(ctx context.Context, authUserID string, req dto.CreateAddressRequest) (*model.Address, error) {
	if strings.TrimSpace(authUserID) == "" {
		slog.WarnContext(ctx, "missing authenticated user context in CreateAddress")
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	if err := req.Validate(); err != nil {
		slog.WarnContext(ctx, "validation failed in CreateAddress", "user_id", authUserID, "error", err)
		return nil, err
	}

	user, err := s.getActiveUser(ctx, authUserID)
	if err != nil {
		return nil, err
	}

	count, err := s.repo.CountByUserID(ctx, authUserID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to count user addresses in CreateAddress", "user_id", authUserID, "error", err)
		return nil, err
	}

	isDefault := false
	if count == 0 {
		isDefault = true
	} else if req.IsDefault != nil {
		isDefault = *req.IsDefault
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
		slog.ErrorContext(ctx, "failed to create address in repository in CreateAddress", "user_id", authUserID, "error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "address created successfully", "address_id", addr.ID, "user_id", authUserID, "is_default", addr.IsDefault)
	return addr, nil
}

func (s *addressService) ListAddresses(ctx context.Context, authUserID string) ([]*model.Address, error) {
	if strings.TrimSpace(authUserID) == "" {
		slog.WarnContext(ctx, "missing authenticated user context in ListAddresses")
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	if _, err := s.getActiveUser(ctx, authUserID); err != nil {
		return nil, err
	}

	addresses, err := s.repo.ListByUserID(ctx, authUserID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list addresses in ListAddresses", "user_id", authUserID, "error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "addresses listed successfully", "user_id", authUserID, "count", len(addresses))
	return addresses, nil
}

func (s *addressService) GetAddress(ctx context.Context, authUserID string, addressID string) (*model.Address, error) {
	if strings.TrimSpace(authUserID) == "" {
		slog.WarnContext(ctx, "missing authenticated user context in GetAddress")
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	if strings.TrimSpace(addressID) == "" {
		slog.WarnContext(ctx, "missing address ID in GetAddress", "user_id", authUserID)
		return nil, errors.BadRequest("address ID is required")
	}

	if _, err := s.getActiveUser(ctx, authUserID); err != nil {
		return nil, err
	}

	addr, err := s.repo.GetByID(ctx, addressID)
	if err != nil {
		slog.WarnContext(ctx, "failed to get address in GetAddress", "user_id", authUserID, "address_id", addressID, "error", err)
		return nil, err
	}

	if addr.UserID != authUserID {
		slog.WarnContext(ctx, "forbidden address access in GetAddress", "auth_user_id", authUserID, "owner_user_id", addr.UserID, "address_id", addressID)
		return nil, errors.Forbidden("user cannot access another user's address")
	}

	slog.InfoContext(ctx, "address retrieved successfully", "address_id", addr.ID, "user_id", authUserID)
	return addr, nil
}

func (s *addressService) UpdateAddress(ctx context.Context, authUserID string, addressID string, req dto.UpdateAddressRequest) (*model.Address, error) {
	if strings.TrimSpace(authUserID) == "" {
		slog.WarnContext(ctx, "missing authenticated user context in UpdateAddress")
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	if strings.TrimSpace(addressID) == "" {
		slog.WarnContext(ctx, "missing address ID in UpdateAddress", "user_id", authUserID)
		return nil, errors.BadRequest("address ID is required")
	}

	if err := req.Validate(); err != nil {
		slog.WarnContext(ctx, "validation failed in UpdateAddress", "user_id", authUserID, "address_id", addressID, "error", err)
		return nil, err
	}

	if _, err := s.getActiveUser(ctx, authUserID); err != nil {
		return nil, err
	}

	existing, err := s.repo.GetByID(ctx, addressID)
	if err != nil {
		slog.WarnContext(ctx, "address not found for update in UpdateAddress", "user_id", authUserID, "address_id", addressID, "error", err)
		return nil, err
	}

	if existing.UserID != authUserID {
		slog.WarnContext(ctx, "forbidden address modification in UpdateAddress", "auth_user_id", authUserID, "owner_user_id", existing.UserID, "address_id", addressID)
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
		if !*req.IsDefault && existing.IsDefault {
			slog.WarnContext(ctx, "cannot unset default address directly in UpdateAddress", "user_id", authUserID, "address_id", addressID)
			return nil, errors.BadRequest("cannot unset default address; set another address as default instead")
		}
		existing.IsDefault = *req.IsDefault
	}

	if err := s.repo.Update(ctx, existing); err != nil {
		slog.ErrorContext(ctx, "failed to update address in repository in UpdateAddress", "user_id", authUserID, "address_id", addressID, "error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "address updated successfully", "address_id", existing.ID, "user_id", authUserID, "is_default", existing.IsDefault)
	return existing, nil
}

func (s *addressService) DeleteAddress(ctx context.Context, authUserID string, addressID string) error {
	if strings.TrimSpace(authUserID) == "" {
		slog.WarnContext(ctx, "missing authenticated user context in DeleteAddress")
		return errors.Unauthorized("authenticated user context is required")
	}

	if strings.TrimSpace(addressID) == "" {
		slog.WarnContext(ctx, "missing address ID in DeleteAddress", "user_id", authUserID)
		return errors.BadRequest("address ID is required")
	}

	if _, err := s.getActiveUser(ctx, authUserID); err != nil {
		return err
	}

	existing, err := s.repo.GetByID(ctx, addressID)
	if err != nil {
		slog.WarnContext(ctx, "address not found for deletion in DeleteAddress", "user_id", authUserID, "address_id", addressID, "error", err)
		return err
	}

	if existing.UserID != authUserID {
		slog.WarnContext(ctx, "forbidden address deletion in DeleteAddress", "auth_user_id", authUserID, "owner_user_id", existing.UserID, "address_id", addressID)
		return errors.Forbidden("user cannot delete another user's address")
	}

	if err := s.repo.Delete(ctx, addressID, authUserID); err != nil {
		slog.ErrorContext(ctx, "failed to delete address in repository in DeleteAddress", "user_id", authUserID, "address_id", addressID, "error", err)
		return err
	}

	slog.InfoContext(ctx, "address deleted successfully", "address_id", addressID, "user_id", authUserID)
	return nil
}

func (s *addressService) SetDefaultAddress(ctx context.Context, authUserID string, addressID string) (*model.Address, error) {
	if strings.TrimSpace(authUserID) == "" {
		slog.WarnContext(ctx, "missing authenticated user context in SetDefaultAddress")
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	if strings.TrimSpace(addressID) == "" {
		slog.WarnContext(ctx, "missing address ID in SetDefaultAddress", "user_id", authUserID)
		return nil, errors.BadRequest("address ID is required")
	}

	if _, err := s.getActiveUser(ctx, authUserID); err != nil {
		return nil, err
	}

	existing, err := s.repo.GetByID(ctx, addressID)
	if err != nil {
		slog.WarnContext(ctx, "address not found in SetDefaultAddress", "user_id", authUserID, "address_id", addressID, "error", err)
		return nil, err
	}

	if existing.UserID != authUserID {
		slog.WarnContext(ctx, "forbidden address default update in SetDefaultAddress", "auth_user_id", authUserID, "owner_user_id", existing.UserID, "address_id", addressID)
		return nil, errors.Forbidden("user cannot modify another user's address")
	}

	if err := s.repo.SetDefaultAddress(ctx, addressID, authUserID); err != nil {
		slog.ErrorContext(ctx, "failed to set default address in repository in SetDefaultAddress", "user_id", authUserID, "address_id", addressID, "error", err)
		return nil, err
	}

	updated, err := s.repo.GetByID(ctx, addressID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to fetch updated address in SetDefaultAddress", "user_id", authUserID, "address_id", addressID, "error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "default address set successfully", "address_id", addressID, "user_id", authUserID)
	return updated, nil
}
