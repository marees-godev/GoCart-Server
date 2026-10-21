package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
)

type DeliveryRepository interface {
	Create(ctx context.Context, delivery *model.Delivery) error
	GetByID(ctx context.Context, id string) (*model.Delivery, error)
	GetByOrderID(ctx context.Context, orderID string) (*model.Delivery, error)
	GetByTrackingNumber(ctx context.Context, trackingNumber string) (*model.Delivery, error)
	List(ctx context.Context, limit, offset int, status string) ([]*model.Delivery, int, error)
	UpdateStatus(ctx context.Context, id string, status model.DeliveryStatus) (*model.Delivery, error)
	UpsertTempOrder(ctx context.Context, order *model.TempOrder) error
	GetTempOrderByID(ctx context.Context, id string) (*model.TempOrder, error)

	// Warehouse & Partner Profile operations
	GetAvailableWarehouses(ctx context.Context) ([]*model.Warehouse, error)
	GetWarehouseByID(ctx context.Context, id uuid.UUID) (*model.Warehouse, error)
	CreateWarehouse(ctx context.Context, w *model.Warehouse) error
	CreateOrUpdatePartnerProfile(ctx context.Context, profile *model.DeliveryPartnerProfile) error
	GetPartnerProfile(ctx context.Context, partnerID uuid.UUID) (*model.DeliveryPartnerProfile, error)
	AssignPartnerWarehouses(ctx context.Context, partnerID uuid.UUID, warehouseIDs []uuid.UUID) error
	GetPartnerWarehouses(ctx context.Context, partnerID uuid.UUID) ([]*model.Warehouse, error)
}

type pgDeliveryRepository struct {
	db          *database.DB
	outboxStore *outbox.Store
	logger      *slog.Logger
}

func NewDeliveryRepository(db *database.DB, log ...*slog.Logger) DeliveryRepository {
	return NewDeliveryRepositoryWithOutbox(db, outbox.NewStore(), log...)
}

func NewDeliveryRepositoryWithOutbox(db *database.DB, outboxStore *outbox.Store, log ...*slog.Logger) DeliveryRepository {
	var l *slog.Logger
	if len(log) > 0 && log[0] != nil {
		l = log[0]
	} else {
		l = slog.Default()
	}
	if outboxStore == nil {
		outboxStore = outbox.NewStore()
	}
	return &pgDeliveryRepository{
		db:          db,
		outboxStore: outboxStore,
		logger:      l,
	}
}

