package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/model"
)

type InventoryRepository interface {
	Create(ctx context.Context, inv *model.Inventory, initialQuantity int, notes string) error
	GetByID(ctx context.Context, id string) (*model.Inventory, error)
	GetByProductAndVariant(ctx context.Context, productID string, variantID *string) (*model.Inventory, error)
	Restock(ctx context.Context, id string, quantity int, refID, notes *string) (*model.Inventory, error)
	UpdateStock(ctx context.Context, productID string, variantID *string, quantity int) (*model.Inventory, error)
	ReserveStock(ctx context.Context, orderID string, items []model.ReserveItem, expiresAt time.Time) (string, error)
	ReleaseStock(ctx context.Context, reservationID string) error
	GetTransactionsByInventoryID(ctx context.Context, inventoryID string, limit, offset int) ([]*model.InventoryTransaction, error)
}

type pgInventoryRepository struct {
	pool *pgxpool.Pool
}

func NewInventoryRepository(pool *pgxpool.Pool) InventoryRepository {
	return &pgInventoryRepository{
		pool: pool,
	}
}

func (r *pgInventoryRepository) Create(ctx context.Context, inv *model.Inventory, initialQuantity int, notes string) error {
	if inv == nil {
		return appErrors.BadRequest("inventory data cannot be nil")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return appErrors.Internal(err, "failed to start database transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if inv.ID == "" {
		inv.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	inv.CreatedAt = now
	inv.UpdatedAt = now

	insertQuery := `
		INSERT INTO inventories (
			id, product_id, variant_id, sku, available_quantity, reserved_quantity, low_stock_threshold, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		) RETURNING id, product_id, variant_id, sku, available_quantity, reserved_quantity, low_stock_threshold, created_at, updated_at;
	`

	err = tx.QueryRow(
		ctx,
		insertQuery,
		inv.ID,
		inv.ProductID,
		inv.VariantID,
		inv.SKU,
		inv.AvailableQuantity,
		inv.ReservedQuantity,
		inv.LowStockThreshold,
		inv.CreatedAt,
		inv.UpdatedAt,
	).Scan(
		&inv.ID,
		&inv.ProductID,
		&inv.VariantID,
		&inv.SKU,
		&inv.AvailableQuantity,
		&inv.ReservedQuantity,
		&inv.LowStockThreshold,
		&inv.CreatedAt,
		&inv.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if strings.Contains(pgErr.ConstraintName, "sku") {
				return appErrors.Conflict("inventory with this SKU already exists")
			}
			return appErrors.Conflict("inventory already exists for this product/variant")
		}
		return appErrors.Internal(err, "failed to insert inventory")
	}

	if initialQuantity > 0 {
		txInsertQuery := `
			INSERT INTO inventory_transactions (
				id, inventory_id, type, quantity, reference_id, notes, created_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7
			);
		`
		var refID *string
		notesVal := notes
		if notesVal == "" {
			notesVal = "Initial stock creation"
		}
		_, err = tx.Exec(
			ctx,
			txInsertQuery,
			uuid.NewString(),
			inv.ID,
			model.TransactionTypeRestock,
			initialQuantity,
			refID,
			&notesVal,
			now,
		)
		if err != nil {
			return appErrors.Internal(err, "failed to record initial inventory transaction")
		}
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"inventory_id":        inv.ID,
		"product_id":          inv.ProductID,
		"variant_id":          inv.VariantID,
		"sku":                 inv.SKU,
		"available_quantity":  inv.AvailableQuantity,
		"reserved_quantity":   inv.ReservedQuantity,
		"low_stock_threshold": inv.LowStockThreshold,
	})

	outboxQuery := `
		INSERT INTO outbox_events (
			id, aggregate_type, aggregate_id, event_type, payload, status, retry_count, created_at
		) VALUES (
			$1, $2, $3, $4, $5, 'PENDING', 0, $6
		);
	`
	_, err = tx.Exec(ctx, outboxQuery, uuid.NewString(), "INVENTORY", inv.ID, "INVENTORY_CREATED", payload, now)
	if err != nil {
		return appErrors.Internal(err, "failed to write outbox event")
	}

	if err := tx.Commit(ctx); err != nil {
		return appErrors.Internal(err, "failed to commit inventory creation transaction")
	}

	return nil
}

