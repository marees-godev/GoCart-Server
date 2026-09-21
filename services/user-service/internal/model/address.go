package model

import "time"

type Address struct {
	ID           string    `json:"id" db:"id"`
	UserID       string    `json:"user_id" db:"user_id"`
	CountryID    *string   `json:"country_id,omitempty" db:"country_id"`
	Label        *string   `json:"label,omitempty" db:"label"`
	FullName     *string   `json:"full_name,omitempty" db:"full_name"`
	PhoneNumber  *string   `json:"phone_number,omitempty" db:"phone_number"`
	EmailAddress *string   `json:"email_address,omitempty" db:"email_address"`
	AddressLine  string    `json:"address_line" db:"address_line"`
	City         string    `json:"city" db:"city"`
	State        string    `json:"state" db:"state"`
	PostalCode   string    `json:"postal_code" db:"postal_code"`
	Country      string    `json:"country" db:"country"`
	IsDefault    bool      `json:"is_default" db:"is_default"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}
