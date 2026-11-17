package model

import "time"

type CustomerActivity struct {
	ID         string
	UserID     *string
	SessionID  *string
	EventType  string
	ProductID  *string
	CategoryID *string
	CreatedAt  time.Time
}
