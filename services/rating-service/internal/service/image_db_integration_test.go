package service_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/service"
)

const pixelReviewBase64 = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func TestImageDBIntegration_RatingService(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dbURL := os.Getenv("RATINGS_SERVICE_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("RATING_SERVICE_DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("Skipping Ratings DB integration test: RATINGS_SERVICE_DATABASE_URL or DATABASE_URL not set")
		return
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("Skipping Ratings DB integration test: failed to initialize pool: %v", err)
		return
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Skipping Ratings DB integration test: database not reachable: %v", err)
		return
	}

	repo := repository.NewRatingRepo(&database.DB{Pool: pool})
	authorID := uuid.New().String()
	orderID := uuid.New().String()
	orderItemID := uuid.New().String()
	productID := uuid.New().String()

	mockOrderCli := &mockOrderClient{
		orders: map[string]*orderpb.Order{
			orderID: {
				Id:          orderID,
				UserId:      authorID,
				OrderStatus: "DELIVERED",
				Items: []*orderpb.OrderItem{
					{Id: orderItemID, ProductId: productID},
				},
			},
		},
	}

	t.Run("PASS_CASE: Successful upload stores Supabase Storage URL in rating_images table", func(t *testing.T) {
		uploader := &mockUploader{}
		svc := service.NewRatingService(repo, mockOrderCli, uploader, nil)

		r, err := svc.CreateRating(ctx, dto.CreateRatingInput{
			UserID:      authorID,
			OrderID:     orderID,
			OrderItemID: orderItemID,
			ProductID:   productID,
			Rating:      5,
			Review:      "DB Verified Review Pass Case",
			Images:      []string{pixelReviewBase64},
		})
		if err != nil {
			t.Fatalf("CreateRating failed: %v", err)
		}
		defer func() {
			_, _ = pool.Exec(ctx, "DELETE FROM rating_images WHERE rating_id = $1", r.ID)
			_, _ = pool.Exec(ctx, "DELETE FROM ratings WHERE id = $1", r.ID)
		}()

		// DIRECT DB QUERY: rating_images table
		var dbImageURL string
		err = pool.QueryRow(ctx, "SELECT url FROM rating_images WHERE rating_id = $1 LIMIT 1", r.ID).Scan(&dbImageURL)
		if err != nil {
			t.Fatalf("DB query for rating_images table failed: %v", err)
		}

		t.Logf("[DB_CHECK] Table 'rating_images', rating_id='%s', url='%s'", r.ID, dbImageURL)

		if strings.HasPrefix(dbImageURL, "data:") {
			t.Errorf("DB cross-check failed: rating_images.url must NOT be base64, got: %s", dbImageURL)
		}
		if !strings.HasPrefix(dbImageURL, "https://cdn.gocart.local/") {
			t.Errorf("DB cross-check failed: rating_images.url must have Storage URL prefix, got: %s", dbImageURL)
		}
	})

	t.Run("FAIL_CASE_1: Storage unavailable (nil uploader) returns error and prevents Base64 record in DB", func(t *testing.T) {
		orderItem2 := uuid.New().String()
		mockOrderCli2 := &mockOrderClient{
			orders: map[string]*orderpb.Order{
				orderID: {
					Id:          orderID,
					UserId:      authorID,
					OrderStatus: "DELIVERED",
					Items: []*orderpb.OrderItem{
						{Id: orderItem2, ProductId: productID},
					},
				},
			},
		}
		svc := service.NewRatingService(repo, mockOrderCli2, nil, nil) // uploader = nil

		r, err := svc.CreateRating(ctx, dto.CreateRatingInput{
			UserID:      authorID,
			OrderID:     orderID,
			OrderItemID: orderItem2,
			ProductID:   productID,
			Rating:      5,
			Review:      "DB Verified Review Nil Uploader",
			Images:      []string{pixelReviewBase64},
		})
		if err == nil {
			t.Fatal("expected error when storage uploader is unavailable, got nil")
		}
		if !strings.Contains(err.Error(), "storage uploader not configured") {
			t.Fatalf("unexpected error message: %v", err)
		}

		// DIRECT DB QUERY: verify no rating or images were written
		var count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM ratings WHERE order_item_id = $1", orderItem2).Scan(&count)
		t.Logf("[DB_CHECK] Table 'ratings', OrderItemID='%s', count=%d (0 expected)", orderItem2, count)
		if count != 0 {
			t.Errorf("DB cross-check failed: rating record should not exist in DB after upload error, found %d", count)
		}
		if r != nil {
			_, _ = pool.Exec(ctx, "DELETE FROM rating_images WHERE rating_id = $1", r.ID)
			_, _ = pool.Exec(ctx, "DELETE FROM ratings WHERE id = $1", r.ID)
		}
	})

	t.Run("FAIL_CASE_2: Storage upload failure returns error and prevents Base64 record in DB", func(t *testing.T) {
		orderItem3 := uuid.New().String()
		mockOrderCli3 := &mockOrderClient{
			orders: map[string]*orderpb.Order{
				orderID: {
					Id:          orderID,
					UserId:      authorID,
					OrderStatus: "DELIVERED",
					Items: []*orderpb.OrderItem{
						{Id: orderItem3, ProductId: productID},
					},
				},
			},
		}
		uploader := &mockUploader{err: context.DeadlineExceeded}
		svc := service.NewRatingService(repo, mockOrderCli3, uploader, nil)

		r, err := svc.CreateRating(ctx, dto.CreateRatingInput{
			UserID:      authorID,
			OrderID:     orderID,
			OrderItemID: orderItem3,
			ProductID:   productID,
			Rating:      5,
			Review:      "DB Verified Review Upload Failure",
			Images:      []string{pixelReviewBase64},
		})
		if err == nil {
			t.Fatal("expected error when storage upload fails, got nil")
		}
		if !strings.Contains(err.Error(), "failed to upload image to storage") {
			t.Fatalf("unexpected error message: %v", err)
		}

		// DIRECT DB QUERY: verify no rating or images were written
		var count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM ratings WHERE order_item_id = $1", orderItem3).Scan(&count)
		t.Logf("[DB_CHECK] Table 'ratings', OrderItemID='%s', count=%d (0 expected)", orderItem3, count)
		if count != 0 {
			t.Errorf("DB cross-check failed: rating record should not exist in DB after upload failure, found %d", count)
		}
		if r != nil {
			_, _ = pool.Exec(ctx, "DELETE FROM rating_images WHERE rating_id = $1", r.ID)
			_, _ = pool.Exec(ctx, "DELETE FROM ratings WHERE id = $1", r.ID)
		}
	})
}
