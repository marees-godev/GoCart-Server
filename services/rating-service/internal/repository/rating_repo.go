package repository

import (
	"context"

	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/rating-service/internal/model"
)

var (
	ErrRatingAlreadyExists = errors.New("user has already submitted a rating for this order item")
	ErrRatingNotFound      = errors.New("rating not found")
	ErrRatingImageNotFound = errors.New("rating image not found")
)

type RatingRepository interface {
	CreateRating(ctx context.Context, r *model.Rating) (*model.Rating, error)
	GetRatingByID(ctx context.Context, id string) (*model.Rating, error)
	GetProductRatings(ctx context.Context, productID string, limit, offset int32) ([]*model.Rating, float64, int32, dto.StarRating, error)
	UpdateRating(ctx context.Context, id string, rating int, review string) (*model.Rating, error)
	DeleteRating(ctx context.Context, id string) error
	CreateFlag(ctx context.Context, f *model.RatingFlag) error
	GetFlaggedRatings(ctx context.Context, limit, offset int32) ([]*dto.FlaggedRatingItem, int32, error)
	ModerateRatings(ctx context.Context, ishidden bool, ratingID string) error
	AddRatingImages(ctx context.Context, ratingID string, urls []string) ([]*model.RatingImage, error)
	GetRatingImageByID(ctx context.Context, imageID string) (*model.RatingImage, error)
	DeleteRatingImage(ctx context.Context, imageID string) error
}

type RatingRepo struct {
	db *database.DB
}

func NewRatingRepo(db *database.DB) *RatingRepo {
	return &RatingRepo{db: db}
}

func (r *RatingRepo) CreateRating(ctx context.Context, rating *model.Rating) (*model.Rating, error) {
	now := time.Now().UTC()
	if rating.CreatedAt.IsZero() {
		rating.CreatedAt = now
	}
	rating.UpdatedAt = now

	query := `
		INSERT INTO ratings (
			user_id, product_id, order_id, order_item_id,
			rating, review, is_verified_purchase,
			is_flagged, is_hidden, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9, $10, $11
		)
		RETURNING id, created_at, updated_at;
	`

	err := r.db.Pool.QueryRow(
		ctx,
		query,
		rating.UserID,
		rating.ProductID,
		rating.OrderID,
		rating.OrderItemID,
		rating.Rating,
		rating.Review,
		rating.IsVerifiedPurchase,
		rating.IsFlagged,
		rating.IsHidden,
		rating.CreatedAt,
		rating.UpdatedAt,
	).Scan(&rating.ID, &rating.CreatedAt, &rating.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrRatingAlreadyExists
		}
		return nil, fmt.Errorf("failed to insert rating: %w", err)
	}

	return rating, nil
}

func (r *RatingRepo) GetRatingByID(ctx context.Context, id string) (*model.Rating, error) {

	query := `
		SELECT id, user_id, product_id, order_id, order_item_id,
		       rating, review, is_verified_purchase,
		       is_flagged, is_hidden, created_at, updated_at
		FROM ratings
		WHERE id = $1 AND deleted_at IS NULL
	`

	var rt model.Rating
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&rt.ID,
		&rt.UserID,
		&rt.ProductID,
		&rt.OrderID,
		&rt.OrderItemID,
		&rt.Rating,
		&rt.Review,
		&rt.IsVerifiedPurchase,
		&rt.IsFlagged,
		&rt.IsHidden,
		&rt.CreatedAt,
		&rt.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRatingNotFound
		}
		return nil, fmt.Errorf("failed to query rating: %w", err)
	}

	if imgMap, err := r.fetchImagesForRatings(ctx, []string{rt.ID}); err == nil {
		rt.Images = imgMap[rt.ID]
	}

	return &rt, nil
}

