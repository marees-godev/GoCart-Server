package dto

import (
	"regexp"
	"strings"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
)

var (
	postalCodeRegex   = regexp.MustCompile(`^[a-zA-Z0-9\s\-]{3,20}$`)
	addressPhoneRegex = regexp.MustCompile(`^\+?[0-9\s\-\(\)]{7,25}$`)
	addressEmailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
)

type CreateAddressRequest struct {
	Label        *string `json:"label,omitempty"`
	FullName     string  `json:"full_name"`
	PhoneNumber  string  `json:"phone_number"`
	EmailAddress *string `json:"email_address,omitempty"`
	AddressLine  string  `json:"address_line"`
	City         string  `json:"city"`
	State        string  `json:"state"`
	PostalCode   string  `json:"postal_code"`
	Country      string  `json:"country"`
	IsDefault    *bool   `json:"is_default,omitempty"`
}

func (r *CreateAddressRequest) Validate() error {
	fullName := strings.TrimSpace(r.FullName)
	if fullName == "" {
		return errors.BadRequest("full_name is required")
	}
	if len(fullName) > 100 {
		return errors.BadRequest("full_name must not exceed 100 characters")
	}

	phoneNumber := strings.TrimSpace(r.PhoneNumber)
	if phoneNumber == "" {
		return errors.BadRequest("phone_number is required")
	}
	if !addressPhoneRegex.MatchString(phoneNumber) {
		return errors.BadRequest("invalid phone_number format")
	}

	if r.EmailAddress != nil && strings.TrimSpace(*r.EmailAddress) != "" {
		emailAddress := strings.TrimSpace(*r.EmailAddress)
		if !addressEmailRegex.MatchString(emailAddress) {
			return errors.BadRequest("invalid email_address format")
		}
	}

	addressLine := strings.TrimSpace(r.AddressLine)
	if addressLine == "" {
		return errors.BadRequest("address_line is required")
	}
	if len(addressLine) > 255 {
		return errors.BadRequest("address_line must not exceed 255 characters")
	}

	city := strings.TrimSpace(r.City)
	if city == "" {
		return errors.BadRequest("city is required")
	}
	if len(city) > 100 {
		return errors.BadRequest("city must not exceed 100 characters")
	}

	state := strings.TrimSpace(r.State)
	if state == "" {
		return errors.BadRequest("state is required")
	}
	if len(state) > 100 {
		return errors.BadRequest("state must not exceed 100 characters")
	}

	postalCode := strings.TrimSpace(r.PostalCode)
	if postalCode == "" {
		return errors.BadRequest("postal_code is required")
	}
	if !postalCodeRegex.MatchString(postalCode) {
		return errors.BadRequest("invalid postal_code format")
	}

	country := strings.TrimSpace(r.Country)
	if country == "" {
		return errors.BadRequest("country is required")
	}
	if len(country) > 100 {
		return errors.BadRequest("country must not exceed 100 characters")
	}

	if r.Label != nil && len(*r.Label) > 50 {
		return errors.BadRequest("label must not exceed 50 characters")
	}

	return nil
}

type UpdateAddressRequest struct {
	Label        *string `json:"label,omitempty"`
	FullName     *string `json:"full_name,omitempty"`
	PhoneNumber  *string `json:"phone_number,omitempty"`
	EmailAddress *string `json:"email_address,omitempty"`
	AddressLine  *string `json:"address_line,omitempty"`
	City         *string `json:"city,omitempty"`
	State        *string `json:"state,omitempty"`
	PostalCode   *string `json:"postal_code,omitempty"`
	Country      *string `json:"country,omitempty"`
	IsDefault    *bool   `json:"is_default,omitempty"`
}

func (r *UpdateAddressRequest) Validate() error {
	if r.FullName != nil {
		fullName := strings.TrimSpace(*r.FullName)
		if fullName == "" {
			return errors.BadRequest("full_name cannot be empty")
		}
		if len(fullName) > 100 {
			return errors.BadRequest("full_name must not exceed 100 characters")
		}
	}

	if r.PhoneNumber != nil {
		phoneNumber := strings.TrimSpace(*r.PhoneNumber)
		if phoneNumber == "" || !addressPhoneRegex.MatchString(phoneNumber) {
			return errors.BadRequest("invalid phone_number format")
		}
	}

	if r.EmailAddress != nil && strings.TrimSpace(*r.EmailAddress) != "" {
		emailAddress := strings.TrimSpace(*r.EmailAddress)
		if !addressEmailRegex.MatchString(emailAddress) {
			return errors.BadRequest("invalid email_address format")
		}
	}

	if r.AddressLine != nil {
		addressLine := strings.TrimSpace(*r.AddressLine)
		if addressLine == "" {
			return errors.BadRequest("address_line cannot be empty")
		}
		if len(addressLine) > 255 {
			return errors.BadRequest("address_line must not exceed 255 characters")
		}
	}

	if r.City != nil {
		city := strings.TrimSpace(*r.City)
		if city == "" {
			return errors.BadRequest("city cannot be empty")
		}
		if len(city) > 100 {
			return errors.BadRequest("city must not exceed 100 characters")
		}
	}

	if r.State != nil {
		state := strings.TrimSpace(*r.State)
		if state == "" {
			return errors.BadRequest("state cannot be empty")
		}
		if len(state) > 100 {
			return errors.BadRequest("state must not exceed 100 characters")
		}
	}

	if r.PostalCode != nil {
		postalCode := strings.TrimSpace(*r.PostalCode)
		if postalCode == "" || !postalCodeRegex.MatchString(postalCode) {
			return errors.BadRequest("invalid postal_code format")
		}
	}

	if r.Country != nil {
		country := strings.TrimSpace(*r.Country)
		if country == "" {
			return errors.BadRequest("country cannot be empty")
		}
		if len(country) > 100 {
			return errors.BadRequest("country must not exceed 100 characters")
		}
	}

	if r.Label != nil && len(*r.Label) > 50 {
		return errors.BadRequest("label must not exceed 50 characters")
	}

	return nil
}

type AddressResponse struct {
	AddressID    string  `json:"address_id"`
	ID           string  `json:"id"`
	UserID       string  `json:"user_id"`
	Label        *string `json:"label,omitempty"`
	FullName     *string `json:"full_name,omitempty"`
	PhoneNumber  *string `json:"phone_number,omitempty"`
	EmailAddress *string `json:"email_address,omitempty"`
	AddressLine  string  `json:"address_line"`
	City         string  `json:"city"`
	State        string  `json:"state"`
	PostalCode   string  `json:"postal_code"`
	Country      string  `json:"country"`
	IsDefault    bool    `json:"is_default"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

func ToAddressResponse(a *model.Address) *AddressResponse {
	if a == nil {
		return nil
	}
	return &AddressResponse{
		AddressID:    a.ID,
		ID:           a.ID,
		UserID:       a.UserID,
		Label:        a.Label,
		FullName:     a.FullName,
		PhoneNumber:  a.PhoneNumber,
		EmailAddress: a.EmailAddress,
		AddressLine:  a.AddressLine,
		City:         a.City,
		State:        a.State,
		PostalCode:   a.PostalCode,
		Country:      a.Country,
		IsDefault:    a.IsDefault,
		CreatedAt:    a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    a.UpdatedAt.Format(time.RFC3339),
	}
}

func ToAddressListResponse(addresses []*model.Address) []*AddressResponse {
	if addresses == nil {
		return []*AddressResponse{}
	}
	res := make([]*AddressResponse, len(addresses))
	for i, a := range addresses {
		res[i] = ToAddressResponse(a)
	}
	return res
}