func (r *pgInventoryRepository) GetByID(ctx context.Context, id string) (*model.Inventory, error) {
	query := `
		SELECT id, product_id, variant_id, sku, available_quantity, reserved_quantity, low_stock_threshold, created_at, updated_at
		FROM inventories
		WHERE id = $1;
	`
	var inv model.Inventory
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&inv.ID,
		&inv.ProductID,
		&inv.VariantID,
		&inv.SKU,
		&inv.AvailableQuantity,
		&inv.ReservedQuantity,
		&inv.LowStockThreshold,
		&inv.CreatedAt,
		&inv.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("inventory not found")
		}
		return nil, appErrors.Internal(err, "failed to get inventory by id")
	}
	return &inv, nil
}

func (r *pgInventoryRepository) GetByProductAndVariant(ctx context.Context, productID string, variantID *string) (*model.Inventory, error) {
	var query string
	var args []interface{}

	if variantID == nil || *variantID == "" {
		query = `
			SELECT id, product_id, variant_id, sku, available_quantity, reserved_quantity, low_stock_threshold, created_at, updated_at
			FROM inventories
			WHERE product_id = $1 AND variant_id IS NULL;
		`
		args = []interface{}{productID}
	} else {
		query = `
			SELECT id, product_id, variant_id, sku, available_quantity, reserved_quantity, low_stock_threshold, created_at, updated_at
			FROM inventories
			WHERE product_id = $1 AND variant_id = $2;
		`
		args = []interface{}{productID, *variantID}
	}

	var inv model.Inventory
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&inv.ID,
		&inv.ProductID,
		&inv.VariantID,
		&inv.SKU,
		&inv.AvailableQuantity,
		&inv.ReservedQuantity,
		&inv.LowStockThreshold,
		&inv.CreatedAt,
		&inv.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("inventory not found for product/variant")
		}
		return nil, appErrors.Internal(err, "failed to get inventory by product and variant")
	}
	return &inv, nil
}

func (r *pgInventoryRepository) Restock(ctx context.Context, id string, quantity int, refID, notes *string) (*model.Inventory, error) {
	if quantity <= 0 {
		return nil, appErrors.BadRequest("restock quantity must be greater than zero")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to start restock transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	updateQuery := `
		UPDATE inventories
		SET available_quantity = available_quantity + $1,
		    updated_at = $2
		WHERE id = $3
		RETURNING id, product_id, variant_id, sku, available_quantity, reserved_quantity, low_stock_threshold, created_at, updated_at;
	`

	var inv model.Inventory
	err = tx.QueryRow(ctx, updateQuery, quantity, now, id).Scan(
		&inv.ID,
		&inv.ProductID,
		&inv.VariantID,
		&inv.SKU,
		&inv.AvailableQuantity,
		&inv.ReservedQuantity,
		&inv.LowStockThreshold,
		&inv.CreatedAt,
		&inv.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("inventory not found for restocking")
		}
		return nil, appErrors.Internal(err, "failed to update inventory available quantity")
	}

	txInsertQuery := `
		INSERT INTO inventory_transactions (
			id, inventory_id, type, quantity, reference_id, notes, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		);
	`
	_, err = tx.Exec(
		ctx,
		txInsertQuery,
		uuid.NewString(),
		inv.ID,
		model.TransactionTypeRestock,
		quantity,
		refID,
		notes,
		now,
	)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to record restock inventory transaction")
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"inventory_id":       inv.ID,
		"product_id":         inv.ProductID,
		"variant_id":         inv.VariantID,
		"restock_quantity":   quantity,
		"available_quantity": inv.AvailableQuantity,
		"reserved_quantity":  inv.ReservedQuantity,
	})

	outboxQuery := `
		INSERT INTO outbox_events (
			id, aggregate_type, aggregate_id, event_type, payload, status, retry_count, created_at
		) VALUES (
			$1, $2, $3, $4, $5, 'PENDING', 0, $6
		);
	`
	_, err = tx.Exec(ctx, outboxQuery, uuid.NewString(), "INVENTORY", inv.ID, "INVENTORY_RESTOCKED", payload, now)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to record restock outbox event")
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, appErrors.Internal(err, "failed to commit restock transaction")
	}

	return &inv, nil
}

