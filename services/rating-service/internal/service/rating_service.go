package service

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/storage"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/repository"
)

const (
	maxImagesPerReview = 5
	maxImageSizeBytes  = 5 * 1024 * 1024
)

type RatingService interface {
	CreateRating(ctx context.Context, input dto.CreateRatingInput) (*model.Rating, error)
	GetRating(ctx context.Context, id string) (*model.Rating, error)
	GetProductRatings(ctx context.Context, productID string, limit, offset int32) ([]*model.Rating, float64, int32, dto.StarRating, error)
	UpdateRating(ctx context.Context, input dto.UpdateRatingInput) (*model.Rating, error)
	DeleteRating(ctx context.Context, input dto.DeleteRatingInput) error
	FlagRating(ctx context.Context, input dto.FlagRatingInput) error
	GetFlaggedRatings(ctx context.Context, input dto.GetFlaggedRatingsInput) ([]*dto.FlaggedRatingItem, int32, error)
	ModerateRatings(ctx context.Context, input dto.ModerateRatingInput) error
	UploadRatingImages(ctx context.Context, input dto.UploadRatingImagesInput) ([]*model.RatingImage, error)
	DeleteRatingImage(ctx context.Context, input dto.DeleteRatingImageInput) error
}

type ratingService struct {
	repo        repository.RatingRepository
	orderClient orderpb.OrderServiceClient
	uploader    storage.Uploader
	log         *slog.Logger
}

