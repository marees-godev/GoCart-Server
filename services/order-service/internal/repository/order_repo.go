package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/model"
)

var (
	ErrOrderNotFound = errors.New("order not found")
)

type OrderRepository interface {
	CreateOrdersWithOutbox(
		ctx context.Context,
		orders []*model.Order,
		envelopes []*events.EventEnvelope,
		topic string,
	) error
	GetOrderByID(ctx context.Context, id string) (*model.Order, error)
	ListOrdersByUserID(ctx context.Context, userID string, limit, offset int) ([]*model.Order, int, error)
	UpdateOrderStatus(ctx context.Context, id string, status model.OrderStatus) (*model.Order, error)
	CancelOrder(ctx context.Context, id string, envelope *events.EventEnvelope, topic string) (*model.Order, error)
	CreateOrderWithOutbox(
		ctx context.Context,
		orderID string,
		insertOrder func(ctx context.Context, tx pgx.Tx) error,
		envelope *events.EventEnvelope,
		topic string,
	) error
}

type OrderRepo struct {
	db          *database.DB
	outboxStore *outbox.Store
}

func NewOrderRepo(db *database.DB, outboxStore *outbox.Store) *OrderRepo {
	return &OrderRepo{db: db, outboxStore: outboxStore}
}

func (r *OrderRepo) CreateOrdersWithOutbox(
	ctx context.Context,
	orders []*model.Order,
	envelopes []*events.EventEnvelope,
	topic string,
) error {
	return r.db.WithTransaction(ctx, func(tx pgx.Tx) error {
		for _, order := range orders {
			insertOrderQuery := `
				INSERT INTO orders (
					id, order_number, parent_order_id, user_id, store_id,
					subtotal, tax_amount, shipping_fee, discount_amount, total_amount,
					currency, payment_status, status, shipping_address, billing_address,
					shipping_address_id, billing_address_id, idempotency_key, created_at, updated_at
				) VALUES (
					$1, $2, $3, $4, $5,
					$6, $7, $8, $9, $10,
					$11, $12, $13, $14, $15,
					$16, $17, $18, $19, $20
				)`

			var parentID any
			if order.ParentOrderID != nil && *order.ParentOrderID != "" {
				parentID = *order.ParentOrderID
			}
			var shipAddrID any
			if order.ShippingAddressID != nil && *order.ShippingAddressID != "" {
				shipAddrID = *order.ShippingAddressID
			}
			var billAddrID any
			if order.BillingAddressID != nil && *order.BillingAddressID != "" {
				billAddrID = *order.BillingAddressID
			}
			var idempKey any
			if order.IdempotencyKey != nil && *order.IdempotencyKey != "" {
				idempKey = *order.IdempotencyKey
			}

			shippingJSON := order.ShippingAddress
			if shippingJSON == "" {
				shippingJSON = "{}"
			}
			billingJSON := order.BillingAddress
			if billingJSON == "" {
				billingJSON = "{}"
			}

			_, err := tx.Exec(ctx, insertOrderQuery,
				order.ID, order.OrderNumber, parentID, order.UserID, order.StoreID,
				order.Subtotal, order.TaxAmount, order.ShippingFee, order.DiscountAmount, order.TotalAmount,
				order.Currency, string(order.PaymentStatus), string(order.Status), shippingJSON, billingJSON,
				shipAddrID, billAddrID, idempKey, order.CreatedAt, order.UpdatedAt,
			)
			if err != nil {
				return fmt.Errorf("failed to insert order %s: %w", order.ID, err)
			}

			insertItemQuery := `
				INSERT INTO order_items (
					id, order_id, product_id, store_id, sku,
					product_name, quantity, unit_price, tax, discount, total_price
				) VALUES (
					$1, $2, $3, $4, $5,
					$6, $7, $8, $9, $10, $11
				)`

			for _, item := range order.Items {
				_, err := tx.Exec(ctx, insertItemQuery,
					item.ID, item.OrderID, item.ProductID, item.StoreID, item.SKU,
					item.ProductName, item.Quantity, item.UnitPrice, item.Tax, item.Discount, item.TotalPrice,
				)
				if err != nil {
					return fmt.Errorf("failed to insert order item %s for order %s: %w", item.ID, order.ID, err)
				}
			}
		}

		for _, env := range envelopes {
			payload, err := env.Marshal()
			if err != nil {
				return fmt.Errorf("failed to marshal outbox event: %w", err)
			}

			evt := &outbox.Event{
				AggregateType: "order",
				AggregateID:   env.AggregateID,
				EventType:     env.EventType,
				Payload:       payload,
				Topic:         topic,
			}

			if err := r.outboxStore.Insert(ctx, tx, evt); err != nil {
				return fmt.Errorf("failed to insert outbox event: %w", err)
			}
		}

		return nil
	})
}

