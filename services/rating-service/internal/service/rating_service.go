package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/repository"
)

type RatingService interface {
	CreateRating(ctx context.Context, input dto.CreateRatingInput) (*model.Rating, error)
	GetRating(ctx context.Context, id string) (*model.Rating, error)
	GetProductRatings(ctx context.Context, productID string, limit, offset int32) ([]*model.Rating, float64, int32, dto.StarRating, error)
	UpdateRating(ctx context.Context, input dto.UpdateRatingInput) (*model.Rating, error)
	DeleteRating(ctx context.Context, input dto.DeleteRatingInput) error
}

type ratingService struct {
	repo        repository.RatingRepository
	orderClient orderpb.OrderServiceClient
	log         *slog.Logger
}

func NewRatingService(repo repository.RatingRepository, orderClient orderpb.OrderServiceClient, log *slog.Logger) RatingService {
	if log == nil {
		log = slog.Default()
	}
	return &ratingService{
		repo:        repo,
		orderClient: orderClient,
		log:         log,
	}
}

func (s *ratingService) CreateRating(ctx context.Context, input dto.CreateRatingInput) (*model.Rating, error) {
	if input.Rating < 1 || input.Rating > 5 {
		return nil, appErrors.BadRequest("rating must be between 1 and 5")
	}

	if input.UserID == "" {
		return nil, appErrors.Unauthorized("unauthenticated: user_id is required")
	}
	if _, err := uuid.Parse(input.UserID); err != nil {
		return nil, appErrors.BadRequest("invalid user_id format")
	}
	if input.OrderID == "" {
		return nil, appErrors.BadRequest("order_id is required")
	}
	if _, err := uuid.Parse(input.OrderID); err != nil {
		return nil, appErrors.BadRequest("invalid order_id format")
	}
	if input.OrderItemID == "" {
		return nil, appErrors.BadRequest("order_item_id is required")
	}
	if _, err := uuid.Parse(input.OrderItemID); err != nil {
		return nil, appErrors.BadRequest("invalid order_item_id format")
	}

	if s.orderClient == nil {
		return nil, appErrors.Internal(nil, "order service client is not configured")
	}

	resp, err := s.orderClient.GetOrder(ctx, &orderpb.GetOrderRequest{
		Id:     input.OrderID,
		UserId: input.UserID,
	})
	if err != nil {
		s.log.Error("Failed to fetch order for rating verification", "order_id", input.OrderID, "user_id", input.UserID, "error", err)
		return nil, appErrors.NotFound("order not found or does not belong to the user")
	}

	order := resp.GetOrder()
	if order == nil {
		return nil, appErrors.NotFound("order not found")
	}

	orderStatus := strings.ToUpper(strings.TrimSpace(order.GetOrderStatus()))
	if orderStatus == "" {
		orderStatus = strings.ToUpper(strings.TrimSpace(order.GetStatus()))
	}
	if orderStatus != "DELIVERED" {
		return nil, appErrors.BadRequest("order must be DELIVERED before submitting a rating")
	}

	var matchedProductID string
	for _, item := range order.GetItems() {
		if item.GetId() == input.OrderItemID {
			matchedProductID = item.GetProductId()
			break
		}
	}

	if matchedProductID == "" {
		return nil, appErrors.BadRequest("order item not found in the referenced order")
	}

	if input.ProductID != "" && input.ProductID != matchedProductID {
		return nil, appErrors.BadRequest("product_id does not match the purchased order item")
	}

	ratingModel := &model.Rating{
		UserID:             input.UserID,
		ProductID:          matchedProductID,
		OrderID:            input.OrderID,
		OrderItemID:        input.OrderItemID,
		Rating:             input.Rating,
		Review:             strings.TrimSpace(input.Review),
		IsVerifiedPurchase: true,
		IsFlagged:          false,
		IsHidden:           false,
	}

	created, err := s.repo.CreateRating(ctx, ratingModel)
	if err != nil {
		if errors.Is(err, repository.ErrRatingAlreadyExists) {
			return nil, appErrors.Conflict("user has already submitted a rating for this order item")
		}
		s.log.Error("Failed to persist rating", "error", err)
		return nil, appErrors.Internal(err, "failed to save rating")
	}

	return created, nil
}

