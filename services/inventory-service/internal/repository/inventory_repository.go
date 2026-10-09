package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	contractsEvents "github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/events"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/model"
)

type InventoryRepository interface {
	Create(ctx context.Context, inv *model.Inventory, initialQuantity int, notes string) error
	GetByID(ctx context.Context, id string) (*model.Inventory, error)
	GetByProductAndVariant(ctx context.Context, productID string, variantID *string) (*model.Inventory, error)
	Restock(ctx context.Context, id string, quantity int, refID, notes *string) (*model.Inventory, error)
	UpdateStock(ctx context.Context, input dto.UpdateStockInput) (*model.Inventory, error)
	ReserveStock(ctx context.Context, orderID string, items []model.ReserveItem, expiresAt time.Time) (string, error)
	ReleaseStock(ctx context.Context, input dto.ReleaseStockInput) error
	CommitStock(ctx context.Context, input dto.CommitStockInput) error
	ReleaseExpiredReservations(ctx context.Context) (int, error)
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
			constraintStr := strings.ToLower(pgErr.ConstraintName + " " + pgErr.Message + " " + pgErr.Detail)
			if strings.Contains(constraintStr, "sku") {
				return appErrors.Conflict("inventory with this SKU already exists")
			}
			return appErrors.Conflict("inventory already exists for this product/variant")
		}
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "23505") || strings.Contains(errStr, "duplicate key") {
			if strings.Contains(errStr, "sku") {
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

	createdEvent := events.InventoryCreatedEvent{
		EventType:         events.EventInventoryCreated,
		InventoryID:       inv.ID,
		ProductID:         inv.ProductID,
		VariantID:         inv.VariantID,
		SKU:               inv.SKU,
		AvailableQuantity: inv.AvailableQuantity,
		ReservedQuantity:  inv.ReservedQuantity,
		LowStockThreshold: inv.LowStockThreshold,
		Timestamp:         now,
	}
	if err := writeOutboxEvent(ctx, tx, "INVENTORY", inv.ID, events.EventInventoryCreated, contractsEvents.TopicInventoryCreated, createdEvent, now); err != nil {
		return appErrors.Internal(err, "failed to write outbox event")
	}

	if inv.AvailableQuantity < inv.LowStockThreshold {
		lowEvent := events.InventoryLowEvent{
			EventType:         events.EventInventoryLow,
			InventoryID:       inv.ID,
			ProductID:         inv.ProductID,
			VariantID:         inv.VariantID,
			SKU:               inv.SKU,
			AvailableQuantity: inv.AvailableQuantity,
			ReservedQuantity:  inv.ReservedQuantity,
			LowStockThreshold: inv.LowStockThreshold,
			Timestamp:         now,
		}
		_ = writeOutboxEvent(ctx, tx, "INVENTORY", inv.ID, events.EventInventoryLow, contractsEvents.TopicInventoryLow, lowEvent, now)
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

	restockedEvent := events.InventoryRestockedEvent{
		EventType:         events.EventInventoryRestocked,
		InventoryID:       inv.ID,
		ProductID:         inv.ProductID,
		VariantID:         inv.VariantID,
		RestockQuantity:   quantity,
		AvailableQuantity: inv.AvailableQuantity,
		ReservedQuantity:  inv.ReservedQuantity,
		Timestamp:         now,
	}
	if err := writeOutboxEvent(ctx, tx, "INVENTORY", inv.ID, events.EventInventoryRestocked, contractsEvents.TopicInventoryRestocked, restockedEvent, now); err != nil {
		return nil, appErrors.Internal(err, "failed to record restock outbox event")
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, appErrors.Internal(err, "failed to commit restock transaction")
	}

	return &inv, nil
}

func (r *pgInventoryRepository) UpdateStock(ctx context.Context, input dto.UpdateStockInput) (*model.Inventory, error) {
	if input.Quantity < 0 {
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

	var invID, prodID, varID string
	if input.InventoryID != nil {
		invID = strings.TrimSpace(*input.InventoryID)
	}
	if input.ProductID != nil {
		prodID = strings.TrimSpace(*input.ProductID)
	}
	if input.VariantID != nil {
		varID = strings.TrimSpace(*input.VariantID)
	}

	if invID != "" {
		updateQuery = `
			UPDATE inventories
			SET available_quantity = $1,
			    updated_at = $2
			WHERE id = $3
			RETURNING id, product_id, variant_id, sku, available_quantity, reserved_quantity, low_stock_threshold, created_at, updated_at;
		`
		args = []interface{}{input.Quantity, now, invID}
	} else if prodID != "" {
		if varID != "" {
			updateQuery = `
				UPDATE inventories
				SET available_quantity = $1,
				    updated_at = $2
				WHERE product_id = $3 AND variant_id = $4
				RETURNING id, product_id, variant_id, sku, available_quantity, reserved_quantity, low_stock_threshold, created_at, updated_at;
			`
			args = []interface{}{input.Quantity, now, prodID, varID}
		} else {
			updateQuery = `
				UPDATE inventories
				SET available_quantity = $1,
				    updated_at = $2
				WHERE product_id = $3 AND variant_id IS NULL
				RETURNING id, product_id, variant_id, sku, available_quantity, reserved_quantity, low_stock_threshold, created_at, updated_at;
			`
			args = []interface{}{input.Quantity, now, prodID}
		}
	} else {
		return nil, appErrors.BadRequest("either inventory_id or product_id must be provided for stock update")
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

	txNotes := fmt.Sprintf("Stock manually updated to %d", input.Quantity)
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
		input.Quantity,
		&txNotes,
		now,
	)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to record stock adjustment transaction")
	}

	if inv.AvailableQuantity < inv.LowStockThreshold {
		lowEvent := events.InventoryLowEvent{
			EventType:         events.EventInventoryLow,
			InventoryID:       inv.ID,
			ProductID:         inv.ProductID,
			VariantID:         inv.VariantID,
			SKU:               inv.SKU,
			AvailableQuantity: inv.AvailableQuantity,
			ReservedQuantity:  inv.ReservedQuantity,
			LowStockThreshold: inv.LowStockThreshold,
			Timestamp:         now,
		}
		_ = writeOutboxEvent(ctx, tx, "INVENTORY", inv.ID, events.EventInventoryLow, contractsEvents.TopicInventoryLow, lowEvent, now)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, appErrors.Internal(err, "failed to commit stock update")
	}

	return &inv, nil
}

func writeOutboxEvent(ctx context.Context, tx pgx.Tx, aggregateType, aggregateID, eventType, topic string, payload interface{}, createdAt time.Time) error {
	envelope, err := contractsEvents.NewEventEnvelope(eventType, "inventory-service", payload)
	if err != nil {
		return fmt.Errorf("failed to create event envelope: %w", err)
	}
	envelopeBytes, err := envelope.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal event envelope: %w", err)
	}
	query := `
		INSERT INTO outbox_events (
			id, aggregate_type, aggregate_id, event_type, payload, topic, status, retry_count, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, 'PENDING', 0, $7
		);
	`
	_, err = tx.Exec(ctx, query, uuid.NewString(), aggregateType, aggregateID, eventType, envelopeBytes, topic, createdAt)
	return err
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

	var existingResID string
	checkQuery := `
		SELECT id FROM inventory_reservations
		WHERE order_id = $1 AND status IN ('RESERVED', 'CONFIRMED')
		LIMIT 1;
	`
	_ = tx.QueryRow(ctx, checkQuery, orderID).Scan(&existingResID)
	if existingResID != "" {
		_ = tx.Commit(ctx)
		return existingResID, nil
	}

	reservationID := uuid.NewString()

	for _, item := range items {
		if item.Quantity <= 0 {
			return "", appErrors.BadRequest(fmt.Sprintf("invalid quantity %d for product %s", item.Quantity, item.ProductID))
		}

		var selectQuery string
		var args []interface{}
		if item.VariantID == nil || *item.VariantID == "" {
			selectQuery = `
				SELECT id, sku, available_quantity, reserved_quantity, low_stock_threshold
				FROM inventories
				WHERE product_id = $1 AND variant_id IS NULL
				FOR UPDATE;
			`
			args = []interface{}{item.ProductID}
		} else {
			selectQuery = `
				SELECT id, sku, available_quantity, reserved_quantity, low_stock_threshold
				FROM inventories
				WHERE product_id = $1 AND variant_id = $2
				FOR UPDATE;
			`
			args = []interface{}{item.ProductID, *item.VariantID}
		}

		var invID, sku string
		var available, reserved, lowStockThreshold int
		err := tx.QueryRow(ctx, selectQuery, args...).Scan(&invID, &sku, &available, &reserved, &lowStockThreshold)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", appErrors.NotFound(fmt.Sprintf("inventory not found for product %s", item.ProductID))
			}
			return "", appErrors.Internal(err, "failed to lock inventory row for reservation")
		}

		if available < item.Quantity {
			return "", appErrors.Conflict(fmt.Sprintf("insufficient stock for product %s: available %d, requested %d", item.ProductID, available, item.Quantity))
		}

		newAvailable := available - item.Quantity
		newReserved := reserved + item.Quantity

		updateQuery := `
			UPDATE inventories
			SET available_quantity = $1,
			    reserved_quantity = $2,
			    updated_at = $3
			WHERE id = $4;
		`
		_, err = tx.Exec(ctx, updateQuery, newAvailable, newReserved, now, invID)
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

		resEvent := events.InventoryReservedEvent{
			EventType:         events.EventInventoryReserved,
			InventoryID:       invID,
			ProductID:         item.ProductID,
			VariantID:         item.VariantID,
			SKU:               sku,
			OrderID:           orderID,
			ReservationID:     reservationID,
			Quantity:          item.Quantity,
			AvailableQuantity: newAvailable,
			ReservedQuantity:  newReserved,
			LowStockThreshold: lowStockThreshold,
			Timestamp:         now,
		}
		if err := writeOutboxEvent(ctx, tx, "INVENTORY", invID, events.EventInventoryReserved, contractsEvents.TopicInventoryReserved, resEvent, now); err != nil {
			return "", appErrors.Internal(err, "failed to record InventoryReserved outbox event")
		}

		if newAvailable < lowStockThreshold {
			lowEvent := events.InventoryLowEvent{
				EventType:         events.EventInventoryLow,
				InventoryID:       invID,
				ProductID:         item.ProductID,
				VariantID:         item.VariantID,
				SKU:               sku,
				AvailableQuantity: newAvailable,
				ReservedQuantity:  newReserved,
				LowStockThreshold: lowStockThreshold,
				Timestamp:         now,
			}
			if err := writeOutboxEvent(ctx, tx, "INVENTORY", invID, events.EventInventoryLow, contractsEvents.TopicInventoryLow, lowEvent, now); err != nil {
				return "", appErrors.Internal(err, "failed to record InventoryLow outbox event")
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", appErrors.Internal(err, "failed to commit stock reservation")
	}

	return reservationID, nil
}

func (r *pgInventoryRepository) ReleaseStock(ctx context.Context, input dto.ReleaseStockInput) error {
	if strings.TrimSpace(input.ReservationID) == "" && strings.TrimSpace(input.OrderID) == "" {
		return appErrors.BadRequest("either reservation_id or order_id is required for release")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return appErrors.Internal(err, "failed to begin release transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()

	var selectQuery string
	var args []interface{}

	if strings.TrimSpace(input.ReservationID) != "" {
		selectQuery = `
			SELECT id, order_id, product_id, variant_id, quantity, status
			FROM inventory_reservations
			WHERE id = $1
			FOR UPDATE;
		`
		args = []interface{}{strings.TrimSpace(input.ReservationID)}
	} else {
		selectQuery = `
			SELECT id, order_id, product_id, variant_id, quantity, status
			FROM inventory_reservations
			WHERE order_id = $1
			FOR UPDATE;
		`
		args = []interface{}{strings.TrimSpace(input.OrderID)}
	}

	rows, err := tx.Query(ctx, selectQuery, args...)
	if err != nil {
		return appErrors.Internal(err, "failed to fetch reservations for release")
	}
	defer rows.Close()

	type resRow struct {
		id, orderID, productID string
		variantID              *string
		quantity               int
		status                 model.ReservationStatus
	}
	var allRows []resRow
	var activeRows []resRow

	for rows.Next() {
		var row resRow
		if err := rows.Scan(&row.id, &row.orderID, &row.productID, &row.variantID, &row.quantity, &row.status); err != nil {
			return appErrors.Internal(err, "failed to scan reservation row")
		}
		allRows = append(allRows, row)
		if row.status == model.ReservationStatusReserved {
			activeRows = append(activeRows, row)
		}
	}
	rows.Close()

	if len(allRows) == 0 {
		return appErrors.NotFound("reservation not found")
	}

	if len(activeRows) == 0 {
		return nil
	}

	targetStatus := model.ReservationStatusReleased
	if strings.EqualFold(input.Reason, "EXPIRED") {
		targetStatus = model.ReservationStatusExpired
	}

	notes := "Stock released"
	if strings.TrimSpace(input.Reason) != "" {
		notes = fmt.Sprintf("Stock released (%s)", strings.TrimSpace(input.Reason))
	}

	for _, row := range activeRows {
		var updateInvQuery string
		var invArgs []interface{}
		if row.variantID == nil || *row.variantID == "" {
			updateInvQuery = `
				UPDATE inventories
				SET available_quantity = available_quantity + $1,
				    reserved_quantity = GREATEST(reserved_quantity - $1, 0),
				    updated_at = $2
				WHERE product_id = $3 AND variant_id IS NULL
				RETURNING id, sku, available_quantity, reserved_quantity;
			`
			invArgs = []interface{}{row.quantity, now, row.productID}
		} else {
			updateInvQuery = `
				UPDATE inventories
				SET available_quantity = available_quantity + $1,
				    reserved_quantity = GREATEST(reserved_quantity - $1, 0),
				    updated_at = $2
				WHERE product_id = $3 AND variant_id = $4
				RETURNING id, sku, available_quantity, reserved_quantity;
			`
			invArgs = []interface{}{row.quantity, now, row.productID, *row.variantID}
		}

		var invID, sku string
		var newAvail, newRes int
		err := tx.QueryRow(ctx, updateInvQuery, invArgs...).Scan(&invID, &sku, &newAvail, &newRes)
		if err != nil {
			return appErrors.Internal(err, "failed to return stock to available inventory")
		}

		ref := row.orderID
		notesVal := notes
		txQuery := `
			INSERT INTO inventory_transactions (
				id, inventory_id, type, quantity, reference_id, notes, created_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7
			);
		`
		_, err = tx.Exec(ctx, txQuery, uuid.NewString(), invID, model.TransactionTypeRelease, row.quantity, &ref, &notesVal, now)
		if err != nil {
			return appErrors.Internal(err, "failed to record release transaction")
		}

		resUpdateQuery := `
			UPDATE inventory_reservations
			SET status = $1, updated_at = $2
			WHERE id = $3 AND status = 'RESERVED';
		`
		_, err = tx.Exec(ctx, resUpdateQuery, targetStatus, now, row.id)
		if err != nil {
			return appErrors.Internal(err, "failed to update reservation status")
		}

		relEvent := events.InventoryReleasedEvent{
			EventType:         events.EventInventoryReleased,
			InventoryID:       invID,
			ProductID:         row.productID,
			VariantID:         row.variantID,
			SKU:               sku,
			OrderID:           row.orderID,
			ReservationID:     row.id,
			Quantity:          row.quantity,
			Reason:            input.Reason,
			AvailableQuantity: newAvail,
			ReservedQuantity:  newRes,
			Timestamp:         now,
		}
		if err := writeOutboxEvent(ctx, tx, "INVENTORY", invID, events.EventInventoryReleased, contractsEvents.TopicInventoryReleased, relEvent, now); err != nil {
			return appErrors.Internal(err, "failed to record InventoryReleased outbox event")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return appErrors.Internal(err, "failed to commit release transaction")
	}

	return nil
}

func (r *pgInventoryRepository) CommitStock(ctx context.Context, input dto.CommitStockInput) error {
	resID := strings.TrimSpace(input.ReservationID)
	orderID := strings.TrimSpace(input.OrderID)

	if resID == "" && orderID == "" {
		return appErrors.BadRequest("either reservation_id or order_id is required for commit")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return appErrors.Internal(err, "failed to begin commit transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()

	var selectQuery string
	var args []interface{}

	if resID != "" {
		selectQuery = `
			SELECT id, order_id, product_id, variant_id, quantity, status
			FROM inventory_reservations
			WHERE id = $1
			FOR UPDATE;
		`
		args = []interface{}{resID}
	} else {
		selectQuery = `
			SELECT id, order_id, product_id, variant_id, quantity, status
			FROM inventory_reservations
			WHERE order_id = $1
			FOR UPDATE;
		`
		args = []interface{}{orderID}
	}

	rows, err := tx.Query(ctx, selectQuery, args...)
	if err != nil {
		return appErrors.Internal(err, "failed to fetch reservations for commit")
	}
	defer rows.Close()

	type resRow struct {
		id, orderID, productID string
		variantID              *string
		quantity               int
		status                 model.ReservationStatus
	}
	var allRows []resRow
	var activeRows []resRow
	var confirmedCount int
	var releasedCount int

	for rows.Next() {
		var row resRow
		if err := rows.Scan(&row.id, &row.orderID, &row.productID, &row.variantID, &row.quantity, &row.status); err != nil {
			return appErrors.Internal(err, "failed to scan reservation row for commit")
		}
		allRows = append(allRows, row)
		if row.status == model.ReservationStatusReserved {
			activeRows = append(activeRows, row)
		} else if row.status == model.ReservationStatusConfirmed {
			confirmedCount++
		} else if row.status == model.ReservationStatusReleased || row.status == model.ReservationStatusExpired {
			releasedCount++
		}
	}
	rows.Close()

	if len(allRows) == 0 {
		return appErrors.NotFound("reservation not found")
	}

	if len(activeRows) == 0 {
		if confirmedCount > 0 {
			// Already committed idempotently
			return nil
		}
		if releasedCount > 0 {
			return appErrors.Conflict("cannot commit released or expired reservation")
		}
		return appErrors.Conflict("reservation is not in a committable state")
	}

	for _, row := range activeRows {
		var updateInvQuery string
		var invArgs []interface{}

		if row.variantID == nil || *row.variantID == "" {
			updateInvQuery = `
				UPDATE inventories
				SET reserved_quantity = GREATEST(reserved_quantity - $1, 0),
				    updated_at = $2
				WHERE product_id = $3 AND variant_id IS NULL
				RETURNING id, sku, available_quantity, reserved_quantity;
			`
			invArgs = []interface{}{row.quantity, now, row.productID}
		} else {
			updateInvQuery = `
				UPDATE inventories
				SET reserved_quantity = GREATEST(reserved_quantity - $1, 0),
				    updated_at = $2
				WHERE product_id = $3 AND variant_id = $4
				RETURNING id, sku, available_quantity, reserved_quantity;
			`
			invArgs = []interface{}{row.quantity, now, row.productID, *row.variantID}
		}

		var invID, sku string
		var avail, newRes int
		err := tx.QueryRow(ctx, updateInvQuery, invArgs...).Scan(&invID, &sku, &avail, &newRes)
		if err != nil {
			return appErrors.Internal(err, "failed to deduct reserved stock for commit")
		}

		ref := row.orderID
		notes := fmt.Sprintf("Stock committed for order %s", row.orderID)
		txQuery := `
			INSERT INTO inventory_transactions (
				id, inventory_id, type, quantity, reference_id, notes, created_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7
			);
		`
		_, err = tx.Exec(ctx, txQuery, uuid.NewString(), invID, model.TransactionTypeCommit, row.quantity, &ref, &notes, now)
		if err != nil {
			return appErrors.Internal(err, "failed to record commit transaction")
		}

		resUpdateQuery := `
			UPDATE inventory_reservations
			SET status = 'CONFIRMED', updated_at = $1
			WHERE id = $2 AND status = 'RESERVED';
		`
		_, err = tx.Exec(ctx, resUpdateQuery, now, row.id)
		if err != nil {
			return appErrors.Internal(err, "failed to update reservation status to CONFIRMED")
		}

		commitEvent := events.InventoryCommittedEvent{
			EventType:         events.EventInventoryCommitted,
			InventoryID:       invID,
			ProductID:         row.productID,
			VariantID:         row.variantID,
			SKU:               sku,
			OrderID:           row.orderID,
			ReservationID:     row.id,
			Quantity:          row.quantity,
			AvailableQuantity: avail,
			ReservedQuantity:  newRes,
			Timestamp:         now,
		}
		if err := writeOutboxEvent(ctx, tx, "INVENTORY", invID, events.EventInventoryCommitted, contractsEvents.TopicInventoryCommitted, commitEvent, now); err != nil {
			return appErrors.Internal(err, "failed to record InventoryCommitted outbox event")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return appErrors.Internal(err, "failed to commit transaction")
	}

	return nil
}

func (r *pgInventoryRepository) ReleaseExpiredReservations(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	query := `
		SELECT DISTINCT id
		FROM inventory_reservations
		WHERE status = 'RESERVED' AND expires_at <= $1;
	`
	rows, err := r.pool.Query(ctx, query, now)
	if err != nil {
		return 0, appErrors.Internal(err, "failed to query expired reservations")
	}
	defer rows.Close()

	var expiredIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			expiredIDs = append(expiredIDs, id)
		}
	}
	rows.Close()

	releasedCount := 0
	for _, id := range expiredIDs {
		err := r.ReleaseStock(ctx, dto.ReleaseStockInput{
			ReservationID: id,
			Reason:        "EXPIRED",
		})
		if err == nil {
			releasedCount++
		}
	}

	return releasedCount, nil
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
