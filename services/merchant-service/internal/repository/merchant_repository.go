package repository

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
)

type MerchantRepository interface {
	Create(ctx context.Context, merchant *model.Merchant) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Merchant, error)
	List(ctx context.Context, limit, offset int, status string) ([]*model.Merchant, int, error)
	ListReactivated(ctx context.Context, limit, offset int) ([]*model.Merchant, int, error)
	Update(ctx context.Context, merchant *model.Merchant) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, rejectionReason string) (*model.Merchant, error)
	UpdateStatusWithAudit(ctx context.Context, id uuid.UUID, newStatus model.MerchantStatus, reason string, updatedBy string) (*model.Merchant, model.MerchantStatus, error)
	ExecuteLifecycleTransition(ctx context.Context, merchantID uuid.UUID, action model.LifecycleAction, reason string, adminID string, reqID string) (*model.Merchant, model.MerchantStatus, error)
	RecordLifecycleAudit(ctx context.Context, audit *model.MerchantLifecycleAudit) error
	CreateAppeal(ctx context.Context, appeal *model.MerchantAppeal) error
	GetAppealsByMerchantID(ctx context.Context, merchantID uuid.UUID) ([]*model.MerchantAppeal, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type pgMerchantRepository struct {
	db          *database.DB
	outboxStore *outbox.Store
	logger      *slog.Logger
}

func NewMerchantRepository(db *database.DB, log ...*slog.Logger) MerchantRepository {
	return NewMerchantRepositoryWithOutbox(db, outbox.NewStore(), log...)
}

func NewMerchantRepositoryWithOutbox(db *database.DB, outboxStore *outbox.Store, log ...*slog.Logger) MerchantRepository {
	var l *slog.Logger
	if len(log) > 0 && log[0] != nil {
		l = log[0]
	} else {
		l = slog.Default()
	}
	if outboxStore == nil {
		outboxStore = outbox.NewStore()
	}
	return &pgMerchantRepository{
		db:          db,
		outboxStore: outboxStore,
		logger:      l,
	}
}

func (r *pgMerchantRepository) Create(ctx context.Context, merchant *model.Merchant) error {
	if merchant.ID == uuid.Nil {
		merchant.ID = uuid.New()
	}

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		r.logger.Error("Repository: failed to begin transaction for merchant creation", slog.Any("error", err))
		return appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	query := `
		INSERT INTO merchants (id, business_name, first_name, last_name, business_email, business_phone, pan_card_number, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
		RETURNING id, created_at, updated_at
	`
	err = tx.QueryRow(ctx, query,
		merchant.ID,
		merchant.BusinessName,
		merchant.FirstName,
		merchant.LastName,
		merchant.BusinessEmail,
		merchant.BusinessPhone,
		merchant.PanCardNumber,
		merchant.Status,
	).Scan(&merchant.ID, &merchant.CreatedAt, &merchant.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Info("Repository: merchant record already exists, fetching existing", slog.String("merchant_id", merchant.ID.String()))
			existing, getErr := r.GetByID(ctx, merchant.ID)
			if getErr != nil {
				return getErr
			}
			*merchant = *existing
			return nil
		}
		r.logger.Error("Repository: failed to insert merchant", slog.String("merchant_id", merchant.ID.String()), slog.Any("error", err))
		return appErrors.Internal(err, "failed to create merchant")
	}

	// Transactional Outbox: Persist MerchantRegistered event in outbox inside same DB transaction
	regEvt := events.MerchantRegisteredEvent{
		MerchantID:   merchant.ID.String(),
		BusinessName: merchant.BusinessName,
		FirstName:    merchant.FirstName,
		LastName:     merchant.LastName,
		Email:        merchant.BusinessEmail,
		Phone:        merchant.BusinessPhone,
		CreatedAt:    merchant.CreatedAt,
		RegisteredAt: merchant.CreatedAt,
	}
	envelope, err := events.NewEventEnvelopeWithAggregate(events.EventTypeMerchantRegistered, "merchant-service", merchant.ID.String(), regEvt)
	if err != nil {
		r.logger.Error("Repository: failed to construct MerchantRegistered envelope", slog.Any("error", err))
		return appErrors.Internal(err, "failed to construct event envelope")
	}
	payloadBytes, err := envelope.Marshal()
	if err != nil {
		r.logger.Error("Repository: failed to marshal MerchantRegistered payload", slog.Any("error", err))
		return appErrors.Internal(err, "failed to marshal event payload")
	}

	outboxEvt := &outbox.Event{
		AggregateType: "merchant",
		AggregateID:   merchant.ID.String(),
		EventType:     events.EventTypeMerchantRegistered,
		Payload:       payloadBytes,
		Topic:         events.TopicMerchantRegistered,
	}
	if err := r.outboxStore.Insert(ctx, tx, outboxEvt); err != nil {
		r.logger.Error("Repository: failed to write MerchantRegistered event to outbox", slog.Any("error", err))
		return appErrors.Internal(err, "failed to record outbox event")
	}

	if err := tx.Commit(ctx); err != nil {
		r.logger.Error("Repository: failed to commit create merchant transaction", slog.Any("error", err))
		return appErrors.Internal(err, "failed to commit transaction")
	}

	r.logger.Debug("Repository: merchant and outbox event inserted successfully", slog.String("merchant_id", merchant.ID.String()))
	return nil
}