func (s *ratingService) GetRating(ctx context.Context, id string) (*model.Rating, error) {

	if id == "" {
		return nil, appErrors.BadRequest("rating id is required")
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, appErrors.BadRequest("invalid rating id format")
	}

	result, err := s.repo.GetRatingByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrRatingNotFound) {
			return nil, appErrors.NotFound("rating not found")
		}
		s.log.Error("Failed to fetch rating", "id", id, "error", err)

		return nil, appErrors.Internal(err, "Failed to fetch rating")
	}

	return result, nil
}

func (s *ratingService) GetProductRatings(ctx context.Context, productID string, limit, offset int32) ([]*model.Rating, float64, int32, dto.StarRating, error) {
	if strings.TrimSpace(productID) == "" {
		return nil, 0, 0, dto.StarRating{}, appErrors.BadRequest("product id is required")
	}
	if _, err := uuid.Parse(productID); err != nil {
		return nil, 0, 0, dto.StarRating{}, appErrors.BadRequest("invalid product id format")
	}

	if limit <= 0 {
		limit = 10
	} else if limit > 100 {
		limit = 100
	}

	if offset < 0 {
		offset = 0
	}

	ratings, avg, total, stars, err := s.repo.GetProductRatings(ctx, productID, limit, offset)
	if err != nil {
		s.log.Error("Failed to fetch product ratings", "product_id", productID, "error", err)
		return nil, 0, 0, dto.StarRating{}, appErrors.Internal(err, "failed to fetch product ratings")
	}

	return ratings, avg, total, stars, nil
}

func (s *ratingService) UpdateRating(ctx context.Context, input dto.UpdateRatingInput) (*model.Rating, error) {

	if input.ID == "" {
		return nil, appErrors.BadRequest("rating id is required")
	}

	if _, err := uuid.Parse(input.ID); err != nil {
		return nil, appErrors.BadRequest("invalid rating id format")
	}

	if input.UserID == "" {
		return nil, appErrors.Unauthorized("unauthenticated: user_id is required")
	}
	if _, err := uuid.Parse(input.UserID); err != nil {
		return nil, appErrors.BadRequest("invalid user_id format")
	}

	if input.Rating < 1 || input.Rating > 5 {
		return nil, appErrors.BadRequest("rating must be between 1 and 5")
	}

	existing, err := s.repo.GetRatingByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, repository.ErrRatingNotFound) {
			return nil, appErrors.NotFound("rating not found")
		}
		return nil, appErrors.Internal(err, "failed to fetch rating")
	}

	if existing.UserID != input.UserID {
		return nil, appErrors.Forbidden("you are not authorized to update this rating")
	}

	rating, err := s.repo.UpdateRating(ctx, input.ID, input.Rating, input.Review)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to update rating")
	}
	return rating, nil
}

func (s *ratingService) DeleteRating(ctx context.Context, input dto.DeleteRatingInput) error {

	if input.ID == "" {
		return appErrors.BadRequest("rating id is required")
	}

	if _, err := uuid.Parse(input.ID); err != nil {
		return appErrors.BadRequest("invalid rating id format")
	}

	if input.UserID == "" {
		return appErrors.Unauthorized("unauthenticated: user_id is required")
	}
	if _, err := uuid.Parse(input.UserID); err != nil {
		return appErrors.BadRequest("invalid user_id format")
	}

	existing, err := s.repo.GetRatingByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, repository.ErrRatingNotFound) {
			return appErrors.NotFound("rating not found")
		}
		return appErrors.Internal(err, "failed to fetch rating")
	}

	if existing.UserID != input.UserID {
		return appErrors.Forbidden("you are not authorized to delete this rating")
	}

	if err := s.repo.DeleteRating(ctx, input.ID); err != nil {
		if errors.Is(err, repository.ErrRatingNotFound) {
			return appErrors.NotFound("rating not found")
		}
		return appErrors.Internal(err, "failed to delete rating")
	}

	return nil

}