func NewRatingService(repo repository.RatingRepository, orderClient orderpb.OrderServiceClient, uploader storage.Uploader, log *slog.Logger) RatingService {
	if log == nil {
		log = slog.Default()
	}
	return &ratingService{
		repo:        repo,
		orderClient: orderClient,
		uploader:    uploader,
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

	if len(input.Images) > maxImagesPerReview {
		return nil, appErrors.BadRequest("maximum 5 images allowed per review")
	}

	var ratingImages []model.RatingImage
	for _, raw := range input.Images {
		u, err := s.processImageDataURL(ctx, raw)
		if err != nil {
			return nil, err
		}
		ratingImages = append(ratingImages, model.RatingImage{URL: u})
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
		Images:             ratingImages,
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

	if existing.IsHidden {
		return nil, appErrors.Forbidden("cannot edit a review that has been hidden by moderation")
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

func (s *ratingService) FlagRating(ctx context.Context, input dto.FlagRatingInput) error {
	if input.RatingID == "" {
		return appErrors.BadRequest("rating id is required")
	}

	if _, err := uuid.Parse(input.RatingID); err != nil {
		return appErrors.BadRequest("invalid rating id format")
	}

	if input.UserID == "" {
		return appErrors.Unauthorized("unauthenticated: user_id is required")
	}

	if _, err := uuid.Parse(input.UserID); err != nil {
		return appErrors.BadRequest("invalid user_id format")
	}

	category := strings.ToUpper(strings.TrimSpace(input.Category))
	if category == "" || category == "UNSPECIFIED" {
		return appErrors.BadRequest("category is required")
	}

	switch category {
	case "SPAM", "INAPPROPRIATE_CONTENT", "FAKE_REVIEW", "IRRELEVANT", "OTHER":
	default:
		return appErrors.BadRequest("invalid category")
	}

	reason := strings.TrimSpace(input.Reason)

	if category == "OTHER" && reason == "" {
		return appErrors.BadRequest("reason is required")
	}

	existing, err := s.repo.GetRatingByID(ctx, input.RatingID)
	if err != nil {
		if errors.Is(err, repository.ErrRatingNotFound) {
			return appErrors.NotFound("rating not found")
		}
		return appErrors.Internal(err, "failed to fetch rating")
	}

	if existing.UserID == input.UserID {
		return appErrors.BadRequest("you cannot flag your own review")
	}

	if strings.TrimSpace(existing.Review) == "" {
		return appErrors.BadRequest("cannot flag a rating without review text")
	}

	flag := &model.RatingFlag{
		RatingID: input.RatingID,
		UserID:   input.UserID,
		Category: category,
		Reason:   reason,
	}

	err = s.repo.CreateFlag(ctx, flag)
	if err != nil {
		if errors.Is(err, repository.ErrRatingAlreadyExists) {
			return appErrors.Conflict("user has already submitted a flag for this review")
		}
		if errors.Is(err, repository.ErrRatingNotFound) {
			return appErrors.NotFound("rating not found")
		}
		s.log.Error("Failed to persist flag for rating", "error", err)
		return appErrors.Internal(err, "failed to save flag rating")
	}

	s.log.Info("Rating flagged by user", "action", "RATING_FLAGGED",
		"rating_id", input.RatingID,
		"user_id", input.UserID,
		"category", category,
		"reason", reason)

	return nil

}

func (s *ratingService) GetFlaggedRatings(ctx context.Context, input dto.GetFlaggedRatingsInput) ([]*dto.FlaggedRatingItem, int32, error) {

	if input.AdminId == "" {
		return nil, 0, appErrors.Unauthorized("unauthenticated: admin_id is required")
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	} else if limit > 100 {
		limit = 100
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}

	items, total, err := s.repo.GetFlaggedRatings(ctx, limit, offset)
	if err != nil {
		s.log.Error("Failed to fetch flagged ratings", "error", err)
		return nil, 0, appErrors.Internal(err, "failed to fetch flagged ratings")
	}
	return items, total, nil
}

func (s *ratingService) ModerateRatings(ctx context.Context, input dto.ModerateRatingInput) error {

	if input.RatingId == "" {
		return appErrors.BadRequest("rating id is required")
	}

	if _, err := uuid.Parse(input.RatingId); err != nil {
		return appErrors.BadRequest("invalid rating id format")
	}

	if input.AdminId == "" {
		return appErrors.Unauthorized("unauthenticated: admin_id is required")
	}

	err := s.repo.ModerateRatings(ctx, input.IsHidden, input.RatingId)
	if err != nil {
		if errors.Is(err, repository.ErrRatingNotFound) {
			return appErrors.NotFound("rating not found")
		}
		s.log.Error("Failed to moderate rating", "error", err, "rating_id", input.RatingId)
		return appErrors.Internal(err, "failed to moderate rating")
	}

	s.log.Info("Rating moderated by admin",
		"rating_id", input.RatingId,
		"is_hidden", input.IsHidden,
	)

	return nil
}

func (s *ratingService) processImageDataURL(ctx context.Context, input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", appErrors.BadRequest("image cannot be empty")
	}

	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed, nil
	}
	if !strings.HasPrefix(trimmed, "data:") {
		return "", appErrors.BadRequest("invalid image format: must be a valid URL or base64 data URI")
	}
	if s.uploader == nil {
		return "", appErrors.Internal(nil, "storage uploader not configured")
	}
	parts := strings.SplitN(trimmed, ";base64,", 2)
	if len(parts) != 2 {
		return "", appErrors.BadRequest("invalid base64 image format")
	}

	header := parts[0]
	var contentType, ext string
	switch {
	case strings.Contains(header, "image/jpeg"), strings.Contains(header, "image/jpg"):
		contentType, ext = "image/jpeg", ".jpg"
	case strings.Contains(header, "image/png"):
		contentType, ext = "image/png", ".png"
	case strings.Contains(header, "image/webp"):
		contentType, ext = "image/webp", ".webp"
	default:
		return "", appErrors.BadRequest("unsupported image format: allowed jpeg, png, webp")
	}
	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", appErrors.BadRequest("failed to decode base64 image")
	}
	if len(decoded) > maxImageSizeBytes {
		return "", appErrors.BadRequest("image exceeds maximum allowed size of 5MB")
	}
	key := s.uploader.GenerateKey("ratings", "review"+ext)
	uploadedURL, err := s.uploader.UploadBytes(ctx, key, decoded, contentType)
	if err != nil {
		return "", appErrors.Internal(err, "failed to upload image to storage")
	}
	return uploadedURL, nil
}

func (s *ratingService) UploadRatingImages(ctx context.Context, input dto.UploadRatingImagesInput) ([]*model.RatingImage, error) {
	if input.RatingID == "" {
		return nil, appErrors.BadRequest("rating_id is required")
	}

	if _, err := uuid.Parse(input.RatingID); err != nil {
		return nil, appErrors.BadRequest("invalid rating_id format")
	}

	if input.UserID == "" {
		return nil, appErrors.Unauthorized("unauthenticated: user_id is required")
	}

	if _, err := uuid.Parse(input.UserID); err != nil {
		return nil, appErrors.BadRequest("invalid user_id format")
	}

	if len(input.Images) == 0 {
		return nil, appErrors.BadRequest("at least one image is required")
	}

	existing, err := s.repo.GetRatingByID(ctx, input.RatingID)
	if err != nil {
		if errors.Is(err, repository.ErrRatingNotFound) {
			return nil, appErrors.NotFound("rating not found")
		}
		return nil, appErrors.Internal(err, "failed to fetch rating")
	}

	if existing.UserID != input.UserID {
		return nil, appErrors.Forbidden("you are not authorized to upload images to this rating")
	}
	if len(existing.Images)+len(input.Images) > maxImagesPerReview {
		return nil, appErrors.BadRequest("maximum 5 images allowed per review")
	}

	uploadedURLs := make([]string, 0, len(input.Images))
	for _, raw := range input.Images {
		u, err := s.processImageDataURL(ctx, raw)
		if err != nil {
			return nil, err
		}
		uploadedURLs = append(uploadedURLs, u)
	}

	saved, err := s.repo.AddRatingImages(ctx, input.RatingID, uploadedURLs)
	if err != nil {
		s.log.Error("Failed to persist rating images", "error", err)
		return nil, appErrors.Internal(err, "failed to save rating images")
	}
	return saved, nil
}

func (s *ratingService) DeleteRatingImage(ctx context.Context, input dto.DeleteRatingImageInput) error {

	if input.ImageID == "" {
		return appErrors.BadRequest("image_id is required")
	}

	if _, err := uuid.Parse(input.ImageID); err != nil {
		return appErrors.BadRequest("invalid image_id format")
	}

	if input.UserID == "" {
		return appErrors.Unauthorized("unauthenticated: user_id is required")
	}

	if _, err := uuid.Parse(input.UserID); err != nil {
		return appErrors.BadRequest("invalid user_id format")
	}

	img, err := s.repo.GetRatingImageByID(ctx, input.ImageID)
	if err != nil {
		if errors.Is(err, repository.ErrRatingImageNotFound) {
			return appErrors.NotFound("rating image not found")
		}
		return appErrors.Internal(err, "failed to fetch rating image")
	}

	rating, err := s.repo.GetRatingByID(ctx, img.RatingID)
	if err != nil {
		if errors.Is(err, repository.ErrRatingNotFound) {
			return appErrors.NotFound("rating not found")
		}
		return appErrors.Internal(err, "failed to fetch parent rating")
	}

	if rating.UserID != input.UserID {
		return appErrors.Forbidden("you are not authorized to delete this rating image")
	}

	if err := s.repo.DeleteRatingImage(ctx, input.ImageID); err != nil {
		if errors.Is(err, repository.ErrRatingImageNotFound) {
			return appErrors.NotFound("rating image is not found")
		}
		return appErrors.Internal(err, "failed to delete rating image")
	}

	return nil
}