func (r *pgInventoryRepository) UpdateStock(ctx context.Context, productID string, variantID *string, quantity int) (*model.Inventory, error) {
	if quantity < 0 {
		return nil, appErrors.BadRequest("quantity cannot be negative")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	var updateQuery string
	var args []interface{}

	if variantID == nil || *variantID == "" {
		updateQuery = `
			UPDATE inventories
			SET available_quantity = $1,
			    updated_at = $2
			WHERE product_id = $3 AND variant_id IS NULL
			RETURNING id, product_id, variant_id, sku, available_quantity, reserved_quantity, low_stock_threshold, created_at, updated_at;
		`
		args = []interface{}{quantity, now, productID}
	} else {
		updateQuery = `
			UPDATE inventories
			SET available_quantity = $1,
			    updated_at = $2
			WHERE product_id = $3 AND variant_id = $4
			RETURNING id, product_id, variant_id, sku, available_quantity, reserved_quantity, low_stock_threshold, created_at, updated_at;
		`
		args = []interface{}{quantity, now, productID, *variantID}
	}

	var inv model.Inventory
	err = tx.QueryRow(ctx, updateQuery, args...).Scan(
		&inv.ID,
		&inv.ProductID,
		&inv.VariantID,
		&inv.SKU,
		&inv.AvailableQuantity,
		&inv.ReservedQuantity,
		&inv.LowStockThreshold,
		&inv.CreatedAt,
		&inv.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("inventory not found for update")
		}
		return nil, appErrors.Internal(err, "failed to update stock")
	}

	txNotes := fmt.Sprintf("Stock manually updated to %d", quantity)
	txInsertQuery := `
		INSERT INTO inventory_transactions (
			id, inventory_id, type, quantity, reference_id, notes, created_at
		) VALUES (
			$1, $2, $3, $4, NULL, $5, $6
		);
	`
	_, err = tx.Exec(
		ctx,
		txInsertQuery,
		uuid.NewString(),
		inv.ID,
		model.TransactionTypeAdjustment,
		quantity,
		&txNotes,
		now,
	)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to record stock adjustment transaction")
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, appErrors.Internal(err, "failed to commit stock update")
	}

	return &inv, nil
}

func (r *pgInventoryRepository) ReserveStock(ctx context.Context, orderID string, items []model.ReserveItem, expiresAt time.Time) (string, error) {
	if len(items) == 0 {
		return "", appErrors.BadRequest("at least one item required for reservation")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", appErrors.Internal(err, "failed to begin reserve transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	reservationID := uuid.NewString()

	for _, item := range items {
		if item.Quantity <= 0 {
			return "", appErrors.BadRequest(fmt.Sprintf("invalid quantity %d for product %s", item.Quantity, item.ProductID))
		}

		var selectQuery string
		var args []interface{}
		if item.VariantID == nil || *item.VariantID == "" {
			selectQuery = `
				SELECT id, available_quantity, reserved_quantity
				FROM inventories
				WHERE product_id = $1 AND variant_id IS NULL
				FOR UPDATE;
			`
			args = []interface{}{item.ProductID}
		} else {
			selectQuery = `
				SELECT id, available_quantity, reserved_quantity
				FROM inventories
				WHERE product_id = $1 AND variant_id = $2
				FOR UPDATE;
			`
			args = []interface{}{item.ProductID, *item.VariantID}
		}

		var invID string
		var available, reserved int
		err := tx.QueryRow(ctx, selectQuery, args...).Scan(&invID, &available, &reserved)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", appErrors.NotFound(fmt.Sprintf("inventory not found for product %s", item.ProductID))
			}
			return "", appErrors.Internal(err, "failed to lock inventory row for reservation")
		}

		if available < item.Quantity {
			return "", appErrors.Conflict(fmt.Sprintf("insufficient stock for product %s: available %d, requested %d", item.ProductID, available, item.Quantity))
		}

		updateQuery := `
			UPDATE inventories
			SET available_quantity = available_quantity - $1,
			    reserved_quantity = reserved_quantity + $1,
			    updated_at = $2
			WHERE id = $3;
		`
		_, err = tx.Exec(ctx, updateQuery, item.Quantity, now, invID)
		if err != nil {
			return "", appErrors.Internal(err, "failed to update stock for reservation")
		}

		resQuery := `
			INSERT INTO inventory_reservations (
				id, order_id, product_id, variant_id, quantity, status, expires_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, 'RESERVED', $6, $7, $8
			);
		`
		_, err = tx.Exec(ctx, resQuery, reservationID, orderID, item.ProductID, item.VariantID, item.Quantity, expiresAt, now, now)
		if err != nil {
			return "", appErrors.Internal(err, "failed to insert reservation record")
		}

		ref := orderID
		notes := fmt.Sprintf("Stock reserved for order %s", orderID)
		txQuery := `
			INSERT INTO inventory_transactions (
				id, inventory_id, type, quantity, reference_id, notes, created_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7
			);
		`
		_, err = tx.Exec(ctx, txQuery, uuid.NewString(), invID, model.TransactionTypeReserve, item.Quantity, &ref, &notes, now)
		if err != nil {
			return "", appErrors.Internal(err, "failed to insert reservation transaction")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", appErrors.Internal(err, "failed to commit stock reservation")
	}

	return reservationID, nil
}