func (r *OrderRepo) CreateOrderWithOutbox(
	ctx context.Context,
	orderID string,
	insertOrder func(ctx context.Context, tx pgx.Tx) error,
	envelope *events.EventEnvelope,
	topic string,
) error {
	return r.db.WithTransaction(ctx, func(tx pgx.Tx) error {
		if err := insertOrder(ctx, tx); err != nil {
			return err
		}

		payload, err := envelope.Marshal()
		if err != nil {
			return err
		}

		evt := &outbox.Event{
			AggregateType: "order",
			AggregateID:   orderID,
			EventType:     envelope.EventType,
			Payload:       payload,
			Topic:         topic,
		}

		return r.outboxStore.Insert(ctx, tx, evt)
	})
}

func (r *OrderRepo) GetOrderByID(ctx context.Context, id string) (*model.Order, error) {
	query := `
		SELECT 
			id, order_number, parent_order_id, user_id, store_id,
			subtotal, tax_amount, shipping_fee, discount_amount, total_amount,
			currency, payment_status, status, shipping_address::text, billing_address::text,
			shipping_address_id, billing_address_id, idempotency_key, created_at, updated_at
		FROM orders
		WHERE id = $1`

	var o model.Order
	var parentID, shipAddrID, billAddrID, idempKey *string
	var shipAddrStr, billAddrStr *string

	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&o.ID, &o.OrderNumber, &parentID, &o.UserID, &o.StoreID,
		&o.Subtotal, &o.TaxAmount, &o.ShippingFee, &o.DiscountAmount, &o.TotalAmount,
		&o.Currency, &o.PaymentStatus, &o.Status, &shipAddrStr, &billAddrStr,
		&shipAddrID, &billAddrID, &idempKey, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("failed to query order by id: %w", err)
	}

	o.ParentOrderID = parentID
	o.ShippingAddressID = shipAddrID
	o.BillingAddressID = billAddrID
	o.IdempotencyKey = idempKey
	if shipAddrStr != nil {
		o.ShippingAddress = *shipAddrStr
	}
	if billAddrStr != nil {
		o.BillingAddress = *billAddrStr
	}

	items, err := r.getItemsByOrderID(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	o.Items = items

	return &o, nil
}

