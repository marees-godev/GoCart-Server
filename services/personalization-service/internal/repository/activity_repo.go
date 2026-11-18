package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/services/personalization-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/personalization-service/internal/model"
)

type ActivityRepository interface {
	IsEventProcessed(ctx context.Context, eventID string) (bool, error)
	SaveActivity(ctx context.Context, eventID string, act *model.CustomerActivity) error
	GetCustomerActivities(ctx context.Context, userID, sessionID string, limit, offset int) ([]*model.CustomerActivity, int, error)
	GetRecentlyViewedProducts(ctx context.Context, userID, sessionID string, limit, offset int) ([]*dto.RecentlyViewedProduct, int, error)
	GetCategoryPreference(ctx context.Context, userID, sessionID string, limit, offset int) ([]*dto.CustomerPreference, error)
}

type activityRepo struct {
	pool *pgxpool.Pool
}

func NewActivityRepository(pool *pgxpool.Pool) ActivityRepository {
	return &activityRepo{pool: pool}
}

func (r *activityRepo) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM outbox_events WHERE event_id = $1)`
	err := r.pool.QueryRow(ctx, query, eventID).Scan(&exists)
	return exists, err
}

func (r *activityRepo) SaveActivity(ctx context.Context, eventID string, act *model.CustomerActivity) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `INSERT INTO outbox_events (event_id, event_type) VALUES ($1, $2)`, eventID, act.EventType)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO customer_activities (user_id, session_id, event_type, product_id, category_id)
		VALUES ($1, $2, $3::activity_event_type, $4, $5)
		RETURNING id, created_at
	`

	err = tx.QueryRow(ctx, query, act.UserID, act.SessionID, act.EventType, act.ProductID, act.CategoryID).Scan(&act.ID, &act.CreatedAt)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *activityRepo) GetCustomerActivities(ctx context.Context, userID, sessionID string, limit, offset int) ([]*model.CustomerActivity, int, error) {
	var countQuery string
	var selectQuery string
	var filterArg string

	if userID != "" {
		filterArg = userID
		countQuery = `SELECT COUNT(*) FROM customer_activities WHERE user_id = $1`
		selectQuery = `
			SELECT id, user_id, session_id, event_type, product_id, category_id, created_at
			FROM customer_activities
			WHERE user_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`
	} else {
		filterArg = sessionID
		countQuery = `SELECT COUNT(*) FROM customer_activities WHERE session_id = $1`
		selectQuery = `
			SELECT id, user_id, session_id, event_type, product_id, category_id, created_at
			FROM customer_activities
			WHERE session_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`
	}

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, filterArg).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, selectQuery, filterArg, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var activities []*model.CustomerActivity
	for rows.Next() {
		var act model.CustomerActivity
		if err := rows.Scan(&act.ID, &act.UserID, &act.SessionID, &act.EventType, &act.ProductID, &act.CategoryID, &act.CreatedAt); err != nil {
			return nil, 0, err
		}
		activities = append(activities, &act)
	}
	return activities, total, rows.Err()
}

func (r *activityRepo) GetRecentlyViewedProducts(ctx context.Context, userID, sessionID string, limit, offset int) ([]*dto.RecentlyViewedProduct, int, error) {
	var countQuery string
	var selectQuery string
	var filterArg string

	if userID != "" {
		filterArg = userID
		countQuery = `
			SELECT COUNT(DISTINCT product_id)
			FROM customer_activities
			WHERE user_id = $1 AND event_type = 'PRODUCT_VIEWED' AND product_id IS NOT NULL`
		selectQuery = `
			SELECT product_id, MAX(created_at) AS last_viewed_at
			FROM customer_activities
			WHERE user_id = $1 AND event_type = 'PRODUCT_VIEWED' AND product_id IS NOT NULL
			GROUP BY product_id
			ORDER BY last_viewed_at DESC
			LIMIT $2 OFFSET $3`
	} else {
		filterArg = sessionID
		countQuery = `
			SELECT COUNT(DISTINCT product_id)
			FROM customer_activities
			WHERE session_id = $1 AND event_type = 'PRODUCT_VIEWED' AND product_id IS NOT NULL`
		selectQuery = `
			SELECT product_id, MAX(created_at) AS last_viewed_at
			FROM customer_activities
			WHERE session_id = $1 AND event_type = 'PRODUCT_VIEWED' AND product_id IS NOT NULL
			GROUP BY product_id
			ORDER BY last_viewed_at DESC
			LIMIT $2 OFFSET $3`
	}

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, filterArg).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx, selectQuery, filterArg, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var products []*dto.RecentlyViewedProduct
	for rows.Next() {
		var item dto.RecentlyViewedProduct
		if err := rows.Scan(&item.ProductID, &item.LastViewedAt); err != nil {
			return nil, 0, err
		}
		products = append(products, &item)
	}

	return products, total, rows.Err()
}

func (r *activityRepo) GetCategoryPreference(ctx context.Context, userID, sessionID string, limit, offset int) ([]*dto.CustomerPreference, error) {
	var query string
	var filterArg string

	if userID != "" {
		filterArg = userID
		query = `
			SELECT 
				category_id,
				ROUND(
					SUM(
						CASE event_type
							WHEN 'PRODUCT_VIEWED'          THEN 1.0
							WHEN 'CATEGORY_VIEWED'         THEN 2.0
							WHEN 'PRODUCT_SEARCHED'        THEN 1.0
							WHEN 'PRODUCT_WISHLISTED'      THEN 4.0
							WHEN 'PRODUCT_ADDED_TO_CART'   THEN 5.0
							WHEN 'PRODUCT_PURCHASED'       THEN 8.0
							ELSE 1.0
						END 
						* EXP(-0.05 * (EXTRACT(EPOCH FROM (NOW() - created_at)) / 86400.0))
					)::numeric, 
					2
				) AS score
			FROM customer_activities
			WHERE user_id = $1 AND category_id IS NOT NULL
			GROUP BY category_id
			ORDER BY score DESC
			LIMIT $2 OFFSET $3`
	} else {
		filterArg = sessionID
		query = `
			SELECT 
				category_id,
				ROUND(
					SUM(
						CASE event_type
							WHEN 'PRODUCT_VIEWED'          THEN 1.0
							WHEN 'CATEGORY_VIEWED'         THEN 2.0
							WHEN 'PRODUCT_SEARCHED'        THEN 1.0
							WHEN 'PRODUCT_WISHLISTED'      THEN 4.0
							WHEN 'PRODUCT_ADDED_TO_CART'   THEN 5.0
							WHEN 'PRODUCT_PURCHASED'       THEN 8.0
							ELSE 1.0
						END 
						* EXP(-0.05 * (EXTRACT(EPOCH FROM (NOW() - created_at)) / 86400.0))
					)::numeric, 
					2
				) AS score
			FROM customer_activities
			WHERE session_id = $1 AND category_id IS NOT NULL
			GROUP BY category_id
			ORDER BY score DESC
			LIMIT $2 OFFSET $3`
	}

	rows, err := r.pool.Query(ctx, query, filterArg, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var preferences []*dto.CustomerPreference
	for rows.Next() {
		var pref dto.CustomerPreference
		if err := rows.Scan(&pref.CategoryID, &pref.Score); err != nil {
			return nil, err
		}
		preferences = append(preferences, &pref)
	}

	return preferences, rows.Err()
}