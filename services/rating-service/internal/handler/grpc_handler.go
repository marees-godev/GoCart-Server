package handler

import (
	"context"
	"log/slog"
	"time"

	ratingpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/rating"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RatingGRPCHandler struct {
	ratingpb.UnimplementedRatingServiceServer
	service service.RatingService
	log     *slog.Logger
}

func NewRatingGRPCHandler(svc service.RatingService, log *slog.Logger) *RatingGRPCHandler {
	if log == nil {
		log = slog.Default()
	}
	return &RatingGRPCHandler{
		service: svc,
		log:     log,
	}
}

func (h *RatingGRPCHandler) CreateRating(ctx context.Context, req *ratingpb.CreateRatingRequest) (*ratingpb.CreateRatingResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	created, err := h.service.CreateRating(ctx, dto.CreateRatingInput{
		UserID:      req.GetUserId(),
		ProductID:   req.GetProductId(),
		OrderID:     req.GetOrderId(),
		OrderItemID: req.GetOrderItemId(),
		Rating:      int(req.GetRating()),
		Review:      req.GetReview(),
		Images:      req.Images,
	})
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &ratingpb.CreateRatingResponse{
		Rating: mapModelToProto(created),
	}, nil
}

func (h *RatingGRPCHandler) GetRating(ctx context.Context, req *ratingpb.GetRatingRequest) (*ratingpb.GetRatingResponse, error) {

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	result, err := h.service.GetRating(ctx, req.GetId())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &ratingpb.GetRatingResponse{
		Rating: mapModelToProto(result),
	}, nil
}

func (h *RatingGRPCHandler) GetProductRatings(ctx context.Context, req *ratingpb.GetProductRatingsRequest) (*ratingpb.GetProductRatingsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	ratings, avg, total, stars, err := h.service.GetProductRatings(ctx, req.GetProductId(), req.GetLimit(), req.GetOffset())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	protoRatings := make([]*ratingpb.Rating, 0, len(ratings))
	for _, r := range ratings {
		protoRatings = append(protoRatings, mapModelToProto(r))
	}

	starRating := &ratingpb.StarRating{
		FiveStar:  int32(stars.FiveStar),
		FourStar:  int32(stars.FourStar),
		ThreeStar: int32(stars.ThreeStar),
		TwoStar:   int32(stars.TwoStar),
		OneStar:   int32(stars.OneStar),
	}

	return &ratingpb.GetProductRatingsResponse{
		Ratings:       protoRatings,
		AverageRating: avg,
		Total:         total,
		Stars:         starRating,
	}, nil
}

func (h *RatingGRPCHandler) UpdateRating(ctx context.Context, req *ratingpb.UpdateRatingRequest) (*ratingpb.UpdateRatingResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	update, err := h.service.UpdateRating(ctx, dto.UpdateRatingInput{
		ID:     req.GetId(),
		UserID: req.GetUserId(),
		Rating: int(req.GetRating()),
		Review: req.GetReview(),
	})

	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &ratingpb.UpdateRatingResponse{
		Rating: mapModelToProto(update),
	}, nil
}

func (h *RatingGRPCHandler) DeleteRating(ctx context.Context, req *ratingpb.DeleteRatingRequest) (*ratingpb.DeleteRatingResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	err := h.service.DeleteRating(ctx, dto.DeleteRatingInput{
		ID:     req.GetId(),
		UserID: req.GetUserId(),
	})

	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &ratingpb.DeleteRatingResponse{
		Success: true,
		Message: "Rating Deleted Successfully",
	}, nil
}

func (h *RatingGRPCHandler) FlagRating(ctx context.Context, req *ratingpb.FlagRatingRequest) (*ratingpb.FlagRatingResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	category := req.GetCategory().String()

	if category == "UNSPECIFIED" || category == "" {
		return nil, status.Error(codes.InvalidArgument, "a valid flag category is required")
	}

	err := h.service.FlagRating(ctx, dto.FlagRatingInput{
		RatingID: req.GetRatingId(),
		UserID:   req.GetUserId(),
		Category: category,
		Reason:   req.GetReason(),
	})

	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &ratingpb.FlagRatingResponse{
		Success: true,
		Message: "Rating has been flagged successfully",
	}, nil

}

