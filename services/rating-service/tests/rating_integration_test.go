package tests

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/repository"
)

func TestRatingDatabasePersistenceAndModeration(t *testing.T) {
	_ = godotenv.Load("../.env")
	dbURL := os.Getenv("RATINGS_SERVICE_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("RATING_SERVICE_DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("Skipping DB integration test: database URL not provided")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to postgres database: %v", err)
	}
	defer pool.Close()

	if migrator, mErr := database.NewMigrator(pool, "../migrations"); mErr == nil {
		if runErr := migrator.Run(ctx); runErr != nil {
			t.Logf("migrator run warning: %v", runErr)
		}
	}

	db := &database.DB{Pool: pool}
	repo := repository.NewRatingRepo(db)

	userID := uuid.New().String()
	flagUserID := uuid.New().String()
	productID := uuid.New().String()
	orderID := uuid.New().String()
	orderItemID := uuid.New().String()

	rating := &model.Rating{
		UserID:             userID,
		ProductID:          productID,
		OrderID:            orderID,
		OrderItemID:        orderItemID,
		Rating:             5,
		Review:             "Integration test product review text",
		IsVerifiedPurchase: true,
	}

	// 1. Test Create & Fetch Persistence
	createdRating, err := repo.CreateRating(ctx, rating)
	if err != nil {
		t.Fatalf("failed to persist rating in database: %v", err)
	}
	if createdRating.ID == "" {
		t.Fatalf("expected non-empty rating ID after database insert")
	}

	fetchedRating, err := repo.GetRatingByID(ctx, createdRating.ID)
	if err != nil {
		t.Fatalf("failed to fetch persisted rating by ID: %v", err)
	}
	if fetchedRating.Rating != 5 || fetchedRating.Review != rating.Review {
		t.Errorf("rating data mismatch: got rating = %d review = %s", fetchedRating.Rating, fetchedRating.Review)
	}

	// 2. Test Uniqueness Constraint: UNIQUE(user_id, order_item_id)
	duplicateRating := &model.Rating{
		UserID:             userID,
		ProductID:          productID,
		OrderID:            orderID,
		OrderItemID:        orderItemID,
		Rating:             4,
		Review:             "Duplicate review attempt",
		IsVerifiedPurchase: true,
	}
	_, dupErr := repo.CreateRating(ctx, duplicateRating)
	if dupErr == nil || !errors.Is(dupErr, repository.ErrRatingAlreadyExists) {
		t.Errorf("expected ErrRatingAlreadyExists for duplicate order_item rating, got: %v", dupErr)
	}

	// 3. Test Flag Creation & Uniqueness Constraint: UNIQUE(rating_id, user_id)
	flag := &model.RatingFlag{
		RatingID: createdRating.ID,
		UserID:   flagUserID,
		Category: "SPAM",
		Reason:   "Suspicious bot link",
	}
	if err := repo.CreateFlag(ctx, flag); err != nil {
		t.Fatalf("failed to persist rating flag: %v", err)
	}
	if flag.ID == "" {
		t.Fatalf("expected non-empty flag ID after insert")
	}

	dupFlagErr := repo.CreateFlag(ctx, flag)
	if dupFlagErr == nil || !errors.Is(dupFlagErr, repository.ErrRatingAlreadyExists) {
		t.Errorf("expected ErrRatingAlreadyExists for duplicate user flag, got: %v", dupFlagErr)
	}

	flaggedItems, totalFlagged, err := repo.GetFlaggedRatings(ctx, 10, 0)
	if err != nil {
		t.Fatalf("failed to get flagged ratings: %v", err)
	}
	if totalFlagged < 1 || len(flaggedItems) < 1 {
		t.Errorf("expected at least 1 flagged rating in queue, got total = %d items = %d", totalFlagged, len(flaggedItems))
	}

	// 4. Test Moderation: Hide Rating
	if err := repo.ModerateRatings(ctx, true, createdRating.ID); err != nil {
		t.Fatalf("failed to hide rating: %v", err)
	}

	// Verify hidden review is excluded from public product ratings
	publicRatings, avg, total, _, err := repo.GetProductRatings(ctx, productID, 10, 0)
	if err != nil {
		t.Fatalf("failed to get product ratings: %v", err)
	}
	if total != 0 || len(publicRatings) != 0 || avg != 0.0 {
		t.Errorf("expected 0 public ratings after hiding, got total = %d items = %d avg = %f", total, len(publicRatings), avg)
	}

	// Verify hidden review is removed from flagged moderation queue
	flaggedAfterHide, totalAfterHide, err := repo.GetFlaggedRatings(ctx, 10, 0)
	if err != nil {
		t.Fatalf("failed to get flagged ratings after hiding: %v", err)
	}
	for _, item := range flaggedAfterHide {
		if item.Rating.ID == createdRating.ID {
			t.Errorf("expected hidden rating to be excluded from flagged queue, but found in results (total=%d)", totalAfterHide)
		}
	}

	// Verify flag record was soft-deleted in rating_flags table
	var flagDeletedAt *time.Time
	err = pool.QueryRow(ctx, "SELECT deleted_at FROM rating_flags WHERE id = $1", flag.ID).Scan(&flagDeletedAt)
	if err != nil || flagDeletedAt == nil {
		t.Errorf("expected rating flag to be soft-deleted in database, got deletedAt=%v err=%v", flagDeletedAt, err)
	}

	// 5. Test Moderation: Unhide Rating
	if err := repo.ModerateRatings(ctx, false, createdRating.ID); err != nil {
		t.Fatalf("failed to unhide ratings: %v", err)
	}

	publicRatingsAfterUnhide, avgAfterUnhide, totalAfterUnhide, _, err := repo.GetProductRatings(ctx, productID, 10, 0)
	if err != nil {
		t.Fatalf("failed to get product ratings after unhiding: %v", err)
	}
	if totalAfterUnhide != 1 || len(publicRatingsAfterUnhide) != 1 || avgAfterUnhide != 5.0 {
		t.Errorf("expected 1 public rating after unhiding, got total=%d avg=%f", totalAfterUnhide, avgAfterUnhide)
	}

	ratingAfterUnhide, err := repo.GetRatingByID(ctx, createdRating.ID)
	if err != nil {
		t.Fatalf("failed to fetch rating after unhide: %v", err)
	}
	if ratingAfterUnhide.IsHidden {
		t.Errorf("expected IsHidden to be false, got true")
	}
	if ratingAfterUnhide.IsFlagged {
		t.Errorf("expected IsFlagged to be reset to false after unhide, got true")
	}

	// 6. Test Soft Delete
	if err := repo.DeleteRating(ctx, createdRating.ID); err != nil {
		t.Fatalf("failed to soft delete rating: %v", err)
	}

	_, getErr := repo.GetRatingByID(ctx, createdRating.ID)
	if getErr == nil || !errors.Is(getErr, repository.ErrRatingNotFound) {
		t.Errorf("expected ErrRatingNotFound for soft-deleted rating, got: %v", getErr)
	}
}
