package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/service"
	grpcPkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockRatingRepo struct {
	ratings   map[string]*model.Rating
	idCounter int
}

func newMockRatingRepo() *mockRatingRepo {
	return &mockRatingRepo{
		ratings: make(map[string]*model.Rating),
	}
}

func (m *mockRatingRepo) CreateRating(ctx context.Context, r *model.Rating) (*model.Rating, error) {
	for _, existing := range m.ratings {
		if existing.DeletedAt == nil && existing.UserID == r.UserID && existing.OrderItemID == r.OrderItemID {
			return nil, repository.ErrRatingAlreadyExists
		}
	}

	if r.ID == "" {
		m.idCounter++
		r.ID = fmt.Sprintf("11111111-1111-1111-1111-%012d", m.idCounter)
	}
	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now

	cp := *r
	m.ratings[r.ID] = &cp
	return &cp, nil
}

func (m *mockRatingRepo) GetRatingByID(ctx context.Context, id string) (*model.Rating, error) {
	r, ok := m.ratings[id]
	if !ok || r.DeletedAt != nil {
		return nil, repository.ErrRatingNotFound
	}
	cp := *r
	return &cp, nil
}

func (m *mockRatingRepo) GetProductRatings(ctx context.Context, productID string, limit, offset int32) ([]*model.Rating, float64, int32, error) {
	var matched []*model.Rating
	var sum float64
	for _, r := range m.ratings {
		if r.ProductID == productID && !r.IsHidden && r.DeletedAt == nil {
			cp := *r
			matched = append(matched, &cp)
			sum += float64(r.Rating)
		}
	}

	total := int32(len(matched))
	var avg float64
	if total > 0 {
		avg = sum / float64(total)
	}

	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if limit <= 0 || end > total {
		end = total
	}

	return matched[start:end], avg, total, nil
}

func (m *mockRatingRepo) UpdateRating(ctx context.Context, id string, rating int, review string) (*model.Rating, error) {
	r, ok := m.ratings[id]
	if !ok || r.DeletedAt != nil {
		return nil, repository.ErrRatingNotFound
	}

	r.Rating = rating
	r.Review = review
	r.UpdatedAt = time.Now().UTC()

	cp := *r
	m.ratings[id] = &cp
	return &cp, nil
}

func (m *mockRatingRepo) DeleteRating(ctx context.Context, id string) error {
	r, ok := m.ratings[id]
	if !ok || r.DeletedAt != nil {
		return repository.ErrRatingNotFound
	}

	now := time.Now().UTC()
	r.DeletedAt = &now
	return nil
}

type mockOrderClient struct {
	orderpb.OrderServiceClient
	orders map[string]*orderpb.Order
	err    error
}

func (m *mockOrderClient) GetOrder(ctx context.Context, in *orderpb.GetOrderRequest, opts ...grpcPkg.CallOption) (*orderpb.GetOrderResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	o, ok := m.orders[in.Id]
	if !ok {
		return nil, status.Error(codes.NotFound, "order not found")
	}
	return &orderpb.GetOrderResponse{Order: o}, nil
}