func (r *pgDeliveryRepository) Create(ctx context.Context, delivery *model.Delivery) error {
	if delivery.ID == "" {
		delivery.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = now
	}
	if delivery.UpdatedAt.IsZero() {
		delivery.UpdatedAt = now
	}
	if delivery.Status == "" {
		delivery.Status = model.StatusCreated
	}

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		r.logger.Error("Repository: failed to begin transaction for delivery creation", slog.Any("error", err))
		return appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	addrBytes, err := json.Marshal(delivery.ShippingAddress)
	if err != nil {
		r.logger.Error("Repository: failed to marshal shipping address", slog.Any("error", err))
		return appErrors.BadRequest("failed to marshal shipping address")
	}

	orderNumID := strings.ReplaceAll(delivery.OrderID, "-", "")
	if len(orderNumID) > 32 {
		orderNumID = orderNumID[:32]
	}
	orderNumber := fmt.Sprintf("ORD-%s", strings.ToUpper(orderNumID))

	tempOrderQuery := `
		INSERT INTO temp_orders (
			id, order_number, status, total_amount, currency,
			street, city, state, postal_code, country, contact_number, shipping_address,
			created_at, updated_at
		) VALUES ($1, $2, 'CONFIRMED', 0.00, 'USD', $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			street = EXCLUDED.street,
			city = EXCLUDED.city,
			state = EXCLUDED.state,
			postal_code = EXCLUDED.postal_code,
			country = EXCLUDED.country,
			contact_number = EXCLUDED.contact_number,
			shipping_address = EXCLUDED.shipping_address,
			updated_at = EXCLUDED.updated_at
	`
	if _, err := tx.Exec(ctx, tempOrderQuery,
		delivery.OrderID,
		orderNumber,
		delivery.ShippingAddress.Street,
		delivery.ShippingAddress.City,
		delivery.ShippingAddress.State,
		delivery.ShippingAddress.PostalCode,
		delivery.ShippingAddress.Country,
		delivery.ShippingAddress.ContactNumber,
		addrBytes,
		delivery.CreatedAt,
		delivery.UpdatedAt,
	); err != nil {
		r.logger.Error("Repository: failed to upsert temporary order", slog.String("order_id", delivery.OrderID), slog.Any("error", err))
		return appErrors.Internal(err, "failed to record temporary order")
	}

	query := `
		INSERT INTO deliveries (
			id, order_id, delivery_partner, tracking_number, status,
			street, city, state, postal_code, country, contact_number, shipping_address,
			estimated_delivery, actual_delivery,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (order_id) DO NOTHING
		RETURNING id, created_at, updated_at
	`

	err = tx.QueryRow(ctx, query,
		delivery.ID,
		delivery.OrderID,
		delivery.DeliveryPartner,
		delivery.TrackingNumber,
		delivery.Status,
		delivery.ShippingAddress.Street,
		delivery.ShippingAddress.City,
		delivery.ShippingAddress.State,
		delivery.ShippingAddress.PostalCode,
		delivery.ShippingAddress.Country,
		delivery.ShippingAddress.ContactNumber,
		addrBytes,
		delivery.EstimatedDelivery,
		delivery.ActualDelivery,
		delivery.CreatedAt,
		delivery.UpdatedAt,
	).Scan(&delivery.ID, &delivery.CreatedAt, &delivery.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Info("Repository: delivery record already exists for order, fetching existing", slog.String("order_id", delivery.OrderID))
			existing, getErr := r.GetByOrderID(ctx, delivery.OrderID)
			if getErr != nil {
				return getErr
			}
			*delivery = *existing
			return nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			r.logger.Info("Repository: duplicate delivery record detected, fetching existing", slog.String("order_id", delivery.OrderID))
			existing, getErr := r.GetByOrderID(ctx, delivery.OrderID)
			if getErr != nil {
				return getErr
			}
			*delivery = *existing
			return nil
		}
		r.logger.Error("Repository: failed to insert delivery", slog.String("order_id", delivery.OrderID), slog.Any("error", err))
		return appErrors.Internal(err, "failed to insert delivery record")
	}

	// Transactional Outbox: Persist DeliveryCreated event in outbox inside same DB transaction
	createdEvt := events.DeliveryCreatedEvent{
		DeliveryID:      delivery.ID,
		OrderID:         delivery.OrderID,
		DeliveryPartner: delivery.DeliveryPartner,
		TrackingNumber:  delivery.TrackingNumber,
		Status:          string(delivery.Status),
		CreatedAt:       delivery.CreatedAt,
	}
	envelope, err := events.NewEventEnvelopeWithAggregate(events.EventTypeDeliveryCreated, "delivery-service", delivery.ID, createdEvt)
	if err != nil {
		r.logger.Error("Repository: failed to construct DeliveryCreated envelope", slog.Any("error", err))
		return appErrors.Internal(err, "failed to construct event envelope")
	}
	payloadBytes, err := envelope.Marshal()
	if err != nil {
		r.logger.Error("Repository: failed to marshal DeliveryCreated payload", slog.Any("error", err))
		return appErrors.Internal(err, "failed to marshal event payload")
	}

	outboxEvt := &outbox.Event{
		AggregateType: "delivery",
		AggregateID:   delivery.ID,
		EventType:     events.EventTypeDeliveryCreated,
		Payload:       payloadBytes,
		Topic:         events.TopicDeliveryCreated,
	}
	if err := r.outboxStore.Insert(ctx, tx, outboxEvt); err != nil {
		r.logger.Error("Repository: failed to write DeliveryCreated event to outbox", slog.Any("error", err))
		return appErrors.Internal(err, "failed to record outbox event")
	}

	if err := tx.Commit(ctx); err != nil {
		r.logger.Error("Repository: failed to commit create delivery transaction", slog.Any("error", err))
		return appErrors.Internal(err, "failed to commit transaction")
	}

	r.logger.Debug("Repository: delivery and outbox event inserted successfully", slog.String("delivery_id", delivery.ID))
	return nil
}

