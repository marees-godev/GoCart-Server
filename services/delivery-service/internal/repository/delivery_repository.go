package repository

import (
	"context"
	"encoding/json"
	"errors"
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
	UpdateStatusWithDetails(ctx context.Context, id string, status model.DeliveryStatus, estimatedDelivery, actualDelivery *time.Time, location, notes string) (*model.Delivery, error)

	// Warehouse & Partner Profile operations
	GetAvailableWarehouses(ctx context.Context) ([]*model.Warehouse, error)
	GetWarehouseByID(ctx context.Context, partnerID uuid.UUID, id uuid.UUID) (*model.Warehouse, error)
	GetWarehousesByPartnerID(ctx context.Context, partnerID uuid.UUID) ([]*model.Warehouse, error)
	GetPartnerWarehouses(ctx context.Context, partnerID uuid.UUID) ([]*model.Warehouse, error)
	CreateWarehouse(ctx context.Context, w *model.Warehouse) error
	UpdateWarehouse(ctx context.Context, w *model.Warehouse) (*model.Warehouse, error)
	DeleteWarehouse(ctx context.Context, partnerID uuid.UUID, id uuid.UUID) error
	CreateOrUpdatePartnerProfile(ctx context.Context, profile *model.DeliveryPartnerProfile) error
	GetPartnerProfile(ctx context.Context, partnerID uuid.UUID) (*model.DeliveryPartnerProfile, error)
	GetPartnerProfileByCompanyName(ctx context.Context, name string) (*model.DeliveryPartnerProfile, error)
	GetPartnerProfileByGSTIN(ctx context.Context, gstin string) (*model.DeliveryPartnerProfile, error)
	ListAppealedPartners(ctx context.Context, limit, offset int) ([]*model.DeliveryPartnerProfile, int, error)
	ListApprovedPartners(ctx context.Context, limit, offset int) ([]*model.DeliveryPartnerProfile, int, error)
	SoftDeletePartnerProfile(ctx context.Context, partnerID uuid.UUID, deletedAt time.Time, reason ...string) error
	SoftDeleteExpiredDeactivatedPartners(ctx context.Context, cutoff time.Time) (int, error)
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

	var shippingAddrUUID *uuid.UUID
	if strings.TrimSpace(delivery.ShippingAddressID) != "" {
		if u, pErr := uuid.Parse(strings.TrimSpace(delivery.ShippingAddressID)); pErr == nil {
			shippingAddrUUID = &u
		}
	}

	query := `
		INSERT INTO deliveries (
			id, order_id, delivery_partner, tracking_number, status,
			street, city, state, postal_code, country, contact_number, shipping_address,
			shipping_address_id,
			estimated_delivery, actual_delivery,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
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
		shippingAddrUUID,
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
			       shipping_address_id,
			       estimated_delivery, actual_delivery,
			       created_at, updated_at
			FROM deliveries
			WHERE id = $1 OR order_id = $1
		`
	} else {
		query = `
			SELECT id, order_id, delivery_partner, tracking_number, status,
			       street, city, state, postal_code, country, contact_number, shipping_address,
			       shipping_address_id,
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
	r.loadTrackingUpdates(ctx, delivery)
	return delivery, nil
}

func (r *pgDeliveryRepository) GetByOrderID(ctx context.Context, orderID string) (*model.Delivery, error) {
	query := `
		SELECT id, order_id, delivery_partner, tracking_number, status,
		       street, city, state, postal_code, country, contact_number, shipping_address,
		       shipping_address_id,
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
	r.loadTrackingUpdates(ctx, delivery)
	return delivery, nil
}

