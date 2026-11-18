package dto

import "time"

type TrackActivityInput struct {
	UserID     string
	SessionID  string
	EventType  string
	ProductID  string
	CategoryID string
}

type RecentlyViewedProduct struct {
	ProductID    string    `json:"product_id"`
	LastViewedAt time.Time `json:"last_viewed_at"`
}

type CustomerPreference struct {
	CategoryID string  `json:"category_id"`
	Score      float64 `json:"score"`
}