func (r *OrderRepo) ListOrdersByUserID(ctx context.Context, userID string, limit, offset int) ([]*model.Order, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	countQuery := `SELECT COUNT(*) FROM orders WHERE user_id = $1`
	var total int
	if err := r.db.Pool.QueryRow(ctx, countQuery, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count orders for user: %w", err)
	}

	query := `
		SELECT 
			id, order_number, parent_order_id, user_id, store_id,
			subtotal, tax_amount, shipping_fee, discount_amount, total_amount,
			currency, payment_status, status, shipping_address::text, billing_address::text,
			shipping_address_id, billing_address_id, idempotency_key, created_at, updated_at
		FROM orders
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Pool.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query orders for user: %w", err)
	}
	defer rows.Close()

	orders := make([]*model.Order, 0)
	for rows.Next() {
		var o model.Order
		var parentID, shipAddrID, billAddrID, idempKey *string
		var shipAddrStr, billAddrStr *string

		if err := rows.Scan(
			&o.ID, &o.OrderNumber, &parentID, &o.UserID, &o.StoreID,
			&o.Subtotal, &o.TaxAmount, &o.ShippingFee, &o.DiscountAmount, &o.TotalAmount,
			&o.Currency, &o.PaymentStatus, &o.Status, &shipAddrStr, &billAddrStr,
			&shipAddrID, &billAddrID, &idempKey, &o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan order row: %w", err)
		}

		o.ParentOrderID = parentID
		o.ShippingAddressID = shipAddrID
		o.BillingAddressID = billAddrID
		o.IdempotencyKey = idempKey
		if shipAddrStr != nil {
			o.ShippingAddress = *shipAddrStr
		}
		if billAddrStr != nil {
			o.BillingAddress = *billAddrStr
		}
		orders = append(orders, &o)
	}

	for _, o := range orders {
		items, err := r.getItemsByOrderID(ctx, o.ID)
		if err != nil {
			return nil, 0, err
		}
		o.Items = items
	}

	return orders, total, nil
}

func (r *OrderRepo) UpdateOrderStatus(ctx context.Context, id string, status model.OrderStatus) (*model.Order, error) {
	query := `
		UPDATE orders
		SET status = $1, updated_at = NOW()
		WHERE id = $2
		RETURNING 
			id, order_number, parent_order_id, user_id, store_id,
			subtotal, tax_amount, shipping_fee, discount_amount, total_amount,
			currency, payment_status, status, shipping_address::text, billing_address::text,
			shipping_address_id, billing_address_id, idempotency_key, created_at, updated_at`

	var o model.Order
	var parentID, shipAddrID, billAddrID, idempKey *string
	var shipAddrStr, billAddrStr *string

	err := r.db.Pool.QueryRow(ctx, query, string(status), id).Scan(
		&o.ID, &o.OrderNumber, &parentID, &o.UserID, &o.StoreID,
		&o.Subtotal, &o.TaxAmount, &o.ShippingFee, &o.DiscountAmount, &o.TotalAmount,
		&o.Currency, &o.PaymentStatus, &o.Status, &shipAddrStr, &billAddrStr,
		&shipAddrID, &billAddrID, &idempKey, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("failed to update order status: %w", err)
	}

	o.ParentOrderID = parentID
	o.ShippingAddressID = shipAddrID
	o.BillingAddressID = billAddrID
	o.IdempotencyKey = idempKey
	if shipAddrStr != nil {
		o.ShippingAddress = *shipAddrStr
	}
	if billAddrStr != nil {
		o.BillingAddress = *billAddrStr
	}

	items, err := r.getItemsByOrderID(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	o.Items = items

	return &o, nil
}

func (r *OrderRepo) getItemsByOrderID(ctx context.Context, orderID string) ([]model.OrderItem, error) {
	query := `
		SELECT id, order_id, product_id, store_id, sku, product_name, quantity, unit_price, tax, discount, total_price
		FROM order_items
		WHERE order_id = $1`

	rows, err := r.db.Pool.Query(ctx, query, orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to query order items: %w", err)
	}
	defer rows.Close()

	items := make([]model.OrderItem, 0)
	for rows.Next() {
		var it model.OrderItem
		if err := rows.Scan(
			&it.ID, &it.OrderID, &it.ProductID, &it.StoreID, &it.SKU,
			&it.ProductName, &it.Quantity, &it.UnitPrice, &it.Tax, &it.Discount, &it.TotalPrice,
		); err != nil {
			return nil, fmt.Errorf("failed to scan order item row: %w", err)
		}
		items = append(items, it)
	}
	return items, nil
}

func (r *OrderRepo) CancelOrder(ctx context.Context, id string, envelope *events.EventEnvelope, topic string) (*model.Order, error) {
	var updatedOrder *model.Order

	err := r.db.WithTransaction(ctx, func(tx pgx.Tx) error {
		query := `
			UPDATE orders
			SET status = $1, updated_at = NOW()
			WHERE id = $2
			RETURNING 
				id, order_number, parent_order_id, user_id, store_id,
				subtotal, tax_amount, shipping_fee, discount_amount, total_amount,
				currency, payment_status, status, shipping_address::text, billing_address::text,
				shipping_address_id, billing_address_id, idempotency_key, created_at, updated_at`

		var o model.Order
		var parentID, shipAddrID, billAddrID, idempKey *string
		var shipAddrStr, billAddrStr *string

		err := tx.QueryRow(ctx, query, string(model.OrderStatusCancelled), id).Scan(
			&o.ID, &o.OrderNumber, &parentID, &o.UserID, &o.StoreID,
			&o.Subtotal, &o.TaxAmount, &o.ShippingFee, &o.DiscountAmount, &o.TotalAmount,
			&o.Currency, &o.PaymentStatus, &o.Status, &shipAddrStr, &billAddrStr,
			&shipAddrID, &billAddrID, &idempKey, &o.CreatedAt, &o.UpdatedAt,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrOrderNotFound
			}
			return fmt.Errorf("failed to cancel order: %w", err)
		}

		o.ParentOrderID = parentID
		o.ShippingAddressID = shipAddrID
		o.BillingAddressID = billAddrID
		o.IdempotencyKey = idempKey
		if shipAddrStr != nil {
			o.ShippingAddress = *shipAddrStr
		}
		if billAddrStr != nil {
			o.BillingAddress = *billAddrStr
		}

		if envelope != nil && r.outboxStore != nil {
			payload, err := envelope.Marshal()
			if err != nil {
				return fmt.Errorf("failed to marshal cancellation outbox event: %w", err)
			}
			evt := &outbox.Event{
				AggregateType: "order",
				AggregateID:   id,
				EventType:     envelope.EventType,
				Payload:       payload,
				Topic:         topic,
			}
			if err := r.outboxStore.Insert(ctx, tx, evt); err != nil {
				return fmt.Errorf("failed to insert cancellation outbox event: %w", err)
			}
		}

		updatedOrder = &o
		return nil
	})
	if err != nil {
		return nil, err
	}

	items, err := r.getItemsByOrderID(ctx, updatedOrder.ID)
	if err != nil {
		return nil, err
	}
	updatedOrder.Items = items

	return updatedOrder, nil
}

