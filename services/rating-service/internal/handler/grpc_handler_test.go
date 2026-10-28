package handler_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	ratingpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/rating"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/model"
)

type mockRatingService struct {
	ratings map[string]*model.Rating
}

func newMockRatingService() *mockRatingService {
	return &mockRatingService{
		ratings: make(map[string]*model.Rating),
	}
}

func (m *mockRatingService) CreateRating(ctx context.Context, input dto.CreateRatingInput) (*model.Rating, error) {
	if input.UserID == "" {
		return nil, appErrors.Unauthorized("unauthenticated: user_id is required")
	}
	if input.Rating < 1 || input.Rating > 5 {
		return nil, appErrors.BadRequest("rating must be between 1 and 5")
	}

	id := "11111111-1111-1111-1111-111111111111"
	now := time.Now().UTC()
	r := &model.Rating{
		ID:                 id,
		UserID:             input.UserID,
		ProductID:          input.ProductID,
		OrderID:            input.OrderID,
		OrderItemID:        input.OrderItemID,
		Rating:             input.Rating,
		Review:             input.Review,
		IsVerifiedPurchase: true,
		Status:             "ACTIVE",
		IsFlagged:          false,
		IsHidden:           false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	m.ratings[id] = r
	return r, nil
}

func (m *mockRatingService) GetRating(ctx context.Context, id string) (*model.Rating, error) {
	r, ok := m.ratings[id]
	if !ok {
		return nil, appErrors.NotFound("rating not found")
	}
	return r, nil
}

func (m *mockRatingService) GetProductRatings(ctx context.Context, productID string, limit, offset int32) ([]*model.Rating, float64, int32, error) {
	var list []*model.Rating
	var sum float64
	for _, r := range m.ratings {
		if r.ProductID == productID && !r.IsHidden {
			list = append(list, r)
			sum += float64(r.Rating)
		}
	}
	var avg float64
	if len(list) > 0 {
		avg = sum / float64(len(list))
	}
	return list, avg, int32(len(list)), nil
}

func (m *mockRatingService) UpdateRating(ctx context.Context, input dto.UpdateRatingInput) (*model.Rating, error) {
	r, ok := m.ratings[input.ID]
	if !ok {
		return nil, appErrors.NotFound("rating not found")
	}
	if r.UserID != input.UserID {
		return nil, appErrors.Forbidden("you are not authorized to update this rating")
	}
	r.Rating = input.Rating
	r.Review = input.Review
	r.UpdatedAt = time.Now().UTC()
	return r, nil
}

func (m *mockRatingService) DeleteRating(ctx context.Context, input dto.DeleteRatingInput) error {
	r, ok := m.ratings[input.ID]
	if !ok {
		return appErrors.NotFound("rating not found")
	}
	if r.UserID != input.UserID {
		return appErrors.Forbidden("you are not authorized to delete this rating")
	}
	delete(m.ratings, input.ID)
	return nil
}

func TestRatingGRPCHandler(t *testing.T) {
	svc := newMockRatingService()
	h := handler.NewRatingGRPCHandler(svc, nil)
	ctx := context.Background()

	ratingID := "11111111-1111-1111-1111-111111111111"
	userID := uuid.New().String()
	productID := uuid.New().String()
	orderID := uuid.New().String()
	orderItemID := uuid.New().String()

	t.Run("CreateRating RPC", func(t *testing.T) {
		req := &ratingpb.CreateRatingRequest{
			ProductId:   productID,
			OrderId:     orderID,
			OrderItemId: orderItemID,
			UserId:      userID,
			Rating:      5,
			Review:      "Excellent product quality!",
		}

		resp, err := h.CreateRating(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetRating() == nil || resp.GetRating().GetId() != ratingID {
			t.Errorf("expected rating ID %s, got %v", ratingID, resp.GetRating())
		}
		if resp.GetRating().GetRating() != 5 {
			t.Errorf("expected rating 5, got %d", resp.GetRating().GetRating())
		}
	})

	t.Run("GetRating RPC", func(t *testing.T) {
		resp, err := h.GetRating(ctx, &ratingpb.GetRatingRequest{Id: ratingID})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetRating().GetReview() != "Excellent product quality!" {
			t.Errorf("expected review match, got %s", resp.GetRating().GetReview())
		}
	})

	t.Run("GetProductRatings RPC", func(t *testing.T) {
		resp, err := h.GetProductRatings(ctx, &ratingpb.GetProductRatingsRequest{
			ProductId: productID,
			Limit:     10,
			Offset:    0,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetTotal() != 1 {
			t.Errorf("expected 1 rating, got %d", resp.GetTotal())
		}
		if resp.GetAverageRating() != 5.0 {
			t.Errorf("expected average 5.0, got %f", resp.GetAverageRating())
		}
	})

	t.Run("UpdateRating RPC", func(t *testing.T) {
		resp, err := h.UpdateRating(ctx, &ratingpb.UpdateRatingRequest{
			Id:     ratingID,
			UserId: userID,
			Rating: 4,
			Review: "Updated review to 4 stars",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetRating().GetRating() != 4 {
			t.Errorf("expected updated rating 4, got %d", resp.GetRating().GetRating())
		}
	})

	t.Run("DeleteRating RPC", func(t *testing.T) {
		resp, err := h.DeleteRating(ctx, &ratingpb.DeleteRatingRequest{
			Id:     ratingID,
			UserId: userID,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !resp.GetSuccess() {
			t.Errorf("expected success true")
		}
	})
}
