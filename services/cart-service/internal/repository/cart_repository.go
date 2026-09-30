package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/pkg/redis"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/model"
)

var (
	ErrCartNotFound = errors.New("cart not found")
	ErrItemNotFound = errors.New("item not found in cart")
)

type CartRepository interface {
	GetCartByUserID(ctx context.Context, userID string) (*model.Cart, error)
	CreateCart(ctx context.Context, userID string) (*model.Cart, error)
	AddOrUpdateItem(ctx context.Context, cartID string, item *model.CartItem) (*model.Cart, error)
	UpdateItemQuantity(ctx context.Context, cartID string, productID, variantID string, quantity int32) (*model.Cart, error)
	RemoveItem(ctx context.Context, cartID string, productID, variantID string) (*model.Cart, error)
	ClearCart(ctx context.Context, cartID string) error

	GetCartFromCache(ctx context.Context, userID string) (*model.Cart, error)
	SetCartInCache(ctx context.Context, cart *model.Cart, ttl time.Duration) error
	DeleteCartFromCache(ctx context.Context, userID string) error
}

type pgCartRepository struct {
	db          *pgxpool.Pool
	redisClient *redis.Client
	log         *slog.Logger
}

func NewCartRepository(db *pgxpool.Pool, redisClient *redis.Client, log *slog.Logger) CartRepository {
	return &pgCartRepository{
		db:          db,
		redisClient: redisClient,
		log:         log,
	}
}

