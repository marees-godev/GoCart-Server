package dto

import (
	"regexp"
	"strings"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
)

var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	phoneRegex = regexp.MustCompile(`^\+?[0-9]{7,15}$`)
)

type CreateUserRequest struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

func (r *CreateUserRequest) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return errors.BadRequest("user ID is required")
	}
	trimmedEmail := strings.TrimSpace(r.Email)
	if trimmedEmail == "" || !emailRegex.MatchString(trimmedEmail) {
		return errors.BadRequest("invalid email address format")
	}
	if strings.TrimSpace(r.FirstName) == "" {
		return errors.BadRequest("first_name is required")
	}
	if strings.TrimSpace(r.LastName) == "" {
		return errors.BadRequest("last_name is required")
	}
	return nil
}

type UpdateUserRequest struct {
	Username       *string `json:"username,omitempty"`
	Email          *string `json:"email,omitempty"`
	EmailAddress   *string `json:"email_address,omitempty"`
	FirstName      *string `json:"first_name,omitempty"`
	LastName       *string `json:"last_name,omitempty"`
	PhoneNumber    *string `json:"phone_number,omitempty"`
	AlternatePhone *string `json:"alternate_phone,omitempty"`
	DateOfBirth    *string `json:"date_of_birth,omitempty"`
	Gender         *string `json:"gender,omitempty"`
	Bio            *string `json:"bio,omitempty"`
	About          *string `json:"about,omitempty"`
	AvatarURL      *string `json:"avatar_url,omitempty"`
}

func (r *UpdateUserRequest) GetEmail() *string {
	if r.EmailAddress != nil && *r.EmailAddress != "" {
		return r.EmailAddress
	}
	return r.Email
}

func (r *UpdateUserRequest) GetPhoneNumber() *string {
	if r.PhoneNumber != nil && *r.PhoneNumber != "" {
		return r.PhoneNumber
	}
	return r.PhoneNumber
}

func (r *UpdateUserRequest) GetBio() *string {
	if r.Bio != nil {
		return r.Bio
	}
	return r.About
}

func (r *UpdateUserRequest) Validate() error {
	email := r.GetEmail()
	if email != nil {
		trimmed := strings.TrimSpace(*email)
		if trimmed == "" || !emailRegex.MatchString(trimmed) {
			return errors.BadRequest("invalid email address format")
		}
	}

	if r.Username != nil {
		trimmed := strings.TrimSpace(*r.Username)
		if trimmed != "" && (len(trimmed) < 3 || len(trimmed) > 50) {
			return errors.BadRequest("username must be between 3 and 50 characters")
		}
	}

	phone := r.GetPhoneNumber()
	if phone != nil {
		trimmed := strings.TrimSpace(*phone)
		if trimmed != "" && !phoneRegex.MatchString(trimmed) {
			return errors.BadRequest("invalid phone number format")
		}
	}

	if r.AlternatePhone != nil {
		trimmed := strings.TrimSpace(*r.AlternatePhone)
		if trimmed != "" && !phoneRegex.MatchString(trimmed) {
			return errors.BadRequest("invalid alternate phone number format")
		}
	}

	if r.DateOfBirth != nil {
		trimmed := strings.TrimSpace(*r.DateOfBirth)
		if trimmed != "" {
			dob, err := time.Parse("2006-01-02", trimmed)
			if err != nil {
				return errors.BadRequest("date_of_birth must be formatted as YYYY-MM-DD")
			}
			if dob.After(time.Now()) {
				return errors.BadRequest("date_of_birth cannot be in the future")
			}
		}
	}

	bio := r.GetBio()
	if bio != nil && len(*bio) > 1000 {
		return errors.BadRequest("bio must not exceed 1000 characters")
	}

	if r.Gender != nil && len(*r.Gender) > 50 {
		return errors.BadRequest("gender field must not exceed 50 characters")
	}

	return nil
}

type UserResponse struct {
	UserID         string  `json:"user_id"`
	Username       *string `json:"username,omitempty"`
	EmailAddress   string  `json:"email_address"`
	FirstName      string  `json:"first_name"`
	LastName       string  `json:"last_name"`
	PhoneNumber    *string `json:"phone_number,omitempty"`
	AlternatePhone *string `json:"alternate_phone,omitempty"`
	DateOfBirth    *string `json:"date_of_birth,omitempty"`
	Gender         *string `json:"gender,omitempty"`
	Bio            *string `json:"bio,omitempty"`
	AvatarURL      *string `json:"avatar_url,omitempty"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	DeactivatedAt  *string `json:"deactivated_at,omitempty"`
	DeletedAt      *string `json:"deleted_at,omitempty"`
}

type DeactivateUserRequest struct {
	Reason *string `json:"reason,omitempty"`
}

type ReactivateUserRequest struct{}

type DeleteUserRequest struct {
	Reason *string `json:"reason,omitempty"`
}

type AccountActionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

func ToUserResponse(u *model.User) *UserResponse {
	if u == nil {
		return nil
	}

	var dobStr *string
	if u.DateOfBirth != nil {
		formatted := u.DateOfBirth.Format("2006-01-02")
		dobStr = &formatted
	}

	var deactivatedAtStr *string
	if u.DeactivatedAt != nil {
		formatted := u.DeactivatedAt.Format(time.RFC3339)
		deactivatedAtStr = &formatted
	}

	var deletedAtStr *string
	if u.DeletedAt != nil {
		formatted := u.DeletedAt.Format(time.RFC3339)
		deletedAtStr = &formatted
	}

	return &UserResponse{
		UserID:         u.ID,
		Username:       u.Username,
		EmailAddress:   u.Email,
		FirstName:      u.FirstName,
		LastName:       u.LastName,
		PhoneNumber:    u.PhoneNumber,
		AlternatePhone: u.AlternatePhone,
		DateOfBirth:    dobStr,
		Gender:         u.Gender,
		Bio:            u.Bio,
		AvatarURL:      u.AvatarURL,
		Status:         u.Status,
		CreatedAt:      u.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      u.UpdatedAt.Format(time.RFC3339),
		DeactivatedAt:  deactivatedAtStr,
		DeletedAt:      deletedAtStr,
	}
}