func TestRatingService_Operations(t *testing.T) {
	ctx := context.Background()
	repo := newMockRatingRepo()

	user1 := uuid.New().String()
	user2 := uuid.New().String()
	order1 := uuid.New().String()
	orderItem1 := uuid.New().String()
	prod1 := uuid.New().String()

	orderClient := &mockOrderClient{
		orders: map[string]*orderpb.Order{
			order1: {
				Id:          order1,
				UserId:      user1,
				OrderStatus: "DELIVERED",
				Items: []*orderpb.OrderItem{
					{
						Id:        orderItem1,
						ProductId: prod1,
					},
				},
			},
		},
	}

	svc := service.NewRatingService(repo, orderClient, nil)

	// 1. Create rating for a delivered order item (should succeed)
	rating, err := svc.CreateRating(ctx, dto.CreateRatingInput{
		UserID:      user1,
		OrderID:     order1,
		OrderItemID: orderItem1,
		ProductID:   prod1,
		Rating:      5,
		Review:      "Excellent product!",
	})
	if err != nil {
		t.Fatalf("failed to create rating: %v", err)
	}
	if rating.ID == "" {
		t.Error("expected non-empty rating ID")
	}
	if rating.Rating != 5 || rating.Review != "Excellent product!" {
		t.Errorf("unexpected rating or review: %+v", rating)
	}
	if !rating.IsVerifiedPurchase {
		t.Errorf("expected verified purchase : %+v", rating)
	}

	// 2. Get rating by ID (should succeed and match created data)
	fetched, err := svc.GetRating(ctx, rating.ID)
	if err != nil {
		t.Fatalf("failed to get rating: %v", err)
	}
	if fetched.ID != rating.ID || fetched.UserID != user1 || fetched.Rating != 5 {
		t.Errorf("unexpected rating fetched: %+v", fetched)
	}

	// 3. Reject duplicate rating for the same order item (should fail with conflict)
	_, err = svc.CreateRating(ctx, dto.CreateRatingInput{
		UserID:      user1,
		OrderID:     order1,
		OrderItemID: orderItem1,
		ProductID:   prod1,
		Rating:      4,
	})
	if err == nil {
		t.Fatal("expected duplicate rating to be rejected")
	}
	if !strings.Contains(err.Error(), "already submitted a rating") {
		t.Errorf("expected conflict error message, got: %v", err)
	}

	// 4. Update rating and review by original author (should succeed)
	updated, err := svc.UpdateRating(ctx, dto.UpdateRatingInput{
		ID:     rating.ID,
		UserID: user1,
		Rating: 4,
		Review: "Updated review after using for a month",
	})
	if err != nil {
		t.Fatalf("failed to update rating: %v", err)
	}
	if updated.Rating != 4 || updated.Review != "Updated review after using for a month" {
		t.Errorf("unexpected updated rating: %+v", updated)
	}

	// 5. Reject update by non-author (should fail with forbidden)
	_, err = svc.UpdateRating(ctx, dto.UpdateRatingInput{
		ID:     rating.ID,
		UserID: user2,
		Rating: 1,
	})
	if err == nil {
		t.Fatal("expected non-author update to fail")
	}
	if !strings.Contains(err.Error(), "not authorized") {
		t.Errorf("expected authorization error message, got: %v", err)
	}

	// 6. Reject delete by non-author (should fail with forbidden)
	err = svc.DeleteRating(ctx, dto.DeleteRatingInput{
		ID:     rating.ID,
		UserID: user2,
	})
	if err == nil {
		t.Fatal("expected non-author delete to fail")
	}
	if !strings.Contains(err.Error(), "not authorized") {
		t.Errorf("expected authorization error message, got: %v", err)
	}

	// 7. Soft delete rating by original author (should succeed)
	err = svc.DeleteRating(ctx, dto.DeleteRatingInput{
		ID:     rating.ID,
		UserID: user1,
	})
	if err != nil {
		t.Fatalf("failed to delete rating: %v", err)
	}

	// 8. GetRating for soft-deleted rating (should fail with not found)
	_, err = svc.GetRating(ctx, rating.ID)
	if err == nil {
		t.Fatal("expected deleted rating lookup to return not found")
	}

	// 9. Soft-deleted rating excluded from product ratings list
	ratings, avg, total, err := svc.GetProductRatings(ctx, prod1, 10, 0)
	if err != nil {
		t.Fatalf("failed to get product ratings: %v", err)
	}
	if total != 0 || len(ratings) != 0 || avg != 0 {
		t.Errorf("expected 0 ratings after soft delete, got total=%d len=%d avg=%f", total, len(ratings), avg)
	}
}