func (r *pgDeliveryRepository) GetByTrackingNumber(ctx context.Context, trackingNumber string) (*model.Delivery, error) {
	query := `
		SELECT id, order_id, delivery_partner, tracking_number, status,
		       street, city, state, postal_code, country, contact_number, shipping_address,
		       shipping_address_id,
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
	r.loadTrackingUpdates(ctx, delivery)
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
			       shipping_address_id,
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
			       shipping_address_id,
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
	return r.UpdateStatusWithDetails(ctx, id, status, nil, nil, "", "")
}

func (r *pgDeliveryRepository) UpdateStatusWithDetails(ctx context.Context, id string, status model.DeliveryStatus, estimatedDelivery, actualDelivery *time.Time, location, notes string) (*model.Delivery, error) {
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
			       shipping_address_id,
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
			       shipping_address_id,
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
	if !model.IsValidStatus(cleanStatus) {
		r.logger.Warn("Repository: invalid or unknown delivery status rejected", slog.String("status", string(status)))
		return nil, appErrors.BadRequest("invalid or unknown delivery status: " + string(status))
	}

	// Idempotency: safe handling of duplicate status events
	if existing.Status == cleanStatus {
		r.logger.Info("Repository: delivery is already in target status (idempotent)", slog.String("delivery_id", existing.ID), slog.String("status", string(cleanStatus)))
		needsUpdate := false
		targetEst := existing.EstimatedDelivery
		if estimatedDelivery != nil && (existing.EstimatedDelivery == nil || !existing.EstimatedDelivery.Equal(*estimatedDelivery)) {
			targetEst = estimatedDelivery
			needsUpdate = true
		}
		targetAct := existing.ActualDelivery
		if actualDelivery != nil && (existing.ActualDelivery == nil || !existing.ActualDelivery.Equal(*actualDelivery)) {
			targetAct = actualDelivery
			needsUpdate = true
		}

		if needsUpdate {
			queryUpdateDetails := `
				UPDATE deliveries
				SET estimated_delivery = $2, actual_delivery = $3, updated_at = NOW()
				WHERE id = $1
				RETURNING id, order_id, delivery_partner, tracking_number, status,
				          street, city, state, postal_code, country, contact_number, shipping_address,
				          shipping_address_id,
				          estimated_delivery, actual_delivery,
				          created_at, updated_at
			`
			updatedDelivery, updateErr := r.scanOne(tx.QueryRow(ctx, queryUpdateDetails, existing.ID, targetEst, targetAct))
			if updateErr != nil {
				return nil, appErrors.Internal(updateErr, "failed to update delivery timestamps")
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return nil, appErrors.Internal(commitErr, "failed to commit update transaction")
			}
			return updatedDelivery, nil
		}
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

	// Update dates
	targetEst := existing.EstimatedDelivery
	if estimatedDelivery != nil {
		targetEst = estimatedDelivery
	}

	targetAct := existing.ActualDelivery
	if cleanStatus == model.StatusDelivered {
		if actualDelivery != nil {
			targetAct = actualDelivery
		} else if targetAct == nil {
			now := time.Now().UTC()
			targetAct = &now
		}
	} else if actualDelivery != nil {
		targetAct = actualDelivery
	}

	queryUpdate := `
		UPDATE deliveries
		SET status = $2,
		    estimated_delivery = $3,
		    actual_delivery = $4,
		    updated_at = $5
		WHERE id = $1
		RETURNING id, order_id, delivery_partner, tracking_number, status,
		          street, city, state, postal_code, country, contact_number, shipping_address,
		          shipping_address_id,
		          estimated_delivery, actual_delivery,
		          created_at, updated_at
	`
	updatedDelivery, err := r.scanOne(tx.QueryRow(ctx, queryUpdate, existing.ID, cleanStatus, targetEst, targetAct, existing.UpdatedAt))
	if err != nil {
		r.logger.Error("Repository: failed to update delivery status", slog.String("delivery_id", existing.ID), slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to update delivery status")
	}

	// Record tracking history
	_, _ = tx.Exec(ctx, `
		INSERT INTO delivery_tracking_updates (delivery_id, status, location, notes, created_at)
		VALUES ($1, $2, $3, $4, NOW())
	`, existing.ID, cleanStatus, location, notes)

	// Publish DeliveryStatusChanged domain event to transactional outbox
	changedEvt := events.DeliveryStatusChangedEvent{
		DeliveryID:        updatedDelivery.ID,
		OrderID:           updatedDelivery.OrderID,
		PreviousStatus:    string(previousStatus),
		NewStatus:         string(updatedDelivery.Status),
		TrackingNumber:    updatedDelivery.TrackingNumber,
		EstimatedDelivery: updatedDelivery.EstimatedDelivery,
		ActualDelivery:    updatedDelivery.ActualDelivery,
		UpdatedAt:         updatedDelivery.UpdatedAt,
	}
	changedEnvelope, err := events.NewEventEnvelopeWithAggregate(events.EventTypeDeliveryStatusChanged, "delivery-service", updatedDelivery.ID, changedEvt)
	if err != nil {
		r.logger.Error("Repository: failed to construct DeliveryStatusChanged envelope", slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to construct event envelope")
	}
	changedPayloadBytes, err := changedEnvelope.Marshal()
	if err != nil {
		r.logger.Error("Repository: failed to marshal DeliveryStatusChanged payload", slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to marshal event payload")
	}

	outboxChangedEvt := &outbox.Event{
		AggregateType: "delivery",
		AggregateID:   updatedDelivery.ID,
		EventType:     events.EventTypeDeliveryStatusChanged,
		Payload:       changedPayloadBytes,
		Topic:         events.TopicDeliveryStatusChanged,
	}
	if err := r.outboxStore.Insert(ctx, tx, outboxChangedEvt); err != nil {
		r.logger.Error("Repository: failed to write DeliveryStatusChanged event to outbox", slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to record outbox event")
	}

	// Also record DeliveryStatusUpdated for backward compatibility
	updatedEvt := events.DeliveryStatusUpdatedEvent{
		DeliveryID:     updatedDelivery.ID,
		OrderID:        updatedDelivery.OrderID,
		PreviousStatus: string(previousStatus),
		NewStatus:      string(updatedDelivery.Status),
		TrackingNumber: updatedDelivery.TrackingNumber,
		UpdatedAt:      updatedDelivery.UpdatedAt,
	}
	envelope, err := events.NewEventEnvelopeWithAggregate(events.EventTypeDeliveryStatusUpdated, "delivery-service", updatedDelivery.ID, updatedEvt)
	if err == nil {
		if payloadBytes, pErr := envelope.Marshal(); pErr == nil {
			_ = r.outboxStore.Insert(ctx, tx, &outbox.Event{
				AggregateType: "delivery",
				AggregateID:   updatedDelivery.ID,
				EventType:     events.EventTypeDeliveryStatusUpdated,
				Payload:       payloadBytes,
				Topic:         events.TopicDeliveryStatusUpdated,
			})
		}
	}

	// Publish OrderShipped when advancing to PICKED_UP or IN_TRANSIT
	if cleanStatus == model.StatusPickedUp || cleanStatus == model.StatusInTransit {
		shippedEvt := events.OrderShippedEvent{
			OrderID:         updatedDelivery.OrderID,
			DeliveryID:      updatedDelivery.ID,
			TrackingNumber:  updatedDelivery.TrackingNumber,
			DeliveryPartner: updatedDelivery.DeliveryPartner,
			Carrier:         updatedDelivery.DeliveryPartner,
			ShippedAt:       time.Now().UTC(),
		}
		if shEnv, shErr := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderShipped, "delivery-service", updatedDelivery.ID, shippedEvt); shErr == nil {
			if shBytes, mErr := shEnv.Marshal(); mErr == nil {
				_ = r.outboxStore.Insert(ctx, tx, &outbox.Event{
					AggregateType: "order",
					AggregateID:   updatedDelivery.OrderID,
					EventType:     events.EventTypeOrderShipped,
					Payload:       shBytes,
					Topic:         events.TopicOrderShipped,
				})
			}
		}
	}

	// Publish OrderDelivered when reaching DELIVERED
	if cleanStatus == model.StatusDelivered {
		deliveredEvt := events.OrderDeliveredEvent{
			OrderID:         updatedDelivery.OrderID,
			DeliveryID:      updatedDelivery.ID,
			TrackingNumber:  updatedDelivery.TrackingNumber,
			DeliveryPartner: updatedDelivery.DeliveryPartner,
			DeliveredAt:     time.Now().UTC(),
		}
		if delEnv, delErr := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderDelivered, "delivery-service", updatedDelivery.ID, deliveredEvt); delErr == nil {
			if delBytes, mErr := delEnv.Marshal(); mErr == nil {
				_ = r.outboxStore.Insert(ctx, tx, &outbox.Event{
					AggregateType: "order",
					AggregateID:   updatedDelivery.OrderID,
					EventType:     events.EventTypeOrderDelivered,
					Payload:       delBytes,
					Topic:         events.TopicOrderDelivered,
				})
			}
		}
	}

	// Publish OrderCancelled when transitioning to CANCELLED
	if cleanStatus == model.StatusCancelled {
		cancelReason := notes
		if strings.TrimSpace(cancelReason) == "" {
			cancelReason = "Delivery cancelled"
		}
		cancelledEvt := events.OrderCancelledEvent{
			OrderID:     updatedDelivery.OrderID,
			Reason:      cancelReason,
			CancelledAt: time.Now().UTC(),
		}
		if canEnv, canErr := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderCancelled, "delivery-service", updatedDelivery.ID, cancelledEvt); canErr == nil {
			if canBytes, mErr := canEnv.Marshal(); mErr == nil {
				_ = r.outboxStore.Insert(ctx, tx, &outbox.Event{
					AggregateType: "order",
					AggregateID:   updatedDelivery.OrderID,
					EventType:     events.EventTypeOrderCancelled,
					Payload:       canBytes,
					Topic:         events.TopicOrderCancelled,
				})
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		r.logger.Error("Repository: failed to commit update delivery status transaction", slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to commit transaction")
	}

	r.logger.Info("Repository: delivery status updated successfully", slog.String("delivery_id", existing.ID), slog.String("new_status", string(cleanStatus)))
	return updatedDelivery, nil
}

func (r *pgDeliveryRepository) loadTrackingUpdates(ctx context.Context, delivery *model.Delivery) {
	if delivery == nil || delivery.ID == "" {
		return
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, delivery_id, status, COALESCE(location, ''), COALESCE(notes, ''), created_at
		FROM delivery_tracking_updates
		WHERE delivery_id = $1
		ORDER BY created_at ASC
	`, delivery.ID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var u model.TrackingUpdate
		if err := rows.Scan(&u.ID, &u.DeliveryID, &u.Status, &u.Location, &u.Notes, &u.CreatedAt); err == nil {
			delivery.TrackingUpdates = append(delivery.TrackingUpdates, u)
		}
	}
}