func (r *pgCartRepository) GetCartByUserID(ctx context.Context, userID string) (*model.Cart, error) {
	if r.db == nil {
		return nil, errors.New("database pool is not initialized")
	}

	cart := &model.Cart{
		UserID: userID,
		Items:  make([]model.CartItem, 0),
	}

	query := `SELECT id, user_id, created_at, updated_at FROM carts WHERE user_id = $1`
	err := r.db.QueryRow(ctx, query, userID).Scan(&cart.ID, &cart.UserID, &cart.CreatedAt, &cart.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCartNotFound
		}
		return nil, fmt.Errorf("failed to query cart: %w", err)
	}

	itemsQuery := `
		SELECT id, cart_id, product_id, COALESCE(variant_id::text, ''), COALESCE(store_id::text, ''), quantity, unit_price, created_at, updated_at
		FROM cart_items
		WHERE cart_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.db.Query(ctx, itemsQuery, cart.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to query cart items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item model.CartItem
		err := rows.Scan(
			&item.ID,
			&item.CartID,
			&item.ProductID,
			&item.VariantID,
			&item.StoreID,
			&item.Quantity,
			&item.UnitPrice,
			&item.CreatedAt,
			&item.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan cart item: %w", err)
		}
		cart.Items = append(cart.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating cart items: %w", err)
	}

	cart.CalculateTotal()
	return cart, nil
}

func (r *pgCartRepository) CreateCart(ctx context.Context, userID string) (*model.Cart, error) {
	if r.db == nil {
		return nil, errors.New("database pool is not initialized")
	}

	cart := &model.Cart{
		UserID: userID,
		Items:  make([]model.CartItem, 0),
	}

	query := `
		INSERT INTO carts (user_id, created_at, updated_at)
		VALUES ($1, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET updated_at = NOW()
		RETURNING id, user_id, created_at, updated_at
	`
	err := r.db.QueryRow(ctx, query, userID).Scan(&cart.ID, &cart.UserID, &cart.CreatedAt, &cart.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create cart: %w", err)
	}

	cart.CalculateTotal()
	return cart, nil
}

func (r *pgCartRepository) AddOrUpdateItem(ctx context.Context, cartID string, item *model.CartItem) (*model.Cart, error) {
	if r.db == nil {
		return nil, errors.New("database pool is not initialized")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var existingID string
	var existingQty int32
	var checkQuery string

	if item.VariantID != "" {
		checkQuery = `SELECT id, quantity FROM cart_items WHERE cart_id = $1 AND product_id = $2 AND variant_id = $3`
		err = tx.QueryRow(ctx, checkQuery, cartID, item.ProductID, item.VariantID).Scan(&existingID, &existingQty)
	} else {
		checkQuery = `SELECT id, quantity FROM cart_items WHERE cart_id = $1 AND product_id = $2 AND (variant_id IS NULL OR variant_id::text = '')`
		err = tx.QueryRow(ctx, checkQuery, cartID, item.ProductID).Scan(&existingID, &existingQty)
	}

	if err == nil {
		newQty := existingQty + item.Quantity
		updateQuery := `UPDATE cart_items SET quantity = $1, unit_price = $2, updated_at = NOW() WHERE id = $3`
		if _, err := tx.Exec(ctx, updateQuery, newQty, item.UnitPrice, existingID); err != nil {
			return nil, fmt.Errorf("failed to update cart item quantity: %w", err)
		}
	} else if errors.Is(err, pgx.ErrNoRows) {
		var insertQuery string
		if item.VariantID != "" && item.StoreID != "" {
			insertQuery = `INSERT INTO cart_items (cart_id, product_id, variant_id, store_id, quantity, unit_price, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`
			_, err = tx.Exec(ctx, insertQuery, cartID, item.ProductID, item.VariantID, item.StoreID, item.Quantity, item.UnitPrice)
		} else if item.VariantID != "" {
			insertQuery = `INSERT INTO cart_items (cart_id, product_id, variant_id, quantity, unit_price, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`
			_, err = tx.Exec(ctx, insertQuery, cartID, item.ProductID, item.VariantID, item.Quantity, item.UnitPrice)
		} else if item.StoreID != "" {
			insertQuery = `INSERT INTO cart_items (cart_id, product_id, store_id, quantity, unit_price, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`
			_, err = tx.Exec(ctx, insertQuery, cartID, item.ProductID, item.StoreID, item.Quantity, item.UnitPrice)
		} else {
			insertQuery = `INSERT INTO cart_items (cart_id, product_id, quantity, unit_price, created_at, updated_at) VALUES ($1, $2, $3, $4, NOW(), NOW())`
			_, err = tx.Exec(ctx, insertQuery, cartID, item.ProductID, item.Quantity, item.UnitPrice)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to insert cart item: %w", err)
		}
	} else {
		return nil, fmt.Errorf("failed to check existing cart item: %w", err)
	}

	_, _ = tx.Exec(ctx, `UPDATE carts SET updated_at = NOW() WHERE id = $1`, cartID)

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	var userID string
	_ = r.db.QueryRow(ctx, `SELECT user_id FROM carts WHERE id = $1`, cartID).Scan(&userID)
	return r.GetCartByUserID(ctx, userID)
}

func (r *pgCartRepository) UpdateItemQuantity(ctx context.Context, cartID string, productID, variantID string, quantity int32) (*model.Cart, error) {
	if r.db == nil {
		return nil, errors.New("database pool is not initialized")
	}

	var updateQuery string
	var tag pgx.Row
	_ = tag

	var commandTag any
	var err error
	if variantID != "" {
		updateQuery = `UPDATE cart_items SET quantity = $1, updated_at = NOW() WHERE cart_id = $2 AND product_id = $3 AND variant_id = $4`
		res, execErr := r.db.Exec(ctx, updateQuery, quantity, cartID, productID, variantID)
		commandTag = res
		err = execErr
	} else {
		updateQuery = `UPDATE cart_items SET quantity = $1, updated_at = NOW() WHERE cart_id = $2 AND product_id = $3 AND (variant_id IS NULL OR variant_id::text = '')`
		res, execErr := r.db.Exec(ctx, updateQuery, quantity, cartID, productID)
		commandTag = res
		err = execErr
	}

	if err != nil {
		return nil, fmt.Errorf("failed to update item quantity: %w", err)
	}

	if tagRes, ok := commandTag.(interface{ RowsAffected() int64 }); ok && tagRes.RowsAffected() == 0 {
		return nil, ErrItemNotFound
	}

	_, _ = r.db.Exec(ctx, `UPDATE carts SET updated_at = NOW() WHERE id = $1`, cartID)

	var userID string
	_ = r.db.QueryRow(ctx, `SELECT user_id FROM carts WHERE id = $1`, cartID).Scan(&userID)
	return r.GetCartByUserID(ctx, userID)
}

func (r *pgCartRepository) RemoveItem(ctx context.Context, cartID string, productID, variantID string) (*model.Cart, error) {
	if r.db == nil {
		return nil, errors.New("database pool is not initialized")
	}

	var deleteQuery string
	var commandTag any
	var err error

	if variantID != "" {
		deleteQuery = `DELETE FROM cart_items WHERE cart_id = $1 AND product_id = $2 AND variant_id = $3`
		res, execErr := r.db.Exec(ctx, deleteQuery, cartID, productID, variantID)
		commandTag = res
		err = execErr
	} else {
		deleteQuery = `DELETE FROM cart_items WHERE cart_id = $1 AND product_id = $2 AND (variant_id IS NULL OR variant_id::text = '')`
		res, execErr := r.db.Exec(ctx, deleteQuery, cartID, productID)
		commandTag = res
		err = execErr
	}

	if err != nil {
		return nil, fmt.Errorf("failed to remove cart item: %w", err)
	}

	if tagRes, ok := commandTag.(interface{ RowsAffected() int64 }); ok && tagRes.RowsAffected() == 0 {
		return nil, ErrItemNotFound
	}

	_, _ = r.db.Exec(ctx, `UPDATE carts SET updated_at = NOW() WHERE id = $1`, cartID)

	var userID string
	_ = r.db.QueryRow(ctx, `SELECT user_id FROM carts WHERE id = $1`, cartID).Scan(&userID)
	return r.GetCartByUserID(ctx, userID)
}

func (r *pgCartRepository) ClearCart(ctx context.Context, cartID string) error {
	if r.db == nil {
		return errors.New("database pool is not initialized")
	}

	deleteQuery := `DELETE FROM cart_items WHERE cart_id = $1`
	if _, err := r.db.Exec(ctx, deleteQuery, cartID); err != nil {
		return fmt.Errorf("failed to clear cart items: %w", err)
	}

	_, _ = r.db.Exec(ctx, `UPDATE carts SET updated_at = NOW() WHERE id = $1`, cartID)
	return nil
}

func (r *pgCartRepository) GetCartFromCache(ctx context.Context, userID string) (*model.Cart, error) {
	if r.redisClient == nil {
		return nil, nil
	}

	cacheKey := fmt.Sprintf("cart:%s", userID)
	var cart model.Cart
	err := r.redisClient.GetJSON(ctx, cacheKey, &cart)
	if err != nil {
		if errors.Is(err, redis.ErrKeyNotFound) {
			return nil, nil
		}
		if r.log != nil {
			r.log.Warn("Failed to fetch cart from Redis cache", "user_id", userID, "error", err)
		}
		return nil, nil
	}
	return &cart, nil
}

func (r *pgCartRepository) SetCartInCache(ctx context.Context, cart *model.Cart, ttl time.Duration) error {
	if r.redisClient == nil || cart == nil {
		return nil
	}

	cacheKey := fmt.Sprintf("cart:%s", cart.UserID)
	cart.UpdatedAt = time.Now()
	err := r.redisClient.SetJSON(ctx, cacheKey, cart, ttl)
	if err != nil && r.log != nil {
		r.log.Warn("Failed to set cart in Redis cache", "user_id", cart.UserID, "error", err)
	}
	return nil
}

func (r *pgCartRepository) DeleteCartFromCache(ctx context.Context, userID string) error {
	if r.redisClient == nil {
		return nil
	}

	cacheKey := fmt.Sprintf("cart:%s", userID)
	err := r.redisClient.Delete(ctx, cacheKey)
	if err != nil && r.log != nil {
		r.log.Warn("Failed to delete cart from Redis cache", "user_id", userID, "error", err)
	}
	return nil
}
