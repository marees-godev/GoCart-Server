package service

import (
	"context"
	"strings"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/repository"
)

type UserService interface {
	CreateUser(ctx context.Context, req dto.CreateUserRequest) (*model.User, error)
	GetUser(ctx context.Context, authUserID, targetUserID string) (*model.User, error)
	GetUserByID(ctx context.Context, id string) (*model.User, error)
	UpdateUser(ctx context.Context, authUserID, targetUserID string, req dto.UpdateUserRequest) (*model.User, error)
}

type userService struct {
	repo repository.UserRepository
}

func NewUserService(repo repository.UserRepository) UserService {
	return &userService{repo: repo}
}

func (s *userService) CreateUser(ctx context.Context, req dto.CreateUserRequest) (*model.User, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	existing, err := s.repo.GetByEmail(ctx, req.Email)
	if err == nil && existing != nil {
		return nil, errors.Conflict("user with this email already exists")
	}


	newUser := &model.User{
		ID:        req.ID,
		Email:     strings.TrimSpace(req.Email),
		FirstName: strings.TrimSpace(req.FirstName),
		LastName:  strings.TrimSpace(req.LastName),
		Status:    "active",
	}

	if err := s.repo.CreateUser(ctx, newUser); err != nil {
		return nil, err
	}

	return newUser, nil
}

func (s *userService) GetUser(ctx context.Context, authUserID, targetUserID string) (*model.User, error) {
	idToFetch := targetUserID
	if idToFetch == "" {
		idToFetch = authUserID
	}

	if idToFetch == "" {
		return nil, errors.BadRequest("user ID is required")
	}

	user, err := s.repo.GetByID(ctx, idToFetch)
	if err != nil {
		return nil, err
	}

	if !strings.EqualFold(user.Status, "active") {
		return nil, errors.Forbidden("user account is not active")
	}

	return user, nil
}

func (s *userService) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.BadRequest("user ID is required")
	}

	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if !strings.EqualFold(user.Status, "active") {
		return nil, errors.Forbidden("user account is not active")
	}

	return user, nil
}

func (s *userService) UpdateUser(ctx context.Context, authUserID, targetUserID string, req dto.UpdateUserRequest) (*model.User, error) {
	if authUserID == "" {
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	if targetUserID == "" {
		targetUserID = authUserID
	}

	if authUserID != targetUserID {
		return nil, errors.Forbidden("user cannot modify another user's profile")
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	user, err := s.repo.GetByID(ctx, targetUserID)
	if err != nil {
		return nil, err
	}

	if !strings.EqualFold(user.Status, "active") {
		return nil, errors.Forbidden("user account is not active")
	}

	if newEmail := req.GetEmail(); newEmail != nil && *newEmail != "" && !strings.EqualFold(*newEmail, user.Email) {
		existing, err := s.repo.GetByEmail(ctx, *newEmail)
		if err == nil && existing != nil && existing.ID != user.ID {
			return nil, errors.Conflict("email address is already in use")
		}
		user.Email = *newEmail
	}

	if req.Username != nil && *req.Username != "" && (user.Username == nil || *user.Username != *req.Username) {
		existing, err := s.repo.GetByUsername(ctx, *req.Username)
		if err == nil && existing != nil && existing.ID != user.ID {
			return nil, errors.Conflict("username is already in use")
		}
		user.Username = req.Username
	}

	if req.FirstName != nil {
		user.FirstName = strings.TrimSpace(*req.FirstName)
	}

	if req.LastName != nil {
		user.LastName = strings.TrimSpace(*req.LastName)
	}

	if phonenumber := req.GetPhoneNumber(); phonenumber != nil {
		user.PhoneNumber = phonenumber
	}

	if req.AlternatePhone != nil {
		user.AlternatePhone = req.AlternatePhone
	}

	if req.DateOfBirth != nil {
		trimmed := strings.TrimSpace(*req.DateOfBirth)
		if trimmed == "" {
			user.DateOfBirth = nil
		} else {
			dob, err := time.Parse("2006-01-02", trimmed)
			if err != nil {
				return nil, errors.BadRequest("invalid date_of_birth format")
			}
			user.DateOfBirth = &dob
		}
	}

	if req.Gender != nil {
		user.Gender = req.Gender
	}

	if bio := req.GetBio(); bio != nil {
		user.Bio = bio
	}

	if req.AvatarURL != nil {
		user.AvatarURL = req.AvatarURL
	}

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}
