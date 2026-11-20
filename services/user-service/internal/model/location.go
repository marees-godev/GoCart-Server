package model

import (
	"time"

	"github.com/google/uuid"
)

type Location struct {
	ID          uuid.UUID    `json:"id" db:"id"`
	Pincode     string       `json:"pincode" db:"pincode"`
	Country     string       `json:"country" db:"country"`
	State       string       `json:"state" db:"state"`
	District    string       `json:"district" db:"district"`
	PostOffices []PostOffice `json:"post_offices"`
	CreatedAt   time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at" db:"updated_at"`
}

type PostOffice struct {
	ID             uuid.UUID `json:"id" db:"id"`
	LocationID     uuid.UUID `json:"location_id" db:"location_id"`
	Name           string    `json:"name" db:"name"`
	BranchType     string    `json:"branch_type" db:"branch_type"`
	DeliveryStatus string    `json:"delivery_status" db:"delivery_status"`
	Circle         string    `json:"circle" db:"circle"`
	District       string    `json:"district" db:"district"`
	Division       string    `json:"division" db:"division"`
	Region         string    `json:"region" db:"region"`
	State          string    `json:"state" db:"state"`
	Country        string    `json:"country" db:"country"`
	Pincode        string    `json:"pincode" db:"pincode"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}