func (r *pgInventoryRepository) ReleaseStock(ctx context.Context, reservationID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return appErrors.Internal(err, "failed to begin release transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	rows, err := tx.Query(ctx, `
		SELECT id, order_id, product_id, variant_id, quantity
		FROM inventory_reservations
		WHERE id = $1 AND status = 'RESERVED'
		FOR UPDATE;
	`, reservationID)
	if err != nil {
		return appErrors.Internal(err, "failed to fetch reservations for release")
	}
	defer rows.Close()

	type resRow struct {
		id, orderID, productID string
		variantID              *string
		quantity               int
	}
	var resList []resRow
	for rows.Next() {
		var row resRow
		if err := rows.Scan(&row.id, &row.orderID, &row.productID, &row.variantID, &row.quantity); err != nil {
			return appErrors.Internal(err, "failed to scan reservation row")
		}
		resList = append(resList, row)
	}
	rows.Close()

	if len(resList) == 0 {
		return appErrors.NotFound("reservation not found or already released/expired")
	}

	for _, row := range resList {
		var updateInvQuery string
		var args []interface{}
		if row.variantID == nil || *row.variantID == "" {
			updateInvQuery = `
				UPDATE inventories
				SET available_quantity = available_quantity + $1,
				    reserved_quantity = GREATEST(reserved_quantity - $1, 0),
				    updated_at = $2
				WHERE product_id = $3 AND variant_id IS NULL
				RETURNING id;
			`
			args = []interface{}{row.quantity, now, row.productID}
		} else {
			updateInvQuery = `
				UPDATE inventories
				SET available_quantity = available_quantity + $1,
				    reserved_quantity = GREATEST(reserved_quantity - $1, 0),
				    updated_at = $2
				WHERE product_id = $3 AND variant_id = $4
				RETURNING id;
			`
			args = []interface{}{row.quantity, now, row.productID, *row.variantID}
		}

		var invID string
		err := tx.QueryRow(ctx, updateInvQuery, args...).Scan(&invID)
		if err != nil {
			return appErrors.Internal(err, "failed to return stock to available inventory")
		}

		ref := row.orderID
		notes := fmt.Sprintf("Stock released from reservation %s", reservationID)
		txQuery := `
			INSERT INTO inventory_transactions (
				id, inventory_id, type, quantity, reference_id, notes, created_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7
			);
		`
		_, err = tx.Exec(ctx, txQuery, uuid.NewString(), invID, model.TransactionTypeRelease, row.quantity, &ref, &notes, now)
		if err != nil {
			return appErrors.Internal(err, "failed to record release transaction")
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE inventory_reservations
		SET status = 'RELEASED', updated_at = $1
		WHERE id = $2;
	`, now, reservationID)
	if err != nil {
		return appErrors.Internal(err, "failed to update reservation status to RELEASED")
	}

	if err := tx.Commit(ctx); err != nil {
		return appErrors.Internal(err, "failed to commit release transaction")
	}

	return nil
}

func (r *pgInventoryRepository) GetTransactionsByInventoryID(ctx context.Context, inventoryID string, limit, offset int) ([]*model.InventoryTransaction, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, inventory_id, type, quantity, reference_id, notes, created_at
		FROM inventory_transactions
		WHERE inventory_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3;
	`

	rows, err := r.pool.Query(ctx, query, inventoryID, limit, offset)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to query inventory transactions")
	}
	defer rows.Close()

	var result []*model.InventoryTransaction
	for rows.Next() {
		var tx model.InventoryTransaction
		err := rows.Scan(
			&tx.ID,
			&tx.InventoryID,
			&tx.Type,
			&tx.Quantity,
			&tx.ReferenceID,
			&tx.Notes,
			&tx.CreatedAt,
		)
		if err != nil {
			return nil, appErrors.Internal(err, "failed to scan inventory transaction")
		}
		result = append(result, &tx)
	}

	return result, nil
}