func (r *pgDeliveryRepository) GetByID(ctx context.Context, id string) (*model.Delivery, error) {
	cleanID := strings.TrimSpace(id)
	var query string
	if _, parseErr := uuid.Parse(cleanID); parseErr == nil {
		query = `
			SELECT id, order_id, delivery_partner, tracking_number, status,
			       street, city, state, postal_code, country, contact_number, shipping_address,
			       estimated_delivery, actual_delivery,
			       created_at, updated_at
			FROM deliveries
			WHERE id = $1 OR order_id = $1
		`
	} else {
		query = `
			SELECT id, order_id, delivery_partner, tracking_number, status,
			       street, city, state, postal_code, country, contact_number, shipping_address,
			       estimated_delivery, actual_delivery,
			       created_at, updated_at
			FROM deliveries
			WHERE id::text = $1 OR order_id::text = $1
		`
	}
	delivery, err := r.scanOne(r.db.Pool.QueryRow(ctx, query, cleanID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Warn("Repository: delivery not found by id", slog.String("delivery_id", cleanID))
			return nil, appErrors.NotFound("delivery not found")
		}
		r.logger.Error("Repository: failed to query delivery by id", slog.String("delivery_id", cleanID), slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to query delivery by id")
	}
	return delivery, nil
}

func (r *pgDeliveryRepository) GetByOrderID(ctx context.Context, orderID string) (*model.Delivery, error) {
	query := `
		SELECT id, order_id, delivery_partner, tracking_number, status,
		       street, city, state, postal_code, country, contact_number, shipping_address,
		       estimated_delivery, actual_delivery,
		       created_at, updated_at
		FROM deliveries
		WHERE order_id = $1
	`
	delivery, err := r.scanOne(r.db.Pool.QueryRow(ctx, query, orderID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Warn("Repository: delivery not found by order_id", slog.String("order_id", orderID))
			return nil, appErrors.NotFound("delivery not found for order")
		}
		r.logger.Error("Repository: failed to query delivery by order_id", slog.String("order_id", orderID), slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to query delivery by order_id")
	}
	return delivery, nil
}

func (r *pgDeliveryRepository) GetByTrackingNumber(ctx context.Context, trackingNumber string) (*model.Delivery, error) {
	query := `
		SELECT id, order_id, delivery_partner, tracking_number, status,
		       street, city, state, postal_code, country, contact_number, shipping_address,
		       estimated_delivery, actual_delivery,
		       created_at, updated_at
		FROM deliveries
		WHERE tracking_number = $1
	`
	delivery, err := r.scanOne(r.db.Pool.QueryRow(ctx, query, trackingNumber))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Warn("Repository: delivery not found by tracking_number", slog.String("tracking_number", trackingNumber))
			return nil, appErrors.NotFound("delivery not found for tracking number")
		}
		r.logger.Error("Repository: failed to query delivery by tracking_number", slog.String("tracking_number", trackingNumber), slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to query delivery by tracking_number")
	}
	return delivery, nil
}

func (r *pgDeliveryRepository) List(ctx context.Context, limit, offset int, status string) ([]*model.Delivery, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var countQuery string
	var listQuery string
	var args []interface{}
	var countArgs []interface{}

	if status != "" {
		countQuery = `SELECT COUNT(*) FROM deliveries WHERE status::text = $1`
		countArgs = append(countArgs, status)
		listQuery = `
			SELECT id, order_id, delivery_partner, tracking_number, status,
			       street, city, state, postal_code, country, contact_number, shipping_address,
			       estimated_delivery, actual_delivery,
			       created_at, updated_at
			FROM deliveries
			WHERE status::text = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`
		args = append(args, status, limit, offset)
	} else {
		countQuery = `SELECT COUNT(*) FROM deliveries`
		listQuery = `
			SELECT id, order_id, delivery_partner, tracking_number, status,
			       street, city, state, postal_code, country, contact_number, shipping_address,
			       estimated_delivery, actual_delivery,
			       created_at, updated_at
			FROM deliveries
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`
		args = append(args, limit, offset)
	}

	var total int
	err := r.db.Pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		r.logger.Error("Repository: failed to count deliveries", slog.Any("error", err))
		return nil, 0, appErrors.Internal(err, "failed to count deliveries")
	}

	rows, err := r.db.Pool.Query(ctx, listQuery, args...)
	if err != nil {
		r.logger.Error("Repository: failed to list deliveries", slog.Any("error", err))
		return nil, 0, appErrors.Internal(err, "failed to list deliveries")
	}
	defer rows.Close()

	var deliveries []*model.Delivery
	for rows.Next() {
		d, scanErr := r.scanOne(rows)
		if scanErr != nil {
			r.logger.Error("Repository: failed to scan delivery row", slog.Any("error", scanErr))
			return nil, 0, appErrors.Internal(scanErr, "failed to scan delivery row")
		}
		deliveries = append(deliveries, d)
	}

	return deliveries, total, nil
}

