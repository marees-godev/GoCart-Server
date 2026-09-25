package repository

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
)

type MerchantRepository interface {
	Create(ctx context.Context, merchant *model.Merchant) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Merchant, error)
	List(ctx context.Context, limit, offset int, status string) ([]*model.Merchant, int, error)
	Update(ctx context.Context, merchant *model.Merchant) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, rejectionReason string) (*model.Merchant, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type pgMerchantRepository struct {
	db     *database.DB
	logger *slog.Logger
}

func NewMerchantRepository(db *database.DB, log ...*slog.Logger) MerchantRepository {
	var l *slog.Logger
	if len(log) > 0 && log[0] != nil {
		l = log[0]
	} else {
		l = slog.Default()
	}
	return &pgMerchantRepository{
		db:     db,
		logger: l,
	}
}

func (r *pgMerchantRepository) Create(ctx context.Context, merchant *model.Merchant) error {
	if merchant.ID == uuid.Nil {
		merchant.ID = uuid.New()
	}
	query := `
		INSERT INTO merchants (id, business_name, first_name, last_name, business_email, business_phone, tax_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
		RETURNING id, created_at, updated_at
	`
	err := r.db.Pool.QueryRow(ctx, query,
		merchant.ID,
		merchant.BusinessName,
		merchant.FirstName,
		merchant.LastName,
		merchant.BusinessEmail,
		merchant.BusinessPhone,
		merchant.TaxID,
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
	r.logger.Debug("Repository: merchant inserted successfully", slog.String("merchant_id", merchant.ID.String()))
	return nil
}

func (r *pgMerchantRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Merchant, error) {
	query := `
		SELECT id, business_name, first_name, last_name, business_email, business_phone, tax_id, status, rejection_reason, created_at, updated_at, deleted_at
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
		&m.TaxID,
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
		countQuery = `SELECT COUNT(*) FROM merchants WHERE status = $1 AND deleted_at IS NULL`
		countArgs = append(countArgs, status)
		listQuery = `
			SELECT id, business_name, first_name, last_name, business_email, business_phone, tax_id, status, rejection_reason, created_at, updated_at, deleted_at
			FROM merchants
			WHERE status = $1 AND deleted_at IS NULL
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`
		args = append(args, status, limit, offset)
	} else {
		countQuery = `SELECT COUNT(*) FROM merchants WHERE deleted_at IS NULL`
		listQuery = `
			SELECT id, business_name, first_name, last_name, business_email, business_phone, tax_id, status, rejection_reason, created_at, updated_at, deleted_at
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
			&m.TaxID,
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

func (r *pgMerchantRepository) Update(ctx context.Context, merchant *model.Merchant) error {
	query := `
		UPDATE merchants
		SET business_name = $1, business_phone = $2, tax_id = $3, updated_at = NOW()
		WHERE id = $4 AND deleted_at IS NULL
		RETURNING updated_at
	`
	err := r.db.Pool.QueryRow(ctx, query,
		merchant.BusinessName,
		merchant.BusinessPhone,
		merchant.TaxID,
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

func (r *pgMerchantRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, rejectionReason string) (*model.Merchant, error) {
	query := `
		UPDATE merchants
		SET status = $1, rejection_reason = $2, updated_at = NOW()
		WHERE id = $3 AND deleted_at IS NULL
		RETURNING id, business_name, first_name, last_name, business_email, business_phone, tax_id, status, rejection_reason, created_at, updated_at, deleted_at
	`
	var m model.Merchant
	err := r.db.Pool.QueryRow(ctx, query, status, rejectionReason, id).Scan(
		&m.ID,
		&m.BusinessName,
		&m.FirstName,
		&m.LastName,
		&m.BusinessEmail,
		&m.BusinessPhone,
		&m.TaxID,
		&m.Status,
		&m.RejectionReason,
		&m.CreatedAt,
		&m.UpdatedAt,
		&m.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Warn("Repository: merchant not found for status update", slog.String("merchant_id", id.String()))
			return nil, appErrors.NotFound("merchant not found")
		}
		r.logger.Error("Repository: failed to update merchant status", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to update merchant status")
	}
	r.logger.Debug("Repository: merchant status updated successfully", slog.String("merchant_id", id.String()), slog.String("status", status))
	return &m, nil
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
