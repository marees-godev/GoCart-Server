package model

import "time"

type UserAuditLog struct {
	ID          string    `json:"id" db:"id"`
	UserID      string    `json:"user_id" db:"user_id"`
	Action      string    `json:"action" db:"action"`
	PerformedBy string    `json:"performed_by" db:"performed_by"`
	Reason      *string   `json:"reason,omitempty" db:"reason"`
	Metadata    []byte    `json:"metadata,omitempty" db:"metadata"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}
