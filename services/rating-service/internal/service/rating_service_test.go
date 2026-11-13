package service_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
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
	flags     map[string]*model.RatingFlag
	images    map[string]*model.RatingImage
	idCounter int
}

func newMockRatingRepo() *mockRatingRepo {
	return &mockRatingRepo{
		ratings: make(map[string]*model.Rating),
		flags:   make(map[string]*model.RatingFlag),
		images:  make(map[string]*model.RatingImage),
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

	for i := range r.Images {
		r.Images[i].ID = uuid.New().String()
		r.Images[i].RatingID = r.ID
		r.Images[i].CreatedAt = now
		imgCopy := r.Images[i]
		m.images[imgCopy.ID] = &imgCopy
	}

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
	cp.Images = nil
	for _, img := range m.images {
		if img.RatingID == id && img.DeletedAt == nil {
			cp.Images = append(cp.Images, *img)
		}
	}
	return &cp, nil
}

func (m *mockRatingRepo) GetProductRatings(ctx context.Context, productID string, limit, offset int32) ([]*model.Rating, float64, int32, dto.StarRating, error) {
	var matched []*model.Rating
	var sum float64
	var stars dto.StarRating

	for _, r := range m.ratings {
		if r.ProductID == productID && !r.IsHidden && r.DeletedAt == nil {
			cp := *r
			matched = append(matched, &cp)
			sum += float64(r.Rating)

			switch r.Rating {
			case 5:
				stars.FiveStar++
			case 4:
				stars.FourStar++
			case 3:
				stars.ThreeStar++
			case 2:
				stars.TwoStar++
			case 1:
				stars.OneStar++
			}
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

	return matched[start:end], avg, total, stars, nil
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

func (m *mockRatingRepo) ModerateRatings(ctx context.Context, isHidden bool, ratingID string) error {
	r, ok := m.ratings[ratingID]
	if !ok || r.DeletedAt != nil {
		return repository.ErrRatingNotFound
	}

	r.IsHidden = isHidden
	if !isHidden {
		r.IsFlagged = false
	}
	r.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *mockRatingRepo) CreateFlag(ctx context.Context, f *model.RatingFlag) error {
	r, ok := m.ratings[f.RatingID]
	if !ok || r.DeletedAt != nil {
		return repository.ErrRatingNotFound
	}
	key := fmt.Sprintf("%s:%s", f.RatingID, f.UserID)
	if _, exists := m.flags[key]; exists {
		return repository.ErrRatingAlreadyExists
	}
	r.IsFlagged = true
	m.flags[key] = f
	return nil
}

func (m *mockRatingRepo) GetFlaggedRatings(ctx context.Context, limit, offset int32) ([]*dto.FlaggedRatingItem, int32, error) {
	var items []*dto.FlaggedRatingItem
	for _, r := range m.ratings {
		if r.IsFlagged && !r.IsHidden && r.DeletedAt == nil {
			var flags []*model.RatingFlag
			for _, f := range m.flags {
				if f.RatingID == r.ID {
					cp := *f
					flags = append(flags, &cp)
				}
			}
			cp := *r
			items = append(items, &dto.FlaggedRatingItem{
				Rating:    &cp,
				FlagCount: int32(len(flags)),
				Flags:     flags,
			})
		}
	}
	total := int32(len(items))
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if limit <= 0 || end > total {
		end = total
	}
	return items[start:end], total, nil
}

func (m *mockRatingRepo) AddRatingImages(ctx context.Context, ratingID string, urls []string) ([]*model.RatingImage, error) {
	now := time.Now().UTC()
	created := make([]*model.RatingImage, 0, len(urls))
	for _, u := range urls {
		img := &model.RatingImage{
			ID:        uuid.New().String(),
			RatingID:  ratingID,
			URL:       u,
			CreatedAt: now,
		}
		m.images[img.ID] = img
		created = append(created, img)
	}
	return created, nil
}

func (m *mockRatingRepo) GetRatingImageByID(ctx context.Context, imageID string) (*model.RatingImage, error) {
	img, ok := m.images[imageID]
	if !ok || img.DeletedAt != nil {
		return nil, repository.ErrRatingImageNotFound
	}
	cp := *img
	return &cp, nil
}

func (m *mockRatingRepo) DeleteRatingImage(ctx context.Context, imageID string) error {
	img, ok := m.images[imageID]
	if !ok || img.DeletedAt != nil {
		return repository.ErrRatingImageNotFound
	}
	now := time.Now().UTC()
	img.DeletedAt = &now
	return nil
}

type mockUploader struct {
	uploadedURL string
	err         error
}

func (m *mockUploader) Upload(ctx context.Context, key string, body io.Reader, contentType string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.GetPublicURL(key), nil
}

func (m *mockUploader) UploadBytes(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.uploadedURL != "" {
		return m.uploadedURL, nil
	}
	return m.GetPublicURL(key), nil
}

func (m *mockUploader) Delete(ctx context.Context, key string) error {
	return m.err
}

func (m *mockUploader) GetPublicURL(key string) string {
	return "https://cdn.gocart.local/" + key
}

func (m *mockUploader) GenerateKey(prefix, filename string) string {
	return prefix + "/" + filename
}

func (m *mockUploader) GetPresignedPutURL(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error) {
	return "https://cdn.gocart.local/presigned/" + key, m.err
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

	svc := service.NewRatingService(repo, orderClient, nil, nil)

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
	ratings, avg, total, _, err := svc.GetProductRatings(ctx, prod1, 10, 0)
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

	svc := service.NewRatingService(repo, orderClient, nil, nil)

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
	nilClientSvc := service.NewRatingService(repo, nil, nil, nil)
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
	_, _, _, _, err = svc.GetProductRatings(ctx, "", 10, 0)
	if err == nil || !strings.Contains(err.Error(), "product id is required") {
		t.Errorf("expected product id required error, got: %v", err)
	}

	_, _, _, _, err = svc.GetProductRatings(ctx, "invalid-uuid", 10, 0)
	if err == nil || !strings.Contains(err.Error(), "invalid product id format") {
		t.Errorf("expected invalid product id error, got: %v", err)
	}
}

func TestRatingService_ProductRatings_Aggregation(t *testing.T) {
	ctx := context.Background()
	repo := newMockRatingRepo()
	svc := service.NewRatingService(repo, &mockOrderClient{}, nil, nil)

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
	ratings, avg, total, stars, err := svc.GetProductRatings(ctx, targetProductID, 10, 0)
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
	if stars.FiveStar != 1 || stars.FourStar != 1 || stars.ThreeStar != 1 {
		t.Errorf("expected 1 of each star (5,4,3), got 5 star = %d 4 star = %d 3 star = %d", stars.FiveStar, stars.FourStar, stars.ThreeStar)
	}

	// 3. Test pagination: Limit=2, Offset=0
	page1, _, total, _, err := svc.GetProductRatings(ctx, targetProductID, 2, 0)
	if err != nil {
		t.Fatalf("failed page 1: %v", err)
	}
	if len(page1) != 2 || total != 3 {
		t.Errorf("expected page 1 to have 2 items with total 3, got len=%d total=%d", len(page1), total)
	}

	// 4. Test pagination: Limit=2, Offset=2
	page2, _, total, _, err := svc.GetProductRatings(ctx, targetProductID, 2, 2)
	if err != nil {
		t.Fatalf("failed page 2: %v", err)
	}
	if len(page2) != 1 || total != 3 {
		t.Errorf("expected page 2 to have 1 item with total 3, got len=%d total=%d", len(page2), total)
	}

	// 5. Test pagination clamping: Limit <= 0 clamped to 10, offset < 0 clamped to 0
	clampedRatings, clampedAvg, clampedTotal, _, err := svc.GetProductRatings(ctx, targetProductID, -5, -1)
	if err != nil {
		t.Fatalf("failed clamped ratings: %v", err)
	}
	if clampedTotal != 3 || len(clampedRatings) != 3 || clampedAvg != 4.0 {
		t.Errorf("expected clamped pagination to return all 3 items, got len=%d total=%d avg=%f",
			len(clampedRatings), clampedTotal, clampedAvg)
	}
}

func TestRatingService_FlagRating(t *testing.T) {
	ctx := context.Background()
	repo := newMockRatingRepo()
	svc := service.NewRatingService(repo, &mockOrderClient{}, nil, nil)

	// 1. Missing rating ID
	err := svc.FlagRating(ctx, dto.FlagRatingInput{
		RatingID: "",
		UserID:   uuid.New().String(),
		Category: "SPAM",
	})
	if err == nil || !strings.Contains(err.Error(), "rating id is required") {
		t.Errorf("expected rating id required error, got: %v", err)
	}

	// 2. Invalid category
	r, err := repo.CreateRating(ctx, &model.Rating{
		UserID:      uuid.New().String(),
		ProductID:   uuid.New().String(),
		OrderID:     uuid.New().String(),
		OrderItemID: uuid.New().String(),
		Rating:      5,
		Review:      "Legit review",
	})
	if err != nil {
		t.Fatalf("failed to create rating: %v", err)
	}

	err = svc.FlagRating(ctx, dto.FlagRatingInput{
		RatingID: r.ID,
		UserID:   uuid.New().String(),
		Category: "INVALID_CAT",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid category") {
		t.Errorf("expected invalid category error, got: %v", err)
	}

	// 3. Flagging a rating without review text should fail
	rNoReview, err := repo.CreateRating(ctx, &model.Rating{
		UserID:      uuid.New().String(),
		ProductID:   uuid.New().String(),
		OrderID:     uuid.New().String(),
		OrderItemID: uuid.New().String(),
		Rating:      5,
		Review:      "",
	})
	if err != nil {
		t.Fatalf("failed to create rating: %v", err)
	}

	err = svc.FlagRating(ctx, dto.FlagRatingInput{
		RatingID: rNoReview.ID,
		UserID:   uuid.New().String(),
		Category: "SPAM",
		Reason:   "Spam",
	})
	if err == nil || !strings.Contains(err.Error(), "cannot flag a rating without review text") {
		t.Errorf("expected error for flagging rating without review text, got: %v", err)
	}

	// 4. Success flagging
	err = svc.FlagRating(ctx, dto.FlagRatingInput{
		RatingID: r.ID,
		UserID:   uuid.New().String(),
		Category: "SPAM",
		Reason:   "Suspicious link",
	})
	if err != nil {
		t.Fatalf("expected successful flag, got: %v", err)
	}
}

func TestRatingService_GetFlaggedRatings(t *testing.T) {
	ctx := context.Background()
	repo := newMockRatingRepo()
	svc := service.NewRatingService(repo, &mockOrderClient{}, nil, nil)

	// 1. Missing admin ID
	_, _, err := svc.GetFlaggedRatings(ctx, dto.GetFlaggedRatingsInput{
		AdminId: "",
	})

	if err == nil || !strings.Contains(err.Error(), "admin_id is required") {
		t.Errorf("expected admin_id required error, got: %v", err)
	}

	// 2. Success with flagged rating
	r, err := repo.CreateRating(ctx, &model.Rating{
		UserID:      uuid.New().String(),
		ProductID:   uuid.New().String(),
		OrderID:     uuid.New().String(),
		OrderItemID: uuid.New().String(),
		Rating:      5,
		Review:      "Spam product",
	})
	if err != nil {
		t.Fatalf("failed to create rating: %v", err)
	}

	_ = repo.CreateFlag(ctx, &model.RatingFlag{
		RatingID: r.ID,
		UserID:   uuid.New().String(),
		Category: "SPAM",
		Reason:   "Bot link",
	})

	items, total, err := svc.GetFlaggedRatings(ctx, dto.GetFlaggedRatingsInput{
		AdminId: "admin-101",
		Limit:   10,
		Offset:  0,
	})
	if err != nil {
		t.Fatalf("unexpected error fetching flagged ratings: %v", err)
	}

	if total != 1 || len(items) != 1 {
		t.Errorf("expected 1 flagged rating, got total %d items %d", total, len(items))
	}

	if items[0].FlagCount != 1 || len(items[0].Flags) != 1 {
		t.Errorf("expected 1 flag attached, got flagCount %d", items[0].FlagCount)
	}
}

func TestRatingService_ModerateRatings(t *testing.T) {
	ctx := context.Background()
	repo := newMockRatingRepo()
	svc := service.NewRatingService(repo, &mockOrderClient{}, nil, nil)

	// 1. Missing rating ID
	err := svc.ModerateRatings(ctx, dto.ModerateRatingInput{
		RatingId: "",
		AdminId:  "admin-101",
		IsHidden: true,
	})
	if err == nil || !strings.Contains(err.Error(), "rating id is required") {
		t.Fatalf("expected 'rating id is required', got: %v", err)
	}

	// 2. Invalid UUID format
	err = svc.ModerateRatings(ctx, dto.ModerateRatingInput{
		RatingId: "invalid-uuid",
		AdminId:  "admin-101",
		IsHidden: true,
	})

	if err == nil || !strings.Contains(err.Error(), "invalid rating id format") {
		t.Fatalf("expected 'invalid rating id format', got: %v", err)
	}

	// 3. Missing Admin ID
	err = svc.ModerateRatings(ctx, dto.ModerateRatingInput{
		RatingId: uuid.New().String(),
		AdminId:  "",
		IsHidden: true,
	})
	if err == nil || !strings.Contains(err.Error(), "admin_id is required") {
		t.Fatalf("expected 'admin_id is required', got: %v", err)
	}

	// 4. Rating Not Found
	err = svc.ModerateRatings(ctx, dto.ModerateRatingInput{
		RatingId: uuid.New().String(),
		AdminId:  "admin-101",
		IsHidden: true,
	})
	if err == nil || !strings.Contains(err.Error(), "rating not found") {
		t.Fatalf("expected 'rating not found', got: %v", err)
	}

	// 5. Success: Hide Rating
	r, err := repo.CreateRating(ctx, &model.Rating{
		UserID:      uuid.New().String(),
		ProductID:   uuid.New().String(),
		OrderID:     uuid.New().String(),
		OrderItemID: uuid.New().String(),
		Rating:      4,
		Review:      "Questionable review",
	})
	if err != nil {
		t.Fatalf("failed to create rating: %v", err)
	}

	_ = repo.CreateFlag(ctx, &model.RatingFlag{
		RatingID: r.ID,
		UserID:   uuid.New().String(),
		Category: "INAPPROPRIATE_CONTENT",
		Reason:   "Bad words",
	})

	err = svc.ModerateRatings(ctx, dto.ModerateRatingInput{
		RatingId: r.ID,
		AdminId:  "admin-101",
		IsHidden: true,
	})
	if err != nil {
		t.Fatalf("unexpected error moderating rating: %v", err)
	}

	flagged, total, err := svc.GetFlaggedRatings(ctx, dto.GetFlaggedRatingsInput{
		AdminId: "admin-101",
	})
	if err != nil {
		t.Fatalf("unexpected error fetching flagged ratings: %v", err)
	}
	if total != 0 || len(flagged) != 0 {
		t.Fatalf("expected 0 flagged rating after hiding, got: %v", err)
	}

	// 6. Success: Unhide and clear flag
	err = svc.ModerateRatings(ctx, dto.ModerateRatingInput{
		RatingId: r.ID,
		AdminId:  "admin-101",
		IsHidden: false,
	})
	if err != nil {
		t.Fatalf("unexpected error unhiding rating: %v", err)
	}

	updated, err := repo.GetRatingByID(ctx, r.ID)
	if err != nil {
		t.Fatalf("failed to get rating: %v", err)
	}

	if updated.IsHidden {
		t.Errorf("expected IsHidden to be false, got true")
	}
	if updated.IsFlagged {
		t.Errorf("expected IsFlagged to be cleared to false, got true")
	}

}

func TestRatingService_UploadAndDeleteImages(t *testing.T) {
	ctx := context.Background()
	repo := newMockRatingRepo()
	uploader := &mockUploader{}
	svc := service.NewRatingService(repo, &mockOrderClient{}, uploader, nil)

	authorID := uuid.New().String()
	otherUserID := uuid.New().String()

	r, err := repo.CreateRating(ctx, &model.Rating{
		UserID:      authorID,
		ProductID:   uuid.New().String(),
		OrderID:     uuid.New().String(),
		OrderItemID: uuid.New().String(),
		Rating:      5,
		Review:      "Great product with photos",
	})
	if err != nil {
		t.Fatalf("failed to seed rating: %v", err)
	}

	_, err = svc.UploadRatingImages(ctx, dto.UploadRatingImagesInput{
		RatingID: r.ID,
		UserID:   otherUserID,
		Images:   []string{"https://example.com/1.jpg"},
	})
	if err == nil || !strings.Contains(err.Error(), "not authorized") {
		t.Errorf("expected forbidden error for non-author, got: %v", err)
	}

	_, err = svc.UploadRatingImages(ctx, dto.UploadRatingImagesInput{
		RatingID: r.ID,
		UserID:   authorID,
		Images:   []string{"data:application/pdf;base64,JVBERi0x"},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported image format") {
		t.Errorf("expected unsupported image format error, got: %v", err)
	}

	b64PNG := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("fake-png-bytes"))
	saved, err := svc.UploadRatingImages(ctx, dto.UploadRatingImagesInput{
		RatingID: r.ID,
		UserID:   authorID,
		Images:   []string{b64PNG, "https://example.com/direct.jpg"},
	})
	if err != nil {
		t.Fatalf("expected upload to suceed, got: %v", err)
	}
	if len(saved) != 2 {
		t.Fatalf("expected 2 saved images,got: %d", len(saved))
	}
	if !strings.HasPrefix(saved[0].URL, "https://cdn.gocart.local/ratings/") {
		t.Errorf("expected S3 uploaded URL, got: %s", saved[0].URL)
	}
	if saved[1].URL != "https://example.com/direct.jpg" {
		t.Errorf("expected direct URL preserved, got: %s", saved[1].URL)
	}

	_, err = svc.UploadRatingImages(ctx, dto.UploadRatingImagesInput{
		RatingID: r.ID,
		UserID:   authorID,
		Images:   []string{"https://a.com/1.jpg", "https://a.com/2.jpg", "https://a.com/3.jpg", "https://a.com/4.jpg"},
	})
	if err == nil || !strings.Contains(err.Error(), "maximum 5 images") {
		t.Errorf("expected max 5 images error, got: %v", err)
	}

	err = svc.DeleteRatingImage(ctx, dto.DeleteRatingImageInput{
		ImageID: saved[0].ID,
		UserID:  otherUserID,
	})
	if err == nil || !strings.Contains(err.Error(), "not authorized") {
		t.Errorf("expected forbidden error on delete by non-author, got: %v", err)
	}

	err = svc.DeleteRatingImage(ctx, dto.DeleteRatingImageInput{
		ImageID: saved[0].ID,
		UserID:  authorID,
	})
	if err != nil {
		t.Fatalf("expected delete image to suceed, got: %v", err)
	}

	fetched, err := svc.GetRating(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRating failed: %v", err)
	}
	if len(fetched.Images) != 1 || fetched.Images[0].ID != saved[1].ID {
		t.Errorf("expected 1 remaining image after deletion, got %d", len(fetched.Images))
	}

}

func TestRatingService_CreateRating_WithImages(t *testing.T) {
	ctx := context.Background()
	repo := newMockRatingRepo()
	svc := service.NewRatingService(repo, &mockOrderClient{
		orders: map[string]*orderpb.Order{
			"11111111-1111-1111-1111-111111111111": {
				Id:          "11111111-1111-1111-1111-111111111111",
				UserId:      "22222222-2222-2222-2222-222222222222",
				OrderStatus: "DELIVERED",
				Items: []*orderpb.OrderItem{
					{Id: "33333333-3333-3333-3333-333333333333", ProductId: "44444444-4444-4444-4444-444444444444"},
				},
			},
		},
	}, &mockUploader{}, nil)

	_, err := svc.CreateRating(ctx, dto.CreateRatingInput{
		UserID:      "22222222-2222-2222-2222-222222222222",
		OrderID:     "11111111-1111-1111-1111-111111111111",
		OrderItemID: "33333333-3333-3333-3333-333333333333",
		ProductID:   "44444444-4444-4444-4444-444444444444",
		Rating:      5,
		Images:      []string{"https://a.com/1.jpg", "https://a.com/2.jpg", "https://a.com/3.jpg", "https://a.com/4.jpg", "https://a.com/5.jpg", "https://a.com/6.jpg"},
	})
	if err == nil || !strings.Contains(err.Error(), "maximum 5 images") {
		t.Fatalf("expected max 5 images error, got: %v", err)
	}

	created, err := svc.CreateRating(ctx, dto.CreateRatingInput{
		UserID:      "22222222-2222-2222-2222-222222222222",
		OrderID:     "11111111-1111-1111-1111-111111111111",
		OrderItemID: "33333333-3333-3333-3333-333333333333",
		ProductID:   "44444444-4444-4444-4444-444444444444",
		Rating:      5,
		Review:      "Great with photo",
		Images:      []string{"https://cdn.gocart.local/ratings/photo.jpg"},
	})
	if err != nil {
		t.Fatalf("expected CreateRating with image to succeed, got: %v", err)
	}
	if len(created.Images) != 1 || created.Images[0].URL != "https://cdn.gocart.local/ratings/photo.jpg" {
		t.Errorf("expected 1 saved image, got: %+v", created.Images)
	}
}