func (r *pgDeliveryRepository) UpdateStatus(ctx context.Context, id string, status model.DeliveryStatus) (*model.Delivery, error) {
	cleanID := strings.TrimSpace(id)
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		r.logger.Error("Repository: failed to begin transaction for status update", slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var queryFetch string
	if _, parseErr := uuid.Parse(cleanID); parseErr == nil {
		queryFetch = `
			SELECT id, order_id, delivery_partner, tracking_number, status,
			       street, city, state, postal_code, country, contact_number, shipping_address,
			       estimated_delivery, actual_delivery,
			       created_at, updated_at
			FROM deliveries
			WHERE id = $1 OR order_id = $1
			FOR UPDATE
		`
	} else {
		queryFetch = `
			SELECT id, order_id, delivery_partner, tracking_number, status,
			       street, city, state, postal_code, country, contact_number, shipping_address,
			       estimated_delivery, actual_delivery,
			       created_at, updated_at
			FROM deliveries
			WHERE id::text = $1 OR order_id::text = $1
			FOR UPDATE
		`
	}
	existing, err := r.scanOne(tx.QueryRow(ctx, queryFetch, cleanID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Warn("Repository: delivery not found for status update", slog.String("delivery_id", cleanID))
			return nil, appErrors.NotFound("delivery not found")
		}
		r.logger.Error("Repository: failed to fetch delivery for update", slog.String("delivery_id", cleanID), slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to fetch delivery for update")
	}

	cleanStatus := model.DeliveryStatus(strings.ToUpper(strings.TrimSpace(string(status))))
	if existing.Status == cleanStatus {
		r.logger.Info("Repository: delivery is already in target status (idempotent)", slog.String("delivery_id", existing.ID), slog.String("status", string(cleanStatus)))
		return existing, nil
	}

	previousStatus := existing.Status
	if err := existing.TransitionStatus(cleanStatus); err != nil {
		var transErr *model.InvalidStateTransitionError
		if errors.As(err, &transErr) {
			return nil, transErr.ToAppError()
		}
		return nil, appErrors.New(appErrors.CodeUnprocessableEntity, err.Error(), http.StatusUnprocessableEntity)
	}

	queryUpdate := `
		UPDATE deliveries
		SET status = $2, updated_at = $3
		WHERE id = $1
		RETURNING id, order_id, delivery_partner, tracking_number, status,
		          street, city, state, postal_code, country, contact_number, shipping_address,
		          estimated_delivery, actual_delivery,
		          created_at, updated_at
	`
	updatedDelivery, err := r.scanOne(tx.QueryRow(ctx, queryUpdate, existing.ID, cleanStatus, existing.UpdatedAt))
	if err != nil {
		r.logger.Error("Repository: failed to update delivery status", slog.String("delivery_id", existing.ID), slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to update delivery status")
	}

	// Record tracking history
	_, _ = tx.Exec(ctx, `
		INSERT INTO delivery_tracking_updates (delivery_id, status, created_at)
		VALUES ($1, $2, NOW())
	`, existing.ID, cleanStatus)

	// Transactional Outbox: Persist DeliveryStatusUpdated event in outbox inside same DB transaction
	updatedEvt := events.DeliveryStatusUpdatedEvent{
		DeliveryID:     updatedDelivery.ID,
		OrderID:        updatedDelivery.OrderID,
		PreviousStatus: string(previousStatus),
		NewStatus:      string(updatedDelivery.Status),
		TrackingNumber: updatedDelivery.TrackingNumber,
		UpdatedAt:      updatedDelivery.UpdatedAt,
	}
	envelope, err := events.NewEventEnvelopeWithAggregate(events.EventTypeDeliveryStatusUpdated, "delivery-service", updatedDelivery.ID, updatedEvt)
	if err != nil {
		r.logger.Error("Repository: failed to construct DeliveryStatusUpdated envelope", slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to construct event envelope")
	}
	payloadBytes, err := envelope.Marshal()
	if err != nil {
		r.logger.Error("Repository: failed to marshal DeliveryStatusUpdated payload", slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to marshal event payload")
	}

	outboxEvt := &outbox.Event{
		AggregateType: "delivery",
		AggregateID:   updatedDelivery.ID,
		EventType:     events.EventTypeDeliveryStatusUpdated,
		Payload:       payloadBytes,
		Topic:         events.TopicDeliveryStatusUpdated,
	}
	if err := r.outboxStore.Insert(ctx, tx, outboxEvt); err != nil {
		r.logger.Error("Repository: failed to write DeliveryStatusUpdated event to outbox", slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to record outbox event")
	}

	if err := tx.Commit(ctx); err != nil {
		r.logger.Error("Repository: failed to commit update delivery status transaction", slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to commit transaction")
	}

	r.logger.Info("Repository: delivery status updated successfully", slog.String("delivery_id", existing.ID), slog.String("new_status", string(cleanStatus)))
	return updatedDelivery, nil
}

func (r *pgDeliveryRepository) scanOne(row pgx.Row) (*model.Delivery, error) {
	var d model.Delivery
	var street, city, state, postalCode, country, contactNumber string
	var addrBytes []byte

	err := row.Scan(
		&d.ID,
		&d.OrderID,
		&d.DeliveryPartner,
		&d.TrackingNumber,
		&d.Status,
		&street,
		&city,
		&state,
		&postalCode,
		&country,
		&contactNumber,
		&addrBytes,
		&d.EstimatedDelivery,
		&d.ActualDelivery,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if len(addrBytes) > 0 {
		_ = json.Unmarshal(addrBytes, &d.ShippingAddress)
	}
	if d.ShippingAddress.Street == "" && street != "" {
		d.ShippingAddress.Street = street
	}
	if d.ShippingAddress.City == "" && city != "" {
		d.ShippingAddress.City = city
	}
	if d.ShippingAddress.State == "" && state != "" {
		d.ShippingAddress.State = state
	}
	if d.ShippingAddress.PostalCode == "" && postalCode != "" {
		d.ShippingAddress.PostalCode = postalCode
	}
	if d.ShippingAddress.Country == "" && country != "" {
		d.ShippingAddress.Country = country
	}
	if d.ShippingAddress.ContactNumber == "" && contactNumber != "" {
		d.ShippingAddress.ContactNumber = contactNumber
	}

	return &d, nil
}

func (r *pgDeliveryRepository) UpsertTempOrder(ctx context.Context, order *model.TempOrder) error {
	if order.ID == "" {
		return appErrors.BadRequest("order id is required")
	}
	if order.OrderNumber == "" {
		orderNumID := strings.ReplaceAll(order.ID, "-", "")
		if len(orderNumID) > 8 {
			orderNumID = orderNumID[:8]
		}
		order.OrderNumber = fmt.Sprintf("ORD-%s", strings.ToUpper(orderNumID))
	}
	if order.Status == "" {
		order.Status = model.TempOrderStatusConfirmed
	}
	if order.Currency == "" {
		order.Currency = "USD"
	}
	now := time.Now().UTC()
	if order.CreatedAt.IsZero() {
		order.CreatedAt = now
	}
	if order.UpdatedAt.IsZero() {
		order.UpdatedAt = now
	}

	addrBytes, err := json.Marshal(order.ShippingAddress)
	if err != nil {
		return appErrors.BadRequest("failed to marshal shipping address")
	}

	query := `
		INSERT INTO temp_orders (
			id, order_number, customer_id, status, total_amount, currency,
			street, city, state, postal_code, country, contact_number, shipping_address,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12, $13,
			$14, $15
		)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			total_amount = EXCLUDED.total_amount,
			currency = EXCLUDED.currency,
			street = EXCLUDED.street,
			city = EXCLUDED.city,
			state = EXCLUDED.state,
			postal_code = EXCLUDED.postal_code,
			country = EXCLUDED.country,
			contact_number = EXCLUDED.contact_number,
			shipping_address = EXCLUDED.shipping_address,
			updated_at = EXCLUDED.updated_at
	`
	_, err = r.db.Pool.Exec(ctx, query,
		order.ID,
		order.OrderNumber,
		order.CustomerID,
		order.Status,
		order.TotalAmount,
		order.Currency,
		order.ShippingAddress.Street,
		order.ShippingAddress.City,
		order.ShippingAddress.State,
		order.ShippingAddress.PostalCode,
		order.ShippingAddress.Country,
		order.ShippingAddress.ContactNumber,
		addrBytes,
		order.CreatedAt,
		order.UpdatedAt,
	)
	if err != nil {
		r.logger.Error("Repository: failed to upsert temp order", slog.String("order_id", order.ID), slog.Any("error", err))
		return appErrors.Internal(err, "failed to upsert temporary order")
	}
	return nil
}

func (r *pgDeliveryRepository) GetTempOrderByID(ctx context.Context, id string) (*model.TempOrder, error) {
	query := `
		SELECT id, order_number, customer_id, status, total_amount, currency,
		       street, city, state, postal_code, country, contact_number, shipping_address,
		       created_at, updated_at
		FROM temp_orders
		WHERE id = $1
	`
	var o model.TempOrder
	var street, city, state, postalCode, country, contactNumber string
	var addrBytes []byte

	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&o.ID,
		&o.OrderNumber,
		&o.CustomerID,
		&o.Status,
		&o.TotalAmount,
		&o.Currency,
		&street,
		&city,
		&state,
		&postalCode,
		&country,
		&contactNumber,
		&addrBytes,
		&o.CreatedAt,
		&o.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("temporary order not found")
		}
		r.logger.Error("Repository: failed to get temp order", slog.String("order_id", id), slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to get temporary order")
	}

	if len(addrBytes) > 0 {
		_ = json.Unmarshal(addrBytes, &o.ShippingAddress)
	}
	if o.ShippingAddress.Street == "" && street != "" {
		o.ShippingAddress.Street = street
	}
	if o.ShippingAddress.City == "" && city != "" {
		o.ShippingAddress.City = city
	}
	if o.ShippingAddress.State == "" && state != "" {
		o.ShippingAddress.State = state
	}
	if o.ShippingAddress.PostalCode == "" && postalCode != "" {
		o.ShippingAddress.PostalCode = postalCode
	}
	if o.ShippingAddress.Country == "" && country != "" {
		o.ShippingAddress.Country = country
	}
	if o.ShippingAddress.ContactNumber == "" && contactNumber != "" {
		o.ShippingAddress.ContactNumber = contactNumber
	}

	return &o, nil
}

func (r *pgDeliveryRepository) GetAvailableWarehouses(ctx context.Context) ([]*model.Warehouse, error) {
	query := `
		SELECT id, name, code, street, city, state, postal_code, is_active, created_at, updated_at
		FROM warehouses
		WHERE is_active = TRUE
		ORDER BY name ASC
	`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		r.logger.Error("Repository: failed to query available warehouses", slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to query available warehouses")
	}
	defer rows.Close()

	var warehouses []*model.Warehouse
	for rows.Next() {
		var w model.Warehouse
		if err := rows.Scan(
			&w.ID,
			&w.Name,
			&w.Code,
			&w.Street,
			&w.City,
			&w.State,
			&w.PostalCode,
			&w.IsActive,
			&w.CreatedAt,
			&w.UpdatedAt,
		); err != nil {
			r.logger.Error("Repository: failed to scan warehouse", slog.Any("error", err))
			return nil, appErrors.Internal(err, "failed to scan warehouse")
		}
		warehouses = append(warehouses, &w)
	}
	if warehouses == nil {
		warehouses = make([]*model.Warehouse, 0)
	}
	return warehouses, nil
}

func (r *pgDeliveryRepository) GetWarehouseByID(ctx context.Context, id uuid.UUID) (*model.Warehouse, error) {
	query := `
		SELECT id, name, code, street, city, state, postal_code, is_active, created_at, updated_at
		FROM warehouses
		WHERE id = $1
	`
	var w model.Warehouse
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&w.ID,
		&w.Name,
		&w.Code,
		&w.Street,
		&w.City,
		&w.State,
		&w.PostalCode,
		&w.IsActive,
		&w.CreatedAt,
		&w.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("warehouse not found")
		}
		return nil, appErrors.Internal(err, "failed to get warehouse")
	}
	return &w, nil
}

func (r *pgDeliveryRepository) CreateWarehouse(ctx context.Context, w *model.Warehouse) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	now := time.Now().UTC()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	if w.UpdatedAt.IsZero() {
		w.UpdatedAt = now
	}
	query := `
		INSERT INTO warehouses (id, name, code, street, city, state, postal_code, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (code) DO UPDATE SET
			name = EXCLUDED.name,
			street = EXCLUDED.street,
			city = EXCLUDED.city,
			state = EXCLUDED.state,
			postal_code = EXCLUDED.postal_code,
			is_active = EXCLUDED.is_active,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.db.Pool.Exec(ctx, query, w.ID, w.Name, w.Code, w.Street, w.City, w.State, w.PostalCode, w.IsActive, w.CreatedAt, w.UpdatedAt)
	if err != nil {
		r.logger.Error("Repository: failed to create warehouse", slog.Any("error", err))
		return appErrors.Internal(err, "failed to create warehouse")
	}
	return nil
}

func (r *pgDeliveryRepository) CreateOrUpdatePartnerProfile(ctx context.Context, profile *model.DeliveryPartnerProfile) error {
	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}
	now := time.Now().UTC()
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}
	if profile.UpdatedAt.IsZero() {
		profile.UpdatedAt = now
	}
	query := `
		INSERT INTO delivery_partner_profiles (id, partner_id, company_name, email, contact_number, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (partner_id) DO UPDATE SET
			company_name = CASE WHEN EXCLUDED.company_name <> '' THEN EXCLUDED.company_name ELSE delivery_partner_profiles.company_name END,
			email = CASE WHEN EXCLUDED.email <> '' THEN EXCLUDED.email ELSE delivery_partner_profiles.email END,
			contact_number = CASE WHEN EXCLUDED.contact_number <> '' THEN EXCLUDED.contact_number ELSE delivery_partner_profiles.contact_number END,
			is_active = EXCLUDED.is_active,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.db.Pool.Exec(ctx, query, profile.ID, profile.PartnerID, profile.CompanyName, profile.Email, profile.ContactNumber, profile.IsActive, profile.CreatedAt, profile.UpdatedAt)
	if err != nil {
		r.logger.Error("Repository: failed to upsert delivery partner profile", slog.String("partner_id", profile.PartnerID.String()), slog.Any("error", err))
		return appErrors.Internal(err, "failed to upsert partner profile")
	}
	return nil
}

func (r *pgDeliveryRepository) GetPartnerProfile(ctx context.Context, partnerID uuid.UUID) (*model.DeliveryPartnerProfile, error) {
	query := `
		SELECT id, partner_id, company_name, email, contact_number, is_active, created_at, updated_at
		FROM delivery_partner_profiles
		WHERE partner_id = $1
	`
	var p model.DeliveryPartnerProfile
	err := r.db.Pool.QueryRow(ctx, query, partnerID).Scan(
		&p.ID,
		&p.PartnerID,
		&p.CompanyName,
		&p.Email,
		&p.ContactNumber,
		&p.IsActive,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("partner profile not found")
		}
		return nil, appErrors.Internal(err, "failed to get partner profile")
	}
	return &p, nil
}

func (r *pgDeliveryRepository) AssignPartnerWarehouses(ctx context.Context, partnerID uuid.UUID, warehouseIDs []uuid.UUID) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Ensure partner profile exists (create shell profile if not yet created via event)
	var exists bool
	_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM delivery_partner_profiles WHERE partner_id = $1)", partnerID).Scan(&exists)
	if !exists {
		_, err := tx.Exec(ctx, `
			INSERT INTO delivery_partner_profiles (id, partner_id, company_name, is_active, created_at, updated_at)
			VALUES ($1, $2, 'Partner', true, NOW(), NOW())
			ON CONFLICT (partner_id) DO NOTHING
		`, uuid.New(), partnerID)
		if err != nil {
			return appErrors.Internal(err, "failed to create partner profile shell")
		}
	}

	// Delete old mappings
	if _, err := tx.Exec(ctx, "DELETE FROM partner_warehouses WHERE partner_id = $1", partnerID); err != nil {
		return appErrors.Internal(err, "failed to clear existing warehouse associations")
	}

	// Insert new mappings
	for _, wID := range warehouseIDs {
		_, err := tx.Exec(ctx, `
			INSERT INTO partner_warehouses (id, partner_id, warehouse_id, created_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (partner_id, warehouse_id) DO NOTHING
		`, uuid.New(), partnerID, wID)
		if err != nil {
			return appErrors.Internal(err, "failed to associate warehouse with partner")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return appErrors.Internal(err, "failed to commit warehouse assignment")
	}
	return nil
}

func (r *pgDeliveryRepository) GetPartnerWarehouses(ctx context.Context, partnerID uuid.UUID) ([]*model.Warehouse, error) {
	query := `
		SELECT w.id, w.name, w.code, w.street, w.city, w.state, w.postal_code, w.is_active, w.created_at, w.updated_at
		FROM warehouses w
		JOIN partner_warehouses pw ON w.id = pw.warehouse_id
		WHERE pw.partner_id = $1 AND w.is_active = TRUE
		ORDER BY w.name ASC
	`
	rows, err := r.db.Pool.Query(ctx, query, partnerID)
	if err != nil {
		r.logger.Error("Repository: failed to query partner warehouses", slog.String("partner_id", partnerID.String()), slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to query partner warehouses")
	}
	defer rows.Close()

	var warehouses []*model.Warehouse
	for rows.Next() {
		var w model.Warehouse
		if err := rows.Scan(
			&w.ID,
			&w.Name,
			&w.Code,
			&w.Street,
			&w.City,
			&w.State,
			&w.PostalCode,
			&w.IsActive,
			&w.CreatedAt,
			&w.UpdatedAt,
		); err != nil {
			return nil, appErrors.Internal(err, "failed to scan partner warehouse")
		}
		warehouses = append(warehouses, &w)
	}
	if warehouses == nil {
		warehouses = make([]*model.Warehouse, 0)
	}
	return warehouses, nil
}

