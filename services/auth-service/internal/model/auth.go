package model

import (
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
)

type Role string

const (
	RoleUser            Role = "USER"
	RoleCustomer        Role = "CUSTOMER"
	RoleMerchant        Role = "MERCHANT"
	RoleDeliveryPartner Role = "DELIVERY_PARTNER"
	RoleAdmin           Role = "ADMIN"
)

func (r Role) String() string {
	return string(r)
}

func (r Role) IsValid() bool {
	switch Role(strings.ToUpper(string(r))) {
	case RoleUser, RoleMerchant, RoleDeliveryPartner, RoleAdmin:
		return true
	default:
		return false
	}
}

func NormalizeRole(r string) Role {
	switch strings.ToUpper(strings.TrimSpace(r)) {
	case "MERCHANT":
		return RoleMerchant
	case "DELIVERY_PARTNER", "DELIVERYPARTNER", "DELIVERY":
		return RoleDeliveryPartner
	case "ADMIN":
		return RoleAdmin
	case "USER", "CUSTOMER":
		return RoleUser
	default:
		return RoleUser
	}
}

type AuthCredential struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	UserID           uuid.UUID  `json:"user_id" db:"user_id"`
	Email            string     `json:"email" db:"email"`
	Phone            *string    `json:"phone,omitempty" db:"phone"`
	PasswordHash     string     `json:"-" db:"password_hash"`
	Role             Role       `json:"role" db:"role"`
	EmailVerified    bool       `json:"email_verified" db:"email_verified"`
	IsActive         bool       `json:"is_active" db:"is_active"`
	FailedLoginCount int        `json:"failed_login_count" db:"failed_login_count"`
	LockedUntil      *time.Time `json:"locked_until,omitempty" db:"locked_until"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}

type RefreshToken struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	UserID    uuid.UUID  `json:"user_id" db:"user_id"`
	TokenHash string     `json:"-" db:"token_hash"`
	ExpiresAt time.Time  `json:"expires_at" db:"expires_at"`
	Revoked   bool       `json:"revoked" db:"revoked"`
	RevokedAt *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

type PasswordResetToken struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	UserID    uuid.UUID  `json:"user_id" db:"user_id"`
	TokenHash string     `json:"-" db:"token_hash"`
	ExpiresAt time.Time  `json:"expires_at" db:"expires_at"`
	Used      bool       `json:"used" db:"used"`
	UsedAt    *time.Time `json:"used_at,omitempty" db:"used_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