func (h *RatingGRPCHandler) GetFlaggedRatings(ctx context.Context, req *ratingpb.GetFlaggedRatingsRequest) (*ratingpb.GetFlaggedRatingsResponse, error) {

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request must not be nil")
	}

	items, total, err := h.service.GetFlaggedRatings(ctx, dto.GetFlaggedRatingsInput{
		AdminId: req.GetAdminId(),
		Limit:   req.GetLimit(),
		Offset:  req.GetOffset(),
	})

	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	protoItems := make([]*ratingpb.FlaggedRatingItem, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}

		var protoFlags []*ratingpb.RatingFlag
		for _, f := range it.Flags {
			if f == nil {
				continue
			}
			protoFlags = append(protoFlags, &ratingpb.RatingFlag{
				Id:        f.ID,
				UserId:    f.UserID,
				Category:  ratingpb.FlagCategory(ratingpb.FlagCategory_value[f.Category]),
				Reason:    f.Reason,
				CreatedAt: f.CreatedAt.Format(time.RFC3339),
			})
		}

		protoItems = append(protoItems, &ratingpb.FlaggedRatingItem{
			Rating:    mapModelToProto(it.Rating),
			FlagCount: it.FlagCount,
			Flags:     protoFlags,
		})
	}

	return &ratingpb.GetFlaggedRatingsResponse{
		Items: protoItems,
		Total: total,
	}, nil

}

func (h *RatingGRPCHandler) ModerateRatings(ctx context.Context, req *ratingpb.ModerateRatingRequest) (*ratingpb.ModerateRatingResponse, error) {

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request must not be nil")
	}

	err := h.service.ModerateRatings(ctx, dto.ModerateRatingInput{
		RatingId: req.RatingId,
		AdminId:  req.AdminId,
		IsHidden: req.IsHidden,
	})

	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	msg := "Rating hidden successfully"
	if !req.IsHidden {
		msg = "Rating unhidden successfully"
	}
	return &ratingpb.ModerateRatingResponse{
		Success: true,
		Message: msg,
	}, nil
}

func (h *RatingGRPCHandler) UploadRatingImages(ctx context.Context, req *ratingpb.UploadRatingImagesRequest) (*ratingpb.UploadRatingImagesResponse, error) {

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request must not be nil")
	}

	images, err := h.service.UploadRatingImages(ctx, dto.UploadRatingImagesInput{
		RatingID: req.RatingId,
		UserID:   req.UserId,
		Images:   req.Images,
	})
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	pbImages := make([]*ratingpb.RatingImage, 0, len(images))
	for _, img := range images {
		pbImages = append(pbImages, &ratingpb.RatingImage{
			Id:        img.ID,
			RatingId:  img.RatingID,
			Url:       img.URL,
			CreatedAt: img.CreatedAt.Format(time.RFC3339),
		})
	}

	return &ratingpb.UploadRatingImagesResponse{
		Images: pbImages,
	}, nil
}

func (h *RatingGRPCHandler) DeleteRatingImage(ctx context.Context, req *ratingpb.DeleteRatingImageRequest) (*ratingpb.DeleteRatingImageResponse, error) {

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request must not be nil")
	}

	err := h.service.DeleteRatingImage(ctx, dto.DeleteRatingImageInput{
		ImageID: req.ImageId,
		UserID:  req.UserId,
	})
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &ratingpb.DeleteRatingImageResponse{
		Success: true,
		Message: "rating image deleted successfully",
	}, nil
}

func mapModelToProto(r *model.Rating) *ratingpb.Rating {
	if r == nil {
		return nil
	}

	pbImages := make([]*ratingpb.RatingImage, 0, len(r.Images))
	for _, img := range r.Images {
		pbImages = append(pbImages, &ratingpb.RatingImage{
			Id:        img.ID,
			RatingId:  img.RatingID,
			Url:       img.URL,
			CreatedAt: img.CreatedAt.Format(time.RFC3339),
		})
	}

	return &ratingpb.Rating{
		Id:          r.ID,
		UserId:      r.UserID,
		ProductId:   r.ProductID,
		OrderId:     r.OrderID,
		OrderItemId: r.OrderItemID,
		Rating:      int32(r.Rating),
		Review:      r.Review,
		IsFlagged:   r.IsFlagged,
		IsHidden:    r.IsHidden,
		CreatedAt:   r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   r.UpdatedAt.Format(time.RFC3339),
		Images:      pbImages,
	}
}