func (r *pgMerchantRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Merchant, error) {
	query := `
		SELECT id, business_name, first_name, last_name, business_email, business_phone, pan_card_number, status, rejection_reason, created_at, updated_at, deleted_at
		FROM merchants
		WHERE id = $1 AND deleted_at IS NULL
	`
	var m model.Merchant
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&m.ID,
		&m.BusinessName,
		&m.FirstName,
		&m.LastName,
		&m.BusinessEmail,
		&m.BusinessPhone,
		&m.PanCardNumber,
		&m.Status,
		&m.RejectionReason,
		&m.CreatedAt,
		&m.UpdatedAt,
		&m.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Warn("Repository: merchant not found by id", slog.String("merchant_id", id.String()))
			return nil, appErrors.NotFound("merchant not found")
		}
		r.logger.Error("Repository: failed to query merchant by id", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to query merchant by id")
	}
	return &m, nil
}

func (r *pgMerchantRepository) List(ctx context.Context, limit, offset int, status string) ([]*model.Merchant, int, error) {
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
		countQuery = `SELECT COUNT(*) FROM merchants WHERE (status::text = $1 OR (lifecycle_status IS NOT NULL AND lifecycle_status::text = $1)) AND deleted_at IS NULL`
		countArgs = append(countArgs, status)
		listQuery = `
			SELECT id, business_name, first_name, last_name, business_email, business_phone, pan_card_number, status, rejection_reason, created_at, updated_at, deleted_at
			FROM merchants
			WHERE (status::text = $1 OR (lifecycle_status IS NOT NULL AND lifecycle_status::text = $1)) AND deleted_at IS NULL
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`
		args = append(args, status, limit, offset)
	} else {
		countQuery = `SELECT COUNT(*) FROM merchants WHERE deleted_at IS NULL`
		listQuery = `
			SELECT id, business_name, first_name, last_name, business_email, business_phone, pan_card_number, status, rejection_reason, created_at, updated_at, deleted_at
			FROM merchants
			WHERE deleted_at IS NULL
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`
		args = append(args, limit, offset)
	}

	var total int
	err := r.db.Pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		r.logger.Error("Repository: failed to count merchants", slog.Any("error", err))
		return nil, 0, appErrors.Internal(err, "failed to count merchants")
	}

	rows, err := r.db.Pool.Query(ctx, listQuery, args...)
	if err != nil {
		r.logger.Error("Repository: failed to list merchants", slog.Any("error", err))
		return nil, 0, appErrors.Internal(err, "failed to list merchants")
	}
	defer rows.Close()

	merchants := make([]*model.Merchant, 0)
	for rows.Next() {
		var m model.Merchant
		if err := rows.Scan(
			&m.ID,
			&m.BusinessName,
			&m.FirstName,
			&m.LastName,
			&m.BusinessEmail,
			&m.BusinessPhone,
			&m.PanCardNumber,
			&m.Status,
			&m.RejectionReason,
			&m.CreatedAt,
			&m.UpdatedAt,
			&m.DeletedAt,
		); err != nil {
			r.logger.Error("Repository: failed to scan merchant row", slog.Any("error", err))
			return nil, 0, appErrors.Internal(err, "failed to scan merchant row")
		}
		merchants = append(merchants, &m)
	}

	if err := rows.Err(); err != nil {
		r.logger.Error("Repository: error iterating merchant rows", slog.Any("error", err))
		return nil, 0, appErrors.Internal(err, "error iterating merchant rows")
	}

	return merchants, total, nil
}

