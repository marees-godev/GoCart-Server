package dto

import "github.com/marees-godev/GoCart-Server/services/rating-service/internal/model"

type CreateRatingInput struct {
	UserID      string `json:"user_id"`
	ProductID   string `json:"product_id"`
	OrderID     string `json:"order_id"`
	OrderItemID string `json:"order_item_id"`
	Rating      int    `json:"rating"`
	Review      string `json:"review"`
}

type UpdateRatingInput struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Rating int    `json:"rating"`
	Review string `json:"review"`
}

type DeleteRatingInput struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
}

type StarRating struct {
	FiveStar  int `json:"five_star"`
	FourStar  int `json:"four_star"`
	ThreeStar int `json:"three_star"`
	TwoStar   int `json:"two_star"`
	OneStar   int `json:"one_star"`
}

type FlagRatingInput struct {
	UserID   string `json:"user_id"`
	RatingID string `json:"rating_id"`
	Category string `json:"category"`
	Reason   string `json:"reason"`
}

type FlaggedRatingItem struct {
	Rating    *model.Rating       `json:"rating"`
	FlagCount int32               `json:"flag_count"`
	Flags     []*model.RatingFlag `json:"flags"`
}

type GetFlaggedRatingsInput struct {
	AdminId string `json:"admin_id"`
	Limit   int32  `json:"limit"`
	Offset  int32  `json:"offset"`
}

type ModerateRatingInput struct {
	RatingId string `json:"rating_id"`
	AdminId  string `json:"admin_id"`
	IsHidden bool   `json:"is_hidden"`
}

type UploadRatingImagesInput struct {
	RatingID string   `json:"rating_id"`
	UserID   string   `json:"user_id"`
	Images   []string `json:"images"`
}

type DeleteRatingImageInput struct {
	ImageID string `json:"image_id"`
	UserID  string `json:"user_id"`
}