func (r *pgDeliveryRepository) scanOne(row pgx.Row) (*model.Delivery, error) {
	var d model.Delivery
	var street, city, state, postalCode, country, contactNumber string
	var addrBytes []byte
	var shippingAddrUUID *uuid.UUID

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
		&shippingAddrUUID,
		&d.EstimatedDelivery,
		&d.ActualDelivery,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if shippingAddrUUID != nil {
		d.ShippingAddressID = shippingAddrUUID.String()
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

func (r *pgDeliveryRepository) GetAvailableWarehouses(ctx context.Context) ([]*model.Warehouse, error) {
	query := `
		SELECT id, delivery_partner_id, name, code, street, city, state, postal_code, is_active, created_at, updated_at
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
			&w.DeliveryPartnerID,
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

func (r *pgDeliveryRepository) GetWarehouseByID(ctx context.Context, partnerID uuid.UUID, id uuid.UUID) (*model.Warehouse, error) {
	if partnerID == uuid.Nil {
		return nil, appErrors.BadRequest("delivery_partner_id is mandatory to proceed")
	}
	if id == uuid.Nil {
		return nil, appErrors.BadRequest("warehouse id is required")
	}
	query := `
		SELECT id, delivery_partner_id, name, code, street, city, state, postal_code, is_active, created_at, updated_at
		FROM warehouses
		WHERE id = $1 AND delivery_partner_id = $2
	`
	var w model.Warehouse
	err := r.db.Pool.QueryRow(ctx, query, id, partnerID).Scan(
		&w.ID,
		&w.DeliveryPartnerID,
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

func (r *pgDeliveryRepository) GetWarehousesByPartnerID(ctx context.Context, partnerID uuid.UUID) ([]*model.Warehouse, error) {
	if partnerID == uuid.Nil {
		return nil, appErrors.BadRequest("delivery_partner_id is mandatory to proceed")
	}
	query := `
		SELECT id, delivery_partner_id, name, code, street, city, state, postal_code, is_active, created_at, updated_at
		FROM warehouses
		WHERE delivery_partner_id = $1
		ORDER BY name ASC
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
			&w.DeliveryPartnerID,
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

func (r *pgDeliveryRepository) GetPartnerWarehouses(ctx context.Context, partnerID uuid.UUID) ([]*model.Warehouse, error) {
	return r.GetWarehousesByPartnerID(ctx, partnerID)
}

func (r *pgDeliveryRepository) CreateWarehouse(ctx context.Context, w *model.Warehouse) error {
	if w.DeliveryPartnerID == uuid.Nil {
		return appErrors.BadRequest("delivery_partner_id is mandatory to proceed")
	}
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
		INSERT INTO warehouses (id, delivery_partner_id, name, code, street, city, state, postal_code, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := r.db.Pool.Exec(ctx, query, w.ID, w.DeliveryPartnerID, w.Name, w.Code, w.Street, w.City, w.State, w.PostalCode, w.IsActive, w.CreatedAt, w.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return appErrors.Conflict("warehouse code already exists for this delivery partner")
		}
		r.logger.Error("Repository: failed to create warehouse", slog.Any("error", err))
		return appErrors.Internal(err, "failed to create warehouse")
	}
	return nil
}

func (r *pgDeliveryRepository) UpdateWarehouse(ctx context.Context, w *model.Warehouse) (*model.Warehouse, error) {
	if w.DeliveryPartnerID == uuid.Nil {
		return nil, appErrors.BadRequest("delivery_partner_id is mandatory to proceed")
	}
	if w.ID == uuid.Nil {
		return nil, appErrors.BadRequest("warehouse id is required")
	}
	w.UpdatedAt = time.Now().UTC()
	query := `
		UPDATE warehouses
		SET name = $1, code = $2, street = $3, city = $4, state = $5, postal_code = $6, is_active = $7, updated_at = $8
		WHERE id = $9 AND delivery_partner_id = $10
		RETURNING id, delivery_partner_id, name, code, street, city, state, postal_code, is_active, created_at, updated_at
	`
	var updated model.Warehouse
	err := r.db.Pool.QueryRow(ctx, query,
		w.Name, w.Code, w.Street, w.City, w.State, w.PostalCode, w.IsActive, w.UpdatedAt,
		w.ID, w.DeliveryPartnerID,
	).Scan(
		&updated.ID,
		&updated.DeliveryPartnerID,
		&updated.Name,
		&updated.Code,
		&updated.Street,
		&updated.City,
		&updated.State,
		&updated.PostalCode,
		&updated.IsActive,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("warehouse not found")
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, appErrors.Conflict("warehouse code already exists for this delivery partner")
		}
		r.logger.Error("Repository: failed to update warehouse", slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to update warehouse")
	}
	return &updated, nil
}

func (r *pgDeliveryRepository) DeleteWarehouse(ctx context.Context, partnerID uuid.UUID, id uuid.UUID) error {
	if partnerID == uuid.Nil {
		return appErrors.BadRequest("delivery_partner_id is mandatory to proceed")
	}
	if id == uuid.Nil {
		return appErrors.BadRequest("warehouse id is required")
	}
	query := `DELETE FROM warehouses WHERE id = $1 AND delivery_partner_id = $2`
	tag, err := r.db.Pool.Exec(ctx, query, id, partnerID)
	if err != nil {
		r.logger.Error("Repository: failed to delete warehouse", slog.Any("error", err))
		return appErrors.Internal(err, "failed to delete warehouse")
	}
	if tag.RowsAffected() == 0 {
		return appErrors.NotFound("warehouse not found")
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
	if profile.ApprovalStatus == "" {
		profile.ApprovalStatus = model.ApprovalStatusPending
	}
	query := `
		INSERT INTO delivery_partner_profiles (
			id, partner_id, company_name, email, contact_number, gstin, is_gstin_verified, is_active,
			approval_status, rejection_reason, appeal_reason, deactivation_reason, deletion_reason, appealed_at, deactivated_at, deleted_at,
			created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (partner_id) DO UPDATE SET
			company_name = CASE WHEN EXCLUDED.company_name <> '' THEN EXCLUDED.company_name ELSE delivery_partner_profiles.company_name END,
			email = CASE WHEN EXCLUDED.email <> '' THEN EXCLUDED.email ELSE delivery_partner_profiles.email END,
			contact_number = CASE WHEN EXCLUDED.contact_number <> '' THEN EXCLUDED.contact_number ELSE delivery_partner_profiles.contact_number END,
			gstin = CASE WHEN EXCLUDED.gstin IS NOT NULL THEN EXCLUDED.gstin ELSE delivery_partner_profiles.gstin END,
			is_gstin_verified = EXCLUDED.is_gstin_verified,
			is_active = EXCLUDED.is_active,
			approval_status = EXCLUDED.approval_status,
			rejection_reason = EXCLUDED.rejection_reason,
			appeal_reason = EXCLUDED.appeal_reason,
			deactivation_reason = EXCLUDED.deactivation_reason,
			deletion_reason = EXCLUDED.deletion_reason,
			appealed_at = EXCLUDED.appealed_at,
			deactivated_at = EXCLUDED.deactivated_at,
			deleted_at = EXCLUDED.deleted_at,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.db.Pool.Exec(ctx, query,
		profile.ID, profile.PartnerID, profile.CompanyName, profile.Email, profile.ContactNumber, profile.GSTIN, profile.IsGSTINVerified, profile.IsActive,
		profile.ApprovalStatus, profile.RejectionReason, profile.AppealReason, profile.DeactivationReason, profile.DeletionReason, profile.AppealedAt, profile.DeactivatedAt, profile.DeletedAt,
		profile.CreatedAt, profile.UpdatedAt,
	)
	if err != nil {
		r.logger.Error("Repository: failed to upsert delivery partner profile", slog.String("partner_id", profile.PartnerID.String()), slog.Any("error", err))
		return appErrors.Internal(err, "failed to upsert partner profile")
	}
	return nil
}

func (r *pgDeliveryRepository) GetPartnerProfile(ctx context.Context, partnerID uuid.UUID) (*model.DeliveryPartnerProfile, error) {
	query := `
		SELECT id, partner_id, company_name, email, contact_number, gstin, is_gstin_verified, is_active,
		       approval_status, rejection_reason, appeal_reason, deactivation_reason, deletion_reason, appealed_at, deactivated_at, deleted_at,
		       created_at, updated_at
		FROM delivery_partner_profiles
		WHERE partner_id = $1 OR id = $1
	`
	var p model.DeliveryPartnerProfile
	err := r.db.Pool.QueryRow(ctx, query, partnerID).Scan(
		&p.ID,
		&p.PartnerID,
		&p.CompanyName,
		&p.Email,
		&p.ContactNumber,
		&p.GSTIN,
		&p.IsGSTINVerified,
		&p.IsActive,
		&p.ApprovalStatus,
		&p.RejectionReason,
		&p.AppealReason,
		&p.DeactivationReason,
		&p.DeletionReason,
		&p.AppealedAt,
		&p.DeactivatedAt,
		&p.DeletedAt,
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

func (r *pgDeliveryRepository) GetPartnerProfileByCompanyName(ctx context.Context, name string) (*model.DeliveryPartnerProfile, error) {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return nil, appErrors.BadRequest("company name is required")
	}
	query := `
		SELECT id, partner_id, company_name, email, contact_number, gstin, is_gstin_verified, is_active,
		       approval_status, rejection_reason, appeal_reason, deactivation_reason, deletion_reason, appealed_at, deactivated_at, deleted_at,
		       created_at, updated_at
		FROM delivery_partner_profiles
		WHERE LOWER(company_name) = LOWER($1)
		LIMIT 1
	`
	var p model.DeliveryPartnerProfile
	err := r.db.Pool.QueryRow(ctx, query, clean).Scan(
		&p.ID,
		&p.PartnerID,
		&p.CompanyName,
		&p.Email,
		&p.ContactNumber,
		&p.GSTIN,
		&p.IsGSTINVerified,
		&p.IsActive,
		&p.ApprovalStatus,
		&p.RejectionReason,
		&p.AppealReason,
		&p.DeactivationReason,
		&p.DeletionReason,
		&p.AppealedAt,
		&p.DeactivatedAt,
		&p.DeletedAt,
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

func (r *pgDeliveryRepository) GetPartnerProfileByGSTIN(ctx context.Context, gstin string) (*model.DeliveryPartnerProfile, error) {
	clean := strings.ToUpper(strings.TrimSpace(gstin))
	if clean == "" {
		return nil, appErrors.BadRequest("GSTIN is required")
	}
	query := `
		SELECT id, partner_id, company_name, email, contact_number, gstin, is_gstin_verified, is_active,
		       approval_status, rejection_reason, appeal_reason, deactivation_reason, deletion_reason, appealed_at, deactivated_at, deleted_at,
		       created_at, updated_at
		FROM delivery_partner_profiles
		WHERE UPPER(gstin) = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`
	var p model.DeliveryPartnerProfile
	err := r.db.Pool.QueryRow(ctx, query, clean).Scan(
		&p.ID,
		&p.PartnerID,
		&p.CompanyName,
		&p.Email,
		&p.ContactNumber,
		&p.GSTIN,
		&p.IsGSTINVerified,
		&p.IsActive,
		&p.ApprovalStatus,
		&p.RejectionReason,
		&p.AppealReason,
		&p.DeactivationReason,
		&p.DeletionReason,
		&p.AppealedAt,
		&p.DeactivatedAt,
		&p.DeletedAt,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("delivery partner profile not found")
		}
		return nil, appErrors.Internal(err, "failed to get delivery partner profile by GSTIN")
	}
	return &p, nil
}

func (r *pgDeliveryRepository) ListAppealedPartners(ctx context.Context, limit, offset int) ([]*model.DeliveryPartnerProfile, int, error) {
	countQuery := `
		SELECT COUNT(*)
		FROM delivery_partner_profiles
		WHERE approval_status = 'APPEALED'
		  AND deleted_at IS NULL
	`
	var total int
	if err := r.db.Pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		r.logger.Error("Repository: failed to count appealed delivery partners", slog.Any("error", err))
		return nil, 0, appErrors.Internal(err, "failed to count appealed delivery partners")
	}

	query := `
		SELECT id, partner_id, company_name, email, contact_number, gstin, is_gstin_verified, is_active,
		       approval_status, rejection_reason, appeal_reason, deactivation_reason, deletion_reason, appealed_at, deactivated_at, deleted_at,
		       created_at, updated_at
		FROM delivery_partner_profiles
		WHERE approval_status = 'APPEALED'
		  AND deleted_at IS NULL
		ORDER BY appealed_at DESC, created_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := r.db.Pool.Query(ctx, query, limit, offset)
	if err != nil {
		r.logger.Error("Repository: failed to list appealed delivery partners", slog.Any("error", err))
		return nil, 0, appErrors.Internal(err, "failed to list appealed delivery partners")
	}
	defer rows.Close()

	var profiles []*model.DeliveryPartnerProfile
	for rows.Next() {
		var p model.DeliveryPartnerProfile
		if err := rows.Scan(
			&p.ID,
			&p.PartnerID,
			&p.CompanyName,
			&p.Email,
			&p.ContactNumber,
			&p.GSTIN,
			&p.IsGSTINVerified,
			&p.IsActive,
			&p.ApprovalStatus,
			&p.RejectionReason,
			&p.AppealReason,
			&p.DeactivationReason,
			&p.DeletionReason,
			&p.AppealedAt,
			&p.DeactivatedAt,
			&p.DeletedAt,
			&p.CreatedAt,
			&p.UpdatedAt,
		); err != nil {
			r.logger.Error("Repository: failed to scan appealed delivery partner", slog.Any("error", err))
			return nil, 0, appErrors.Internal(err, "failed to scan delivery partner profile")
		}
		profiles = append(profiles, &p)
	}

	return profiles, total, nil
}

func (r *pgDeliveryRepository) ListApprovedPartners(ctx context.Context, limit, offset int) ([]*model.DeliveryPartnerProfile, int, error) {
	countQuery := `
		SELECT COUNT(*)
		FROM delivery_partner_profiles
		WHERE approval_status = 'APPROVED'
		  AND is_active = TRUE
		  AND deleted_at IS NULL
	`
	var total int
	if err := r.db.Pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		r.logger.Error("Repository: failed to count approved delivery partners", slog.Any("error", err))
		return nil, 0, appErrors.Internal(err, "failed to count approved delivery partners")
	}

	query := `
		SELECT id, partner_id, company_name, email, contact_number, gstin, is_gstin_verified, is_active,
		       approval_status, rejection_reason, appeal_reason, deactivation_reason, deletion_reason, appealed_at, deactivated_at, deleted_at,
		       created_at, updated_at
		FROM delivery_partner_profiles
		WHERE approval_status = 'APPROVED'
		  AND is_active = TRUE
		  AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := r.db.Pool.Query(ctx, query, limit, offset)
	if err != nil {
		r.logger.Error("Repository: failed to list approved delivery partners", slog.Any("error", err))
		return nil, 0, appErrors.Internal(err, "failed to list approved delivery partners")
	}
	defer rows.Close()

	var profiles []*model.DeliveryPartnerProfile
	for rows.Next() {
		var p model.DeliveryPartnerProfile
		if err := rows.Scan(
			&p.ID,
			&p.PartnerID,
			&p.CompanyName,
			&p.Email,
			&p.ContactNumber,
			&p.GSTIN,
			&p.IsGSTINVerified,
			&p.IsActive,
			&p.ApprovalStatus,
			&p.RejectionReason,
			&p.AppealReason,
			&p.DeactivationReason,
			&p.DeletionReason,
			&p.AppealedAt,
			&p.DeactivatedAt,
			&p.DeletedAt,
			&p.CreatedAt,
			&p.UpdatedAt,
		); err != nil {
			r.logger.Error("Repository: failed to scan approved delivery partner", slog.Any("error", err))
			return nil, 0, appErrors.Internal(err, "failed to scan delivery partner profile")
		}
		profiles = append(profiles, &p)
	}

	return profiles, total, nil
}

func (r *pgDeliveryRepository) SoftDeletePartnerProfile(ctx context.Context, partnerID uuid.UUID, deletedAt time.Time, reason ...string) error {
	var cleanReason *string
	if len(reason) > 0 && strings.TrimSpace(reason[0]) != "" {
		cr := strings.TrimSpace(reason[0])
		cleanReason = &cr
	}

	query := `
		UPDATE delivery_partner_profiles
		SET deleted_at = $1, is_active = FALSE, updated_at = $1, deletion_reason = $3
		WHERE (partner_id = $2 OR id = $2) AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query, deletedAt, partnerID, cleanReason)
	if err != nil {
		r.logger.Error("Repository: failed to soft delete delivery partner profile", slog.String("partner_id", partnerID.String()), slog.Any("error", err))
		return appErrors.Internal(err, "failed to delete partner profile")
	}
	if tag.RowsAffected() == 0 {
		return appErrors.NotFound("delivery partner profile not found")
	}
	return nil
}

func (r *pgDeliveryRepository) SoftDeleteExpiredDeactivatedPartners(ctx context.Context, cutoff time.Time) (int, error) {
	query := `
		UPDATE delivery_partner_profiles
		SET deleted_at = NOW(), is_active = FALSE, updated_at = NOW()
		WHERE is_active = FALSE
		  AND deactivated_at IS NOT NULL
		  AND deactivated_at <= $1
		  AND deleted_at IS NULL
	`
	tag, err := r.db.Pool.Exec(ctx, query, cutoff)
	if err != nil {
		r.logger.Error("Repository: failed to process expired deactivations", slog.Any("error", err))
		return 0, appErrors.Internal(err, "failed to process expired deactivations")
	}
	return int(tag.RowsAffected()), nil
}