func (r *pgMerchantRepository) ListReactivated(ctx context.Context, limit, offset int) ([]*model.Merchant, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	countQuery := `
		SELECT COUNT(DISTINCT m.id)
		FROM merchants m
		WHERE (m.status = 'APPROVED' OR m.status = 'ACTIVE')
		  AND m.deleted_at IS NULL
		  AND (
		    EXISTS (
		      SELECT 1 FROM merchant_lifecycle_audit a
		      WHERE a.merchant_id = m.id AND (a.action = 'REACTIVATE' OR (a.previous_status = 'SUSPENDED' AND a.new_status IN ('APPROVED', 'ACTIVE', 'PENDING')))
		    )
		    OR EXISTS (
		      SELECT 1 FROM merchant_appeals ap
		      WHERE ap.merchant_id = m.id
		    )
		  )
	`
	var total int
	err := r.db.Pool.QueryRow(ctx, countQuery).Scan(&total)
	if err != nil {
		r.logger.Error("Repository: failed to count reactivated merchants", slog.Any("error", err))
		return nil, 0, appErrors.Internal(err, "failed to count reactivated merchants")
	}

	listQuery := `
		SELECT DISTINCT m.id, m.business_name, m.first_name, m.last_name, m.business_email, m.business_phone, m.pan_card_number, m.status, m.rejection_reason, m.created_at, m.updated_at, m.deleted_at
		FROM merchants m
		WHERE (m.status = 'APPROVED' OR m.status = 'ACTIVE')
		  AND m.deleted_at IS NULL
		  AND (
		    EXISTS (
		      SELECT 1 FROM merchant_lifecycle_audit a
		      WHERE a.merchant_id = m.id AND (a.action = 'REACTIVATE' OR (a.previous_status = 'SUSPENDED' AND a.new_status IN ('APPROVED', 'ACTIVE', 'PENDING')))
		    )
		    OR EXISTS (
		      SELECT 1 FROM merchant_appeals ap
		      WHERE ap.merchant_id = m.id
		    )
		  )
		ORDER BY m.created_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := r.db.Pool.Query(ctx, listQuery, limit, offset)
	if err != nil {
		r.logger.Error("Repository: failed to list reactivated merchants", slog.Any("error", err))
		return nil, 0, appErrors.Internal(err, "failed to list reactivated merchants")
	}
	defer rows.Close()

	merchants := make([]*model.Merchant, 0)
	for rows.Next() {
		var m model.Merchant
		if err := rows.Scan(
			&m.ID,
			&m.BusinessName,
			&m.FirstName,
			&m.LastName,
			&m.BusinessEmail,
			&m.BusinessPhone,
			&m.PanCardNumber,
			&m.Status,
			&m.RejectionReason,
			&m.CreatedAt,
			&m.UpdatedAt,
			&m.DeletedAt,
		); err != nil {
			r.logger.Error("Repository: failed to scan reactivated merchant row", slog.Any("error", err))
			return nil, 0, appErrors.Internal(err, "failed to scan reactivated merchant row")
		}
		merchants = append(merchants, &m)
	}

	if err := rows.Err(); err != nil {
		r.logger.Error("Repository: error iterating reactivated merchant rows", slog.Any("error", err))
		return nil, 0, appErrors.Internal(err, "error iterating reactivated merchant rows")
	}

	return merchants, total, nil
}

func (r *pgMerchantRepository) Update(ctx context.Context, merchant *model.Merchant) error {
	query := `
		UPDATE merchants
		SET business_name = $1, first_name = $2, last_name = $3, business_phone = $4, pan_card_number = $5, updated_at = NOW()
		WHERE id = $6 AND deleted_at IS NULL
		RETURNING updated_at
	`
	err := r.db.Pool.QueryRow(ctx, query,
		merchant.BusinessName,
		merchant.FirstName,
		merchant.LastName,
		merchant.BusinessPhone,
		merchant.PanCardNumber,
		merchant.ID,
	).Scan(&merchant.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Warn("Repository: merchant not found for update", slog.String("merchant_id", merchant.ID.String()))
			return appErrors.NotFound("merchant not found")
		}
		r.logger.Error("Repository: failed to update merchant", slog.String("merchant_id", merchant.ID.String()), slog.Any("error", err))
		return appErrors.Internal(err, "failed to update merchant")
	}
	r.logger.Debug("Repository: merchant updated successfully", slog.String("merchant_id", merchant.ID.String()))
	return nil
}

func (r *pgMerchantRepository) UpdateStatusWithAudit(ctx context.Context, id uuid.UUID, newStatus model.MerchantStatus, reason string, updatedBy string) (*model.Merchant, model.MerchantStatus, error) {
	if updatedBy == "" {
		updatedBy = "ADMIN"
	}

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		r.logger.Error("Repository: failed to begin transaction for status update", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, "", appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatusStr string
	queryCurrent := `
		SELECT status
		FROM merchants
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`
	err = tx.QueryRow(ctx, queryCurrent, id).Scan(&currentStatusStr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Warn("Repository: merchant not found for status update", slog.String("merchant_id", id.String()))
			return nil, "", appErrors.NotFound("merchant not found")
		}
		r.logger.Error("Repository: failed to query merchant status", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, "", appErrors.Internal(err, "failed to query merchant status")
	}

	currentStatus := model.MerchantStatus(currentStatusStr)

	validator := model.NewStateTransitionValidator()
	if err := validator.Validate(currentStatus, newStatus); err != nil {
		r.logger.Warn("Repository: invalid state transition",
			slog.String("merchant_id", id.String()),
			slog.String("from", string(currentStatus)),
			slog.String("to", string(newStatus)),
			slog.Any("error", err),
		)
		return nil, currentStatus, err
	}

	queryUpdate := `
		UPDATE merchants
		SET status = $1, rejection_reason = $2, updated_at = NOW()
		WHERE id = $3 AND deleted_at IS NULL
		RETURNING id, business_name, first_name, last_name, business_email, business_phone, pan_card_number, status, rejection_reason, created_at, updated_at, deleted_at
	`
	var m model.Merchant
	err = tx.QueryRow(ctx, queryUpdate, string(newStatus), reason, id).Scan(
		&m.ID,
		&m.BusinessName,
		&m.FirstName,
		&m.LastName,
		&m.BusinessEmail,
		&m.BusinessPhone,
		&m.PanCardNumber,
		&m.Status,
		&m.RejectionReason,
		&m.CreatedAt,
		&m.UpdatedAt,
		&m.DeletedAt,
	)
	if err != nil {
		r.logger.Error("Repository: failed to update merchant record in tx", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, currentStatus, appErrors.Internal(err, "failed to update merchant status")
	}

	var action model.LifecycleAction
	switch newStatus {
	case model.MerchantStatusApproved:
		action = model.LifecycleActionApprove
	case model.MerchantStatusRejected:
		action = model.LifecycleActionReject
	case model.MerchantStatusSuspended:
		action = model.LifecycleActionSuspend
	case model.MerchantStatusActive:
		if currentStatus == model.MerchantStatusSuspended {
			action = model.LifecycleActionReactivate
		} else {
			action = model.LifecycleActionActivate
		}
	default:
		action = model.LifecycleActionUpdateStatus
	}

	auditQuery := `
		INSERT INTO merchant_lifecycle_audit (merchant_id, admin_id, action, previous_status, new_status, reason, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'SUCCESS', NOW())
	`
	_, err = tx.Exec(ctx, auditQuery, id, updatedBy, string(action), string(currentStatus), string(newStatus), reason)
	if err != nil {
		r.logger.Error("Repository: failed to write to merchant_lifecycle_audit in tx", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, currentStatus, appErrors.Internal(err, "failed to record merchant lifecycle audit")
	}

	// Update pending appeals with admin comment and reviewed_at timestamp
	nowUtc := time.Now().UTC()
	switch newStatus {
	case model.MerchantStatusRejected, model.MerchantStatusSuspended:
		queryAppeal := `
			UPDATE merchant_appeals
			SET status = 'REJECTED', admin_comment = $1, reviewed_at = $2, updated_at = $2
			WHERE merchant_id = $3 AND status = 'PENDING'
		`
		_, _ = tx.Exec(ctx, queryAppeal, reason, nowUtc, id)
	case model.MerchantStatusApproved, model.MerchantStatusActive:
		queryAppeal := `
			UPDATE merchant_appeals
			SET status = 'APPROVED', admin_comment = $1, reviewed_at = $2, updated_at = $2
			WHERE merchant_id = $3 AND status = 'PENDING'
		`
		_, _ = tx.Exec(ctx, queryAppeal, reason, nowUtc, id)
	}


	// Transactional Outbox integration: write state transition domain event
	switch newStatus {
	case model.MerchantStatusActive, model.MerchantStatusApproved:
		actPayload := events.MerchantActivatedEvent{
			MerchantID:     id.String(),
			PreviousStatus: string(currentStatus),
			NewStatus:      string(newStatus),
			ActivatedBy:    updatedBy,
			Reason:         reason,
			ActivatedAt:    time.Now().UTC(),
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeMerchantActivated, "merchant-service", id.String(), actPayload)
		if err == nil {
			if pBytes, err := env.Marshal(); err == nil {
				_ = r.outboxStore.Insert(ctx, tx, &outbox.Event{
					AggregateType: "merchant",
					AggregateID:   id.String(),
					EventType:     events.EventTypeMerchantActivated,
					Payload:       pBytes,
					Topic:         events.TopicMerchantActivated,
				})
			}
		}
	case model.MerchantStatusSuspended:
		suspPayload := events.MerchantSuspendedEvent{
			MerchantID:     id.String(),
			PreviousStatus: string(currentStatus),
			NewStatus:      string(newStatus),
			SuspendedBy:    updatedBy,
			Reason:         reason,
			SuspendedAt:    time.Now().UTC(),
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeMerchantSuspended, "merchant-service", id.String(), suspPayload)
		if err == nil {
			if pBytes, err := env.Marshal(); err == nil {
				_ = r.outboxStore.Insert(ctx, tx, &outbox.Event{
					AggregateType: "merchant",
					AggregateID:   id.String(),
					EventType:     events.EventTypeMerchantSuspended,
					Payload:       pBytes,
					Topic:         events.TopicMerchantSuspended,
				})
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		r.logger.Error("Repository: failed to commit status update tx", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, currentStatus, appErrors.Internal(err, "failed to commit transaction")
	}

	r.logger.Info("Repository: merchant status updated and audited successfully",
		slog.String("merchant_id", id.String()),
		slog.String("previous_status", string(currentStatus)),
		slog.String("status", string(newStatus)),
	)
	return &m, currentStatus, nil
}

func (r *pgMerchantRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, rejectionReason string) (*model.Merchant, error) {
	m, _, err := r.UpdateStatusWithAudit(ctx, id, model.MerchantStatus(status), rejectionReason, "ADMIN")
	return m, err
}

func (r *pgMerchantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE merchants
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`
	cmdTag, err := r.db.Pool.Exec(ctx, query, id)
	if err != nil {
		r.logger.Error("Repository: failed to soft delete merchant", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return appErrors.Internal(err, "failed to delete merchant")
	}
	if cmdTag.RowsAffected() == 0 {
		r.logger.Warn("Repository: merchant not found or already deleted", slog.String("merchant_id", id.String()))
		return appErrors.NotFound("merchant not found")
	}
	r.logger.Debug("Repository: merchant soft deleted successfully", slog.String("merchant_id", id.String()))
	return nil
}

func (r *pgMerchantRepository) ExecuteLifecycleTransition(
	ctx context.Context,
	merchantID uuid.UUID,
	action model.LifecycleAction,
	reason string,
	adminID string,
	reqID string,
) (*model.Merchant, model.MerchantStatus, error) {
	if adminID == "" {
		adminID = "ADMIN"
	}

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		r.logger.Error("Repository: failed to begin tx for lifecycle transition", slog.Any("error", err))
		return nil, "", appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the merchant row with FOR UPDATE to serialize concurrent requests and eliminate race conditions
	var m model.Merchant
	queryCurrent := `
		SELECT id, business_name, first_name, last_name, business_email, business_phone, pan_card_number, status, rejection_reason, version, created_at, updated_at, deleted_at
		FROM merchants
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`
	err = tx.QueryRow(ctx, queryCurrent, merchantID).Scan(
		&m.ID,
		&m.BusinessName,
		&m.FirstName,
		&m.LastName,
		&m.BusinessEmail,
		&m.BusinessPhone,
		&m.PanCardNumber,
		&m.Status,
		&m.RejectionReason,
		&m.Version,
		&m.CreatedAt,
		&m.UpdatedAt,
		&m.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Warn("Repository: merchant not found for lifecycle transition", slog.String("merchant_id", merchantID.String()))
			return nil, "", appErrors.NotFound("merchant not found")
		}
		r.logger.Error("Repository: failed to query merchant for lifecycle transition", slog.String("merchant_id", merchantID.String()), slog.Any("error", err))
		return nil, "", appErrors.Internal(err, "failed to query merchant")
	}

	currentStatus := model.MerchantStatus(m.Status)

	// Enforce strict state machine transition rules
	targetStatus, transErr := model.ValidateLifecycleTransition(action, currentStatus)
	if transErr != nil {
		r.logger.Warn("Repository: invalid lifecycle transition attempt",
			slog.String("merchant_id", merchantID.String()),
			slog.String("action", string(action)),
			slog.String("current_status", string(currentStatus)),
			slog.Any("error", transErr),
		)

		// Emit immutable FAILED audit record
		failedAudit := &model.MerchantLifecycleAudit{
			ID:             uuid.New(),
			MerchantID:     merchantID,
			AdminID:        adminID,
			Action:         action,
			PreviousStatus: string(currentStatus),
			NewStatus:      string(currentStatus),
			Reason:         reason,
			Status:         model.AuditStatusFailed,
		}
		_ = r.RecordLifecycleAudit(ctx, failedAudit)

		return nil, currentStatus, transErr
	}

	// Apply atomic update with optimistic version bump
	queryUpdate := `
		UPDATE merchants
		SET status = $1, version = version + 1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING id, business_name, first_name, last_name, business_email, business_phone, pan_card_number, status, rejection_reason, version, created_at, updated_at, deleted_at
	`
	err = tx.QueryRow(ctx, queryUpdate, string(targetStatus), merchantID).Scan(
		&m.ID,
		&m.BusinessName,
		&m.FirstName,
		&m.LastName,
		&m.BusinessEmail,
		&m.BusinessPhone,
		&m.PanCardNumber,
		&m.Status,
		&m.RejectionReason,
		&m.Version,
		&m.CreatedAt,
		&m.UpdatedAt,
		&m.DeletedAt,
	)
	if err != nil {
		r.logger.Error("Repository: failed to update merchant status during lifecycle transition", slog.String("merchant_id", merchantID.String()), slog.Any("error", err))
		return nil, currentStatus, appErrors.Internal(err, "failed to update merchant status")
	}

	// Write immutable SUCCESS audit record into merchant_lifecycle_audit table
	queryAudit := `
		INSERT INTO merchant_lifecycle_audit (merchant_id, admin_id, action, previous_status, new_status, reason, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`
	_, err = tx.Exec(ctx, queryAudit, merchantID, adminID, string(action), string(currentStatus), string(targetStatus), reason, string(model.AuditStatusSuccess))
	if err != nil {
		r.logger.Error("Repository: failed to insert into merchant_lifecycle_audit", slog.String("merchant_id", merchantID.String()), slog.Any("error", err))
		return nil, currentStatus, appErrors.Internal(err, "failed to record merchant lifecycle audit")
	}

	// Update pending appeals with admin comment and reviewed_at timestamp
	switch action {
	case model.LifecycleActionSuspend:
		queryAppeal := `
			UPDATE merchant_appeals
			SET status = 'REJECTED', admin_comment = $1, reviewed_at = $2, updated_at = $2
			WHERE merchant_id = $3 AND status = 'PENDING'
		`
		_, _ = tx.Exec(ctx, queryAppeal, reason, time.Now().UTC(), merchantID)
	case model.LifecycleActionActivate, model.LifecycleActionReactivate:
		queryAppeal := `
			UPDATE merchant_appeals
			SET status = 'APPROVED', admin_comment = $1, reviewed_at = $2, updated_at = $2
			WHERE merchant_id = $3 AND status = 'PENDING'
		`
		_, _ = tx.Exec(ctx, queryAppeal, reason, time.Now().UTC(), merchantID)
	}

	// Transactional Outbox: Write domain event to outbox inside same DB transaction
	switch targetStatus {
	case model.MerchantStatusActive:
		actPayload := events.MerchantActivatedEvent{
			MerchantID:     merchantID.String(),
			PreviousStatus: string(currentStatus),
			NewStatus:      string(targetStatus),
			ActivatedBy:    adminID,
			Reason:         reason,
			ActivatedAt:    time.Now().UTC(),
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeMerchantActivated, "merchant-service", merchantID.String(), actPayload)
		if err != nil {
			r.logger.Error("Repository: failed to construct MerchantActivated envelope", slog.Any("error", err))
			return nil, currentStatus, appErrors.Internal(err, "failed to construct event envelope")
		}
		payloadBytes, err := env.Marshal()
		if err != nil {
			r.logger.Error("Repository: failed to marshal MerchantActivated payload", slog.Any("error", err))
			return nil, currentStatus, appErrors.Internal(err, "failed to marshal event payload")
		}
		outboxEvt := &outbox.Event{
			AggregateType: "merchant",
			AggregateID:   merchantID.String(),
			EventType:     events.EventTypeMerchantActivated,
			Payload:       payloadBytes,
			Topic:         events.TopicMerchantActivated,
		}
		if err := r.outboxStore.Insert(ctx, tx, outboxEvt); err != nil {
			r.logger.Error("Repository: failed to write MerchantActivated event to outbox", slog.Any("error", err))
			return nil, currentStatus, appErrors.Internal(err, "failed to record outbox event")
		}
	case model.MerchantStatusSuspended:
		suspPayload := events.MerchantSuspendedEvent{
			MerchantID:     merchantID.String(),
			PreviousStatus: string(currentStatus),
			NewStatus:      string(targetStatus),
			SuspendedBy:    adminID,
			Reason:         reason,
			SuspendedAt:    time.Now().UTC(),
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeMerchantSuspended, "merchant-service", merchantID.String(), suspPayload)
		if err != nil {
			r.logger.Error("Repository: failed to construct MerchantSuspended envelope", slog.Any("error", err))
			return nil, currentStatus, appErrors.Internal(err, "failed to construct event envelope")
		}
		payloadBytes, err := env.Marshal()
		if err != nil {
			r.logger.Error("Repository: failed to marshal MerchantSuspended payload", slog.Any("error", err))
			return nil, currentStatus, appErrors.Internal(err, "failed to marshal event payload")
		}
		outboxEvt := &outbox.Event{
			AggregateType: "merchant",
			AggregateID:   merchantID.String(),
			EventType:     events.EventTypeMerchantSuspended,
			Payload:       payloadBytes,
			Topic:         events.TopicMerchantSuspended,
		}
		if err := r.outboxStore.Insert(ctx, tx, outboxEvt); err != nil {
			r.logger.Error("Repository: failed to write MerchantSuspended event to outbox", slog.Any("error", err))
			return nil, currentStatus, appErrors.Internal(err, "failed to record outbox event")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		r.logger.Error("Repository: failed to commit lifecycle transition transaction", slog.String("merchant_id", merchantID.String()), slog.Any("error", err))
		return nil, currentStatus, appErrors.Internal(err, "failed to commit transaction")
	}

	r.logger.Info("Repository: merchant lifecycle transition committed successfully",
		slog.String("merchant_id", merchantID.String()),
		slog.String("action", string(action)),
		slog.String("previous_status", string(currentStatus)),
		slog.String("new_status", string(targetStatus)),
		slog.String("admin_id", adminID),
	)

	return &m, currentStatus, nil
}

func (r *pgMerchantRepository) RecordLifecycleAudit(ctx context.Context, audit *model.MerchantLifecycleAudit) error {
	query := `
		INSERT INTO merchant_lifecycle_audit (id, merchant_id, admin_id, action, previous_status, new_status, reason, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	`
	id := audit.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	_, err := r.db.Pool.Exec(ctx, query,
		id,
		audit.MerchantID,
		audit.AdminID,
		string(audit.Action),
		audit.PreviousStatus,
		audit.NewStatus,
		audit.Reason,
		string(audit.Status),
	)
	if err != nil {
		r.logger.Error("Repository: failed to record standalone lifecycle audit", slog.Any("error", err))
		return appErrors.Internal(err, "failed to record lifecycle audit")
	}
	return nil
}

func (r *pgMerchantRepository) CreateAppeal(ctx context.Context, appeal *model.MerchantAppeal) error {
	query := `
		INSERT INTO merchant_appeals (id, merchant_id, reason, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`
	if appeal.ID == uuid.Nil {
		appeal.ID = uuid.New()
	}
	now := time.Now()
	if appeal.CreatedAt.IsZero() {
		appeal.CreatedAt = now
	}
	if appeal.UpdatedAt.IsZero() {
		appeal.UpdatedAt = now
	}
	if appeal.Status == "" {
		appeal.Status = string(model.MerchantAppealStatusPending)
	}

	return r.db.Pool.QueryRow(ctx, query,
		appeal.ID,
		appeal.MerchantID,
		appeal.Reason,
		appeal.Status,
		appeal.CreatedAt,
		appeal.UpdatedAt,
	).Scan(&appeal.ID, &appeal.CreatedAt, &appeal.UpdatedAt)
}

func (r *pgMerchantRepository) GetAppealsByMerchantID(ctx context.Context, merchantID uuid.UUID) ([]*model.MerchantAppeal, error) {
	query := `
		SELECT id, merchant_id, reason, status, admin_comment, reviewed_at, created_at, updated_at
		FROM merchant_appeals
		WHERE merchant_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.Pool.Query(ctx, query, merchantID)
	if err != nil {
		r.logger.Error("Repository: failed to query merchant appeals", slog.String("merchant_id", merchantID.String()), slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to query merchant appeals")
	}
	defer rows.Close()

	var appeals []*model.MerchantAppeal
	for rows.Next() {
		var a model.MerchantAppeal
		if err := rows.Scan(
			&a.ID,
			&a.MerchantID,
			&a.Reason,
			&a.Status,
			&a.AdminComment,
			&a.ReviewedAt,
			&a.CreatedAt,
			&a.UpdatedAt,
		); err != nil {
			r.logger.Error("Repository: failed to scan merchant appeal", slog.String("merchant_id", merchantID.String()), slog.Any("error", err))
			return nil, appErrors.Internal(err, "failed to scan merchant appeal")
		}
		appeals = append(appeals, &a)
	}

	return appeals, nil
}
