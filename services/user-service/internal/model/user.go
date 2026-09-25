package model

import "time"

type Gender string

const (
	GenderMale   Gender = "male"
	GenderFemale Gender = "female"
	GenderOthers Gender = "others"
)

func (g Gender) IsValid() bool {
	switch g {
	case GenderMale, GenderFemale, GenderOthers:
		return true
	default:
		return false
	}
}

func (g Gender) String() string {
	return string(g)
}

type User struct {
	ID             string     `json:"id" db:"id"`
	Username       *string    `json:"username,omitempty" db:"username"`
	Email          string     `json:"email" db:"email"`
	FirstName      string     `json:"first_name" db:"first_name"`
	LastName       string     `json:"last_name" db:"last_name"`
	PhoneNumber    *string    `json:"phonenumber,omitempty" db:"phone"`
	AlternatePhone *string    `json:"alternate_phone,omitempty" db:"alternate_phone"`
	DateOfBirth    *time.Time `json:"date_of_birth,omitempty" db:"date_of_birth"`
	Gender         *Gender    `json:"gender,omitempty" db:"gender"`
	Bio            *string    `json:"bio,omitempty" db:"bio"`
	AvatarURL      *string    `json:"avatar_url,omitempty" db:"avatar_url"`
	Status         string     `json:"status" db:"status"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
	DeactivatedAt  *time.Time `json:"deactivated_at,omitempty" db:"deactivated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}
