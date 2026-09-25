package service

import (
	"context"
	"log/slog"
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
	DeactivateUser(ctx context.Context, authUserID, targetUserID string, req dto.DeactivateUserRequest) (*dto.AccountActionResponse, error)
	ReactivateUser(ctx context.Context, authUserID, targetUserID string) (*dto.AccountActionResponse, error)
	DeleteUser(ctx context.Context, authUserID, targetUserID string, req dto.DeleteUserRequest) (*dto.AccountActionResponse, error)
	ProcessExpiredDeactivations(ctx context.Context, retentionPeriod time.Duration) (int, error)
}

type userService struct {
	repo repository.UserRepository
}

func NewUserService(repo repository.UserRepository) UserService {
	return &userService{repo: repo}
}

func (s *userService) CreateUser(ctx context.Context, req dto.CreateUserRequest) (*model.User, error) {
	if err := req.Validate(); err != nil {
		slog.WarnContext(ctx, "validation failed in CreateUser", "error", err)
		return nil, err
	}

	existing, err := s.repo.GetByEmail(ctx, req.Email)
	if err == nil && existing != nil {
		slog.WarnContext(ctx, "user with email already exists in CreateUser", "email", req.Email)
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
		slog.ErrorContext(ctx, "failed to create user in repository in CreateUser", "user_id", req.ID, "email", req.Email, "error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "user created successfully in CreateUser", "user_id", newUser.ID, "email", newUser.Email)
	return newUser, nil
}

func (s *userService) GetUser(ctx context.Context, authUserID, targetUserID string) (*model.User, error) {
	if strings.TrimSpace(authUserID) == "" {
		slog.WarnContext(ctx, "missing authenticated user context in GetUser")
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	idToFetch := strings.TrimSpace(targetUserID)
	if idToFetch == "" {
		idToFetch = strings.TrimSpace(authUserID)
	}

	if idToFetch == "" {
		slog.WarnContext(ctx, "missing user ID in GetUser")
		return nil, errors.BadRequest("user_id is required")
	}

	if !dto.IsValidID(idToFetch) {
		slog.WarnContext(ctx, "invalid user ID in GetUser", "user_id", idToFetch)
		return nil, errors.BadRequest("user_id is invalid")
	}

	user, err := s.repo.GetByID(ctx, idToFetch)
	if err != nil {
		slog.WarnContext(ctx, "failed to get user in GetUser", "user_id", idToFetch, "error", err)
		return nil, err
	}

	if !strings.EqualFold(user.Status, "active") {
		if strings.EqualFold(user.Status, "deactivated") && user.DeactivatedAt != nil && time.Since(*user.DeactivatedAt) >= 30*24*time.Hour {
			reason := "Automatic permanent deletion after 30 days of deactivation"
			_ = s.repo.DeleteUser(ctx, user.ID, "SYSTEM", &reason)
			slog.WarnContext(ctx, "deactivation grace period expired; permanently deleted", "user_id", idToFetch)
			return nil, errors.Forbidden("account deactivation period of 30 days has expired; account has been permanently deleted")
		}
		slog.WarnContext(ctx, "user account not active in GetUser", "user_id", idToFetch, "status", user.Status)
		return nil, errors.Forbidden("user account is not active")
	}

	slog.InfoContext(ctx, "user fetched successfully in GetUser", "user_id", user.ID)
	return user, nil
}

func (s *userService) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		slog.WarnContext(ctx, "missing user ID in GetUserByID")
		return nil, errors.BadRequest("user_id is required")
	}

	if !dto.IsValidID(trimmedID) {
		slog.WarnContext(ctx, "invalid user ID in GetUserByID", "user_id", id)
		return nil, errors.BadRequest("user_id is invalid")
	}

	user, err := s.repo.GetByID(ctx, trimmedID)
	if err != nil {
		slog.WarnContext(ctx, "failed to get user in GetUserByID", "user_id", id, "error", err)
		return nil, err
	}

	if !strings.EqualFold(user.Status, "active") {
		if strings.EqualFold(user.Status, "deactivated") && user.DeactivatedAt != nil && time.Since(*user.DeactivatedAt) >= 30*24*time.Hour {
			reason := "Automatic permanent deletion after 30 days of deactivation"
			_ = s.repo.DeleteUser(ctx, user.ID, "SYSTEM", &reason)
			slog.WarnContext(ctx, "deactivation grace period expired; permanently deleted", "user_id", id)
			return nil, errors.Forbidden("account deactivation period of 30 days has expired; account has been permanently deleted")
		}
		slog.WarnContext(ctx, "user account not active in GetUserByID", "user_id", id, "status", user.Status)
		return nil, errors.Forbidden("user account is not active")
	}

	slog.InfoContext(ctx, "user fetched successfully in GetUserByID", "user_id", user.ID)
	return user, nil
}

func (s *userService) UpdateUser(ctx context.Context, authUserID, targetUserID string, req dto.UpdateUserRequest) (*model.User, error) {
	if strings.TrimSpace(authUserID) == "" {
		slog.WarnContext(ctx, "missing authenticated user context in UpdateUser")
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	if strings.TrimSpace(targetUserID) == "" {
		targetUserID = authUserID
	}

	targetUserID = strings.TrimSpace(targetUserID)
	authUserID = strings.TrimSpace(authUserID)

	if !dto.IsValidID(targetUserID) {
		slog.WarnContext(ctx, "invalid user id in UpdateUser", "target_user_id", targetUserID)
		return nil, errors.BadRequest("user_id is invalid")
	}

	if authUserID != targetUserID {
		slog.WarnContext(ctx, "user id mismatch in UpdateUser", "auth_user_id", authUserID, "target_user_id", targetUserID)
		return nil, errors.BadRequest("user_id does not match the expected value")
	}

	if req.ID != nil && strings.TrimSpace(*req.ID) != "" && strings.TrimSpace(*req.ID) != targetUserID {
		slog.WarnContext(ctx, "id in body does not match target user in UpdateUser", "body_id", *req.ID, "target_user_id", targetUserID)
		return nil, errors.BadRequest("user_id does not match the expected value")
	}

	if req.UserID != nil && strings.TrimSpace(*req.UserID) != "" && strings.TrimSpace(*req.UserID) != targetUserID {
		slog.WarnContext(ctx, "user_id in body does not match target user in UpdateUser", "body_user_id", *req.UserID, "target_user_id", targetUserID)
		return nil, errors.BadRequest("user_id does not match the expected value")
	}

	if err := req.Validate(); err != nil {
		slog.WarnContext(ctx, "validation failed in UpdateUser", "user_id", targetUserID, "error", err)
		return nil, err
	}

	user, err := s.repo.GetByID(ctx, targetUserID)
	if err != nil {
		slog.WarnContext(ctx, "failed to get user for update in UpdateUser", "user_id", targetUserID, "error", err)
		return nil, err
	}

	if !strings.EqualFold(user.Status, "active") {
		slog.WarnContext(ctx, "user account not active in UpdateUser", "user_id", targetUserID, "status", user.Status)
		return nil, errors.Forbidden("user account is not active")
	}

	if newEmail := req.GetEmail(); newEmail != nil && *newEmail != "" && !strings.EqualFold(*newEmail, user.Email) {
		existing, err := s.repo.GetByEmail(ctx, *newEmail)
		if err == nil && existing != nil && existing.ID != user.ID {
			slog.WarnContext(ctx, "email already in use in UpdateUser", "user_id", targetUserID, "email", *newEmail)
			return nil, errors.Conflict("email address is already in use")
		}
		user.Email = *newEmail
	}

	if req.Username != nil && *req.Username != "" && (user.Username == nil || *user.Username != *req.Username) {
		existing, err := s.repo.GetByUsername(ctx, *req.Username)
		if err == nil && existing != nil && existing.ID != user.ID {
			slog.WarnContext(ctx, "username already in use in UpdateUser", "user_id", targetUserID, "username", *req.Username)
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
				slog.WarnContext(ctx, "invalid date_of_birth format in UpdateUser", "user_id", targetUserID, "error", err)
				return nil, errors.BadRequest("invalid date_of_birth format")
			}
			user.DateOfBirth = &dob
		}
	}

	if req.Gender != nil {
		user.Gender = req.GetGender()
	}

	if bio := req.GetBio(); bio != nil {
		user.Bio = bio
	}

	if req.AvatarURL != nil {
		user.AvatarURL = req.AvatarURL
	}

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		slog.ErrorContext(ctx, "failed to update user in repository in UpdateUser", "user_id", targetUserID, "error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "user updated successfully in UpdateUser", "user_id", user.ID)
	return user, nil
}

func (s *userService) DeactivateUser(ctx context.Context, authUserID, targetUserID string, req dto.DeactivateUserRequest) (*dto.AccountActionResponse, error) {
	if strings.TrimSpace(authUserID) == "" {
		slog.WarnContext(ctx, "missing authenticated user context in DeactivateUser")
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	if strings.TrimSpace(targetUserID) == "" {
		targetUserID = authUserID
	}

	targetUserID = strings.TrimSpace(targetUserID)
	authUserID = strings.TrimSpace(authUserID)

	if !dto.IsValidID(targetUserID) {
		slog.WarnContext(ctx, "invalid user id in DeactivateUser", "target_user_id", targetUserID)
		return nil, errors.BadRequest("user_id is invalid")
	}

	if authUserID != targetUserID {
		slog.WarnContext(ctx, "user id mismatch in DeactivateUser", "auth_user_id", authUserID, "target_user_id", targetUserID)
		return nil, errors.BadRequest("user_id does not match the expected value")
	}

	if err := s.repo.DeactivateUser(ctx, targetUserID, authUserID, req.Reason); err != nil {
		slog.ErrorContext(ctx, "failed to deactivate user in repository", "user_id", targetUserID, "error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "account deactivated successfully", "user_id", targetUserID)
	return &dto.AccountActionResponse{
		Success: true,
		Message: "account deactivated successfully",
		Status:  "deactivated",
	}, nil
}

func (s *userService) DeleteUser(ctx context.Context, authUserID, targetUserID string, req dto.DeleteUserRequest) (*dto.AccountActionResponse, error) {
	if strings.TrimSpace(authUserID) == "" {
		slog.WarnContext(ctx, "missing authenticated user context in DeleteUser")
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	if strings.TrimSpace(targetUserID) == "" {
		targetUserID = authUserID
	}

	targetUserID = strings.TrimSpace(targetUserID)
	authUserID = strings.TrimSpace(authUserID)

	if !dto.IsValidID(targetUserID) {
		slog.WarnContext(ctx, "invalid user id in DeleteUser", "target_user_id", targetUserID)
		return nil, errors.BadRequest("user_id is invalid")
	}

	if authUserID != targetUserID {
		slog.WarnContext(ctx, "user id mismatch in DeleteUser", "auth_user_id", authUserID, "target_user_id", targetUserID)
		return nil, errors.BadRequest("user_id does not match the expected value")
	}

	if err := s.repo.DeleteUser(ctx, targetUserID, authUserID, req.Reason); err != nil {
		slog.ErrorContext(ctx, "failed to delete user in repository", "user_id", targetUserID, "error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "account deleted successfully", "user_id", targetUserID)
	return &dto.AccountActionResponse{
		Success: true,
		Message: "account deleted successfully",
		Status:  "deleted",
	}, nil
}

func (s *userService) ReactivateUser(ctx context.Context, authUserID, targetUserID string) (*dto.AccountActionResponse, error) {
	if strings.TrimSpace(authUserID) == "" {
		slog.WarnContext(ctx, "missing authenticated user context in ReactivateUser")
		return nil, errors.Unauthorized("authenticated user context is required")
	}

	if strings.TrimSpace(targetUserID) == "" {
		targetUserID = authUserID
	}

	targetUserID = strings.TrimSpace(targetUserID)
	authUserID = strings.TrimSpace(authUserID)

	if !dto.IsValidID(targetUserID) {
		slog.WarnContext(ctx, "invalid user id in ReactivateUser", "target_user_id", targetUserID)
		return nil, errors.BadRequest("user_id is invalid")
	}

	if authUserID != targetUserID {
		slog.WarnContext(ctx, "user id mismatch in ReactivateUser", "auth_user_id", authUserID, "target_user_id", targetUserID)
		return nil, errors.BadRequest("user_id does not match the expected value")
	}

	if err := s.repo.ReactivateUser(ctx, targetUserID, authUserID); err != nil {
		slog.ErrorContext(ctx, "failed to reactivate user in repository", "user_id", targetUserID, "error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "account reactivated successfully", "user_id", targetUserID)
	return &dto.AccountActionResponse{
		Success: true,
		Message: "account reactivated successfully",
		Status:  "active",
	}, nil
}

func (s *userService) ProcessExpiredDeactivations(ctx context.Context, retentionPeriod time.Duration) (int, error) {
	if retentionPeriod <= 0 {
		retentionPeriod = 30 * 24 * time.Hour
	}
	cutoff := time.Now().UTC().Add(-retentionPeriod)
	userIDs, err := s.repo.GetExpiredDeactivatedUserIDs(ctx, cutoff)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get expired deactivated users", "cutoff", cutoff, "error", err)
		return 0, err
	}

	processedCount := 0
	reason := "Automatic permanent deletion after 30 days of deactivation"
	for _, id := range userIDs {
		if err := s.repo.DeleteUser(ctx, id, "SYSTEM", &reason); err != nil {
			slog.ErrorContext(ctx, "failed to automatically soft-delete expired deactivated user", "user_id", id, "error", err)
			continue
		}
		processedCount++
	}

	slog.InfoContext(ctx, "processed expired deactivations", "count", processedCount, "total_found", len(userIDs))
	return processedCount, nil
}
