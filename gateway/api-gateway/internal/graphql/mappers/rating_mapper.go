package maps

import (
	ratingpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/rating"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
)

func MapRating(r *ratingpb.Rating) *model.Rating {
	if r == nil {
		return nil
	}

	var createdAt *string
	if r.CreatedAt != "" {
		createdAt = &r.CreatedAt
	}

	var updatedAt *string
	if r.UpdatedAt != "" {
		updatedAt = &r.UpdatedAt
	}

	return &model.Rating{
		ID:          r.Id,
		ProductID:   r.ProductId,
		UserID:      r.UserId,
		OrderID:     r.OrderId,
		OrderItemID: r.OrderItemId,
		Rating:      int(r.Rating),
		Review:      r.Review,
		IsFlagged:   r.IsFlagged,
		IsHidden:    r.IsHidden,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

func MapRatings(ratings []*ratingpb.Rating) []*model.Rating {
	if ratings == nil {
		return []*model.Rating{}
	}
	res := make([]*model.Rating, 0, len(ratings))
	for _, r := range ratings {
		if r != nil {
			res = append(res, MapRating(r))
		}
	}
	return res
}

func MapStarRating(star *ratingpb.StarRating) *model.StarRating {
	if star == nil {
		return &model.StarRating{}
	}

	return &model.StarRating{
		FiveStar:  int(star.FiveStar),
		FourStar:  int(star.FourStar),
		ThreeStar: int(star.ThreeStar),
		TwoStar:   int(star.TwoStar),
		OneStar:   int(star.OneStar),
	}
}