func (r *RatingRepo) GetProductRatings(ctx context.Context, productID string, limit, offset int32) ([]*model.Rating, float64, int32, dto.StarRating, error) {
	var total int32
	var avg float64
	var star dto.StarRating

	summaryQuery := `
		SELECT 
		COUNT(*) AS total_ratings,
		COALESCE(ROUND(AVG(rating)::numeric, 1), 0.0) AS average_rating,
		COUNT(*) FILTER (WHERE rating = 5) AS five_star,
		COUNT(*) FILTER (WHERE rating = 4) AS four_star,
		COUNT(*) FILTER (WHERE rating = 3) AS three_star,
		COUNT(*) FILTER (WHERE rating = 2) AS two_star,
		COUNT(*) FILTER (WHERE rating = 1) AS one_star
		FROM ratings
		WHERE product_id = $1 
		AND is_hidden = false 
		AND deleted_at IS NULL
	`
	if err := r.db.Pool.QueryRow(ctx, summaryQuery, productID).Scan(&total, &avg, &star.FiveStar, &star.FourStar, &star.ThreeStar, &star.TwoStar, &star.OneStar); err != nil {
		return nil, 0, 0, dto.StarRating{}, fmt.Errorf("failed to get rating summary: %w", err)
	}

	if total == 0 {
		return []*model.Rating{}, 0, 0, dto.StarRating{}, nil
	}

	listQuery := `
		SELECT id, user_id, product_id, order_id, order_item_id,
		       rating, review, is_verified_purchase,
		       is_flagged, is_hidden, created_at, updated_at
		FROM ratings
		WHERE product_id = $1 AND is_hidden = false AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.Pool.Query(ctx, listQuery, productID, limit, offset)
	if err != nil {
		return nil, 0, 0, dto.StarRating{}, fmt.Errorf("failed to query product ratings: %w", err)
	}
	defer rows.Close()

	var ratings []*model.Rating
	for rows.Next() {
		var rt model.Rating
		if err := rows.Scan(
			&rt.ID, &rt.UserID, &rt.ProductID, &rt.OrderID, &rt.OrderItemID,
			&rt.Rating, &rt.Review, &rt.IsVerifiedPurchase,
			&rt.IsFlagged, &rt.IsHidden, &rt.CreatedAt, &rt.UpdatedAt,
		); err != nil {
			return nil, 0, 0, dto.StarRating{}, fmt.Errorf("failed to scan rating: %w", err)
		}
		ratings = append(ratings, &rt)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, 0, dto.StarRating{}, fmt.Errorf("error during rating rows iteration: %w", err)
	}

	ratingIDs := make([]string, len(ratings))
	for i, rt := range ratings {
		ratingIDs[i] = rt.ID
	}

	if imgMap, err := r.fetchImagesForRatings(ctx, ratingIDs); err == nil {
		for _, rt := range ratings {
			rt.Images = imgMap[rt.ID]
		}
	}

	return ratings, avg, total, star, nil
}

func (r *RatingRepo) UpdateRating(ctx context.Context, id string, rating int, review string) (*model.Rating, error) {

	updateQuery := `
		UPDATE ratings
		SET rating = $2, review = $3, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING 
			id, user_id, product_id, order_id, order_item_id, rating, review, 
			is_verified_purchase, is_flagged, is_hidden, created_at, updated_at`

	var rt model.Rating
	err := r.db.Pool.QueryRow(ctx, updateQuery, id, rating, review).Scan(
		&rt.ID,
		&rt.UserID,
		&rt.ProductID,
		&rt.OrderID,
		&rt.OrderItemID,
		&rt.Rating,
		&rt.Review,
		&rt.IsVerifiedPurchase,
		&rt.IsFlagged,
		&rt.IsHidden,
		&rt.CreatedAt,
		&rt.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRatingNotFound
		}
		return nil, fmt.Errorf("failed to update rating: %w", err)
	}

	if imgMap, err := r.fetchImagesForRatings(ctx, []string{rt.ID}); err == nil {
		rt.Images = imgMap[rt.ID]
	}

	return &rt, nil

}

func (r *RatingRepo) DeleteRating(ctx context.Context, id string) error {

	deleteQuery := `
		UPDATE ratings
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`

	tag, err := r.db.Pool.Exec(ctx, deleteQuery, id)
	if err != nil {
		return fmt.Errorf("failed to delete rating: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrRatingNotFound
	}

	return nil

}

func (r *RatingRepo) CreateFlag(ctx context.Context, flagRating *model.RatingFlag) error {

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updateQuery := `
		UPDATE ratings
		SET is_flagged = true, updated_at = NOW()
		WHERE id = $1 AND is_hidden = false AND deleted_at IS NULL`

	tag, err := tx.Exec(ctx, updateQuery, flagRating.RatingID)
	if err != nil {
		return fmt.Errorf("failed to update flag in rating: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrRatingNotFound
	}

	insertQuery := `
		INSERT INTO rating_flags (
		rating_id, user_id, category, reason) VALUES (
			$1, $2, $3, $4
		)
		RETURNING id, created_at; `

	err = tx.QueryRow(
		ctx,
		insertQuery,
		flagRating.RatingID,
		flagRating.UserID,
		flagRating.Category,
		flagRating.Reason,
	).Scan(&flagRating.ID, &flagRating.CreatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrRatingAlreadyExists
		}
		return fmt.Errorf("failed to insert rating: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil

}

func (r *RatingRepo) GetFlaggedRatings(ctx context.Context, limit, offset int32) ([]*dto.FlaggedRatingItem, int32, error) {
	countQuery := `
		SELECT COUNT(*)
		FROM ratings
		WHERE is_flagged = true AND is_hidden = false AND deleted_at IS NULL
	`
	var total int32
	if err := r.db.Pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count flagged ratings: %w", err)
	}

	if total == 0 {
		return []*dto.FlaggedRatingItem{}, 0, nil
	}

	listQuery := `
		SELECT 
			r.id, r.user_id, r.product_id, r.order_id, r.order_item_id,
			r.rating, r.review, r.is_verified_purchase, r.is_flagged, r.is_hidden,
			r.created_at, r.updated_at,
			f.id, f.user_id, f.rating_id, f.category, f.reason, f.created_at
		FROM ratings r
		INNER JOIN rating_flags f ON r.id = f.rating_id AND f.deleted_at IS NULL
		WHERE r.is_flagged = true AND r.is_hidden = false AND r.deleted_at IS NULL
		ORDER BY r.created_at DESC, f.created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Pool.Query(ctx, listQuery, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query flagged ratings: %w", err)
	}
	defer rows.Close()

	itemMap := make(map[string]*dto.FlaggedRatingItem)
	var items []*dto.FlaggedRatingItem

	for rows.Next() {
		var rt model.Rating
		var flag model.RatingFlag

		if err := rows.Scan(
			&rt.ID, &rt.UserID, &rt.ProductID, &rt.OrderID, &rt.OrderItemID,
			&rt.Rating, &rt.Review, &rt.IsVerifiedPurchase, &rt.IsFlagged, &rt.IsHidden,
			&rt.CreatedAt, &rt.UpdatedAt,
			&flag.ID, &flag.UserID, &flag.RatingID, &flag.Category, &flag.Reason, &flag.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan flagged rating row: %w", err)
		}

		item, exists := itemMap[rt.ID]
		if !exists {
			item = &dto.FlaggedRatingItem{
				Rating:    &rt,
				FlagCount: 0,
				Flags:     []*model.RatingFlag{},
			}
			itemMap[rt.ID] = item
			items = append(items, item)
		}

		item.Flags = append(item.Flags, &flag)
		item.FlagCount++
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error during flagged rating rows iteration: %w", err)
	}

	ratingIDs := make([]string, len(items))
	for i, it := range items {
		ratingIDs[i] = it.Rating.ID
	}
	if imgMap, err := r.fetchImagesForRatings(ctx, ratingIDs); err == nil {
		for _, it := range items {
			it.Rating.Images = imgMap[it.Rating.ID]
		}
	}

	return items, total, nil
}