func TestRatingService_Validation(t *testing.T) {
	ctx := context.Background()
	repo := newMockRatingRepo()

	user1 := uuid.New().String()
	order1 := uuid.New().String()
	orderItem1 := uuid.New().String()
	prod1 := uuid.New().String()

	pendingOrder := uuid.New().String()

	orderClient := &mockOrderClient{
		orders: map[string]*orderpb.Order{
			order1: {
				Id:          order1,
				UserId:      user1,
				OrderStatus: "DELIVERED",
				Items: []*orderpb.OrderItem{
					{
						Id:        orderItem1,
						ProductId: prod1,
					},
				},
			},
			pendingOrder: {
				Id:          pendingOrder,
				UserId:      user1,
				OrderStatus: "PENDING",
				Items: []*orderpb.OrderItem{
					{
						Id:        orderItem1,
						ProductId: prod1,
					},
				},
			},
		},
	}

	svc := service.NewRatingService(repo, orderClient, nil)

	// Rating score validation (< 1 or > 5)
	_, err := svc.CreateRating(ctx, dto.CreateRatingInput{
		Rating: 0,
	})
	if err == nil || !strings.Contains(err.Error(), "between 1 and 5") {
		t.Errorf("expected rating range error for rating 0, got: %v", err)
	}

	_, err = svc.CreateRating(ctx, dto.CreateRatingInput{
		Rating: 6,
	})
	if err == nil || !strings.Contains(err.Error(), "between 1 and 5") {
		t.Errorf("expected rating range error for rating 6, got: %v", err)
	}

	// User ID validation
	_, err = svc.CreateRating(ctx, dto.CreateRatingInput{
		Rating: 5,
		UserID: "",
	})
	if err == nil || !strings.Contains(err.Error(), "user_id is required") {
		t.Errorf("expected user_id required error, got: %v", err)
	}

	_, err = svc.CreateRating(ctx, dto.CreateRatingInput{
		Rating: 5,
		UserID: "invalid-uuid",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid user_id format") {
		t.Errorf("expected invalid user_id error, got: %v", err)
	}

	// Order ID validation
	_, err = svc.CreateRating(ctx, dto.CreateRatingInput{
		Rating:  5,
		UserID:  user1,
		OrderID: "",
	})
	if err == nil || !strings.Contains(err.Error(), "order_id is required") {
		t.Errorf("expected order_id required error, got: %v", err)
	}

	// Order Item ID validation
	_, err = svc.CreateRating(ctx, dto.CreateRatingInput{
		Rating:      5,
		UserID:      user1,
		OrderID:     order1,
		OrderItemID: "",
	})
	if err == nil || !strings.Contains(err.Error(), "order_item_id is required") {
		t.Errorf("expected order_item_id required error, got: %v", err)
	}

	// Nil order client
	nilClientSvc := service.NewRatingService(repo, nil, nil)
	_, err = nilClientSvc.CreateRating(ctx, dto.CreateRatingInput{
		Rating:      5,
		UserID:      user1,
		OrderID:     order1,
		OrderItemID: orderItem1,
	})
	if err == nil || !strings.Contains(err.Error(), "order service client is not configured") {
		t.Errorf("expected client not configured error, got: %v", err)
	}

	// Order not found
	_, err = svc.CreateRating(ctx, dto.CreateRatingInput{
		Rating:      5,
		UserID:      user1,
		OrderID:     uuid.New().String(),
		OrderItemID: orderItem1,
	})
	if err == nil || !strings.Contains(err.Error(), "order not found") {
		t.Errorf("expected order not found error, got: %v", err)
	}

	// Order not delivered
	_, err = svc.CreateRating(ctx, dto.CreateRatingInput{
		Rating:      5,
		UserID:      user1,
		OrderID:     pendingOrder,
		OrderItemID: orderItem1,
	})
	if err == nil || !strings.Contains(err.Error(), "must be DELIVERED") {
		t.Errorf("expected order must be DELIVERED error, got: %v", err)
	}

	// Order item not found in order
	_, err = svc.CreateRating(ctx, dto.CreateRatingInput{
		Rating:      5,
		UserID:      user1,
		OrderID:     order1,
		OrderItemID: uuid.New().String(),
	})
	if err == nil || !strings.Contains(err.Error(), "order item not found") {
		t.Errorf("expected order item not found error, got: %v", err)
	}

	// Product ID mismatch with purchased item
	_, err = svc.CreateRating(ctx, dto.CreateRatingInput{
		Rating:      5,
		UserID:      user1,
		OrderID:     order1,
		OrderItemID: orderItem1,
		ProductID:   uuid.New().String(),
	})
	if err == nil || !strings.Contains(err.Error(), "product_id does not match") {
		t.Errorf("expected product_id mismatch error, got: %v", err)
	}

	// GetRating validation
	_, err = svc.GetRating(ctx, "")
	if err == nil || !strings.Contains(err.Error(), "rating id is required") {
		t.Errorf("expected rating id required error, got: %v", err)
	}

	_, err = svc.GetRating(ctx, "not-a-uuid")
	if err == nil || !strings.Contains(err.Error(), "invalid rating id format") {
		t.Errorf("expected invalid rating id error, got: %v", err)
	}

	// GetProductRatings validation
	_, _, _, err = svc.GetProductRatings(ctx, "", 10, 0)
	if err == nil || !strings.Contains(err.Error(), "product id is required") {
		t.Errorf("expected product id required error, got: %v", err)
	}

	_, _, _, err = svc.GetProductRatings(ctx, "invalid-uuid", 10, 0)
	if err == nil || !strings.Contains(err.Error(), "invalid product id format") {
		t.Errorf("expected invalid product id error, got: %v", err)
	}
}

func TestRatingService_ProductRatings_Aggregation(t *testing.T) {
	ctx := context.Background()
	repo := newMockRatingRepo()
	svc := service.NewRatingService(repo, &mockOrderClient{}, nil)

	targetProductID := uuid.New().String()
	otherProductID := uuid.New().String()

	// 1. Seed ratings for target product: ratings of 5, 4, 3 (average = 4.0)
	r1 := &model.Rating{
		ProductID: targetProductID,
		UserID:    uuid.New().String(),
		Rating:    5,
		Review:    "Top quality",
	}
	r2 := &model.Rating{
		ProductID: targetProductID,
		UserID:    uuid.New().String(),
		Rating:    4,
		Review:    "Pretty good",
	}
	r3 := &model.Rating{
		ProductID: targetProductID,
		UserID:    uuid.New().String(),
		Rating:    3,
		Review:    "Average",
	}
	// Seed 1 rating for different product
	rOther := &model.Rating{
		ProductID: otherProductID,
		UserID:    uuid.New().String(),
		Rating:    1,
	}

	if _, err := repo.CreateRating(ctx, r1); err != nil {
		t.Fatalf("failed to seed r1: %v", err)
	}
	if _, err := repo.CreateRating(ctx, r2); err != nil {
		t.Fatalf("failed to seed r2: %v", err)
	}
	if _, err := repo.CreateRating(ctx, r3); err != nil {
		t.Fatalf("failed to seed r3: %v", err)
	}
	if _, err := repo.CreateRating(ctx, rOther); err != nil {
		t.Fatalf("failed to seed rOther: %v", err)
	}

	// 2. Fetch ratings with default pagination
	ratings, avg, total, err := svc.GetProductRatings(ctx, targetProductID, 10, 0)
	if err != nil {
		t.Fatalf("failed to get product ratings: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if avg != 4.0 {
		t.Errorf("expected average rating 4.0, got %f", avg)
	}
	if len(ratings) != 3 {
		t.Errorf("expected 3 ratings in result list, got %d", len(ratings))
	}

	// 3. Test pagination: Limit=2, Offset=0
	page1, _, total, err := svc.GetProductRatings(ctx, targetProductID, 2, 0)
	if err != nil {
		t.Fatalf("failed page 1: %v", err)
	}
	if len(page1) != 2 || total != 3 {
		t.Errorf("expected page 1 to have 2 items with total 3, got len=%d total=%d", len(page1), total)
	}

	// 4. Test pagination: Limit=2, Offset=2
	page2, _, total, err := svc.GetProductRatings(ctx, targetProductID, 2, 2)
	if err != nil {
		t.Fatalf("failed page 2: %v", err)
	}
	if len(page2) != 1 || total != 3 {
		t.Errorf("expected page 2 to have 1 item with total 3, got len=%d total=%d", len(page2), total)
	}

	// 5. Test pagination clamping: Limit <= 0 clamped to 10, offset < 0 clamped to 0
	clampedRatings, clampedAvg, clampedTotal, err := svc.GetProductRatings(ctx, targetProductID, -5, -1)
	if err != nil {
		t.Fatalf("failed clamped ratings: %v", err)
	}
	if clampedTotal != 3 || len(clampedRatings) != 3 || clampedAvg != 4.0 {
		t.Errorf("expected clamped pagination to return all 3 items, got len=%d total=%d avg=%f",
			len(clampedRatings), clampedTotal, clampedAvg)
	}
}
