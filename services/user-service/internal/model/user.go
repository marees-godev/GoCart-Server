package model

import "time"

type User struct {
	ID             string     `json:"id" db:"id"`
	Username       *string    `json:"username,omitempty" db:"username"`
	Email          string     `json:"email" db:"email"`
	FirstName      string     `json:"first_name" db:"first_name"`
	LastName       string     `json:"last_name" db:"last_name"`
	Phone          *string    `json:"phone,omitempty" db:"phone"`
	AlternatePhone *string    `json:"alternate_phone,omitempty" db:"alternate_phone"`
	DateOfBirth    *time.Time `json:"date_of_birth,omitempty" db:"date_of_birth"`
	Gender         *string    `json:"gender,omitempty" db:"gender"`
	Bio            *string    `json:"bio,omitempty" db:"bio"`
	AvatarURL      *string    `json:"avatar_url,omitempty" db:"avatar_url"`
	Role           string     `json:"role" db:"role"`
	Status         string     `json:"status" db:"status"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}