func (r *RatingRepo) ModerateRatings(ctx context.Context, isHidden bool, ratingID string) error {

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updateRatingQuery := `
		UPDATE ratings
		SET is_hidden = $1,
		    is_flagged = CASE WHEN $1 = false THEN false ELSE is_flagged END,
		    updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL`

	tag, err := tx.Exec(ctx, updateRatingQuery, isHidden, ratingID)
	if err != nil {
		return fmt.Errorf("failed to update hidden status: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrRatingNotFound
	}

	deleteFlagQuery := `
		UPDATE rating_flags
		SET deleted_at = NOW()
		WHERE rating_id = $1 AND deleted_at IS NULL
	`

	if _, err := tx.Exec(ctx, deleteFlagQuery, ratingID); err != nil {
		return fmt.Errorf("failed to soft delete rating flags: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (r *RatingRepo) fetchImagesForRatings(ctx context.Context, ratingIDs []string) (map[string][]model.RatingImage, error) {
	result := make(map[string][]model.RatingImage)
	if len(ratingIDs) == 0 {
		return result, nil
	}

	query := `
		SELECT id, rating_id, url, created_at 
		FROM rating_images 
		WHERE rating_id = ANY($1::uuid[]) AND deleted_at IS NULL 
		ORDER BY created_at ASC
	`

	rows, err := r.db.Pool.Query(ctx, query, ratingIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to query rating images: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var img model.RatingImage
		if err := rows.Scan(&img.ID, &img.RatingID, &img.URL, &img.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan rating image: %w", err)
		}
		result[img.RatingID] = append(result[img.RatingID], img)
	}
	return result, rows.Err()
}

func (r *RatingRepo) AddRatingImages(ctx context.Context, ratingID string, urls []string) ([]*model.RatingImage, error) {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	insertQuery := `
		INSERT INTO rating_images (rating_id,url)
		VALUES ($1, $2)
		RETURNING id,rating_id,url,created_at
	`

	created := make([]*model.RatingImage, 0, len(urls))
	for _, u := range urls {
		var img model.RatingImage
		if err := tx.QueryRow(ctx, insertQuery, ratingID, u).Scan(&img.ID, &img.RatingID, &img.URL, &img.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to insert rating image: %w", err)
		}
		created = append(created, &img)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return created, nil
}

func (r *RatingRepo) GetRatingImageByID(ctx context.Context, imageID string) (*model.RatingImage, error) {
	query := `
		SELECT id, rating_id, url, created_at 
		FROM rating_images
		WHERE id = $1 AND deleted_at IS NULL	
	`

	var img model.RatingImage

	err := r.db.Pool.QueryRow(ctx, query, imageID).Scan(&img.ID, &img.RatingID, &img.URL, &img.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRatingImageNotFound
		}
		return nil, fmt.Errorf("failed to query rating image: %w", err)
	}

	return &img, nil
}

func (r *RatingRepo) DeleteRatingImage(ctx context.Context, imageID string) error {
	query := `
		UPDATE rating_images
		SET deleted_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`

	tag, err := r.db.Pool.Exec(ctx, query, imageID)
	if err != nil {
		return fmt.Errorf("failed to delete rating image: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRatingImageNotFound
	}
	return nil
}
