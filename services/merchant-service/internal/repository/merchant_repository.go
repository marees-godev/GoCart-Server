package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
)

type MerchantRepository interface {
	Create(ctx context.Context, merchant *model.Merchant) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Merchant, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) (*model.Merchant, error)
	List(ctx context.Context, limit, offset int, status string) ([]*model.Merchant, int, error)
	Update(ctx context.Context, merchant *model.Merchant) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, rejectionReason string) (*model.Merchant, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type pgMerchantRepository struct {
	db *database.DB
}

func NewMerchantRepository(db *database.DB) MerchantRepository {
	return &pgMerchantRepository{db: db}
}

func (r *pgMerchantRepository) Create(ctx context.Context, merchant *model.Merchant) error {
	if merchant.ID == uuid.Nil {
		merchant.ID = uuid.New()
	}
	query := `
		INSERT INTO merchants (id, user_id, business_name, first_name, last_name, business_email, business_phone, tax_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
		RETURNING id, created_at, updated_at
	`
	err := r.db.Pool.QueryRow(ctx, query,
		merchant.ID,
		merchant.UserID,
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
			existing, getErr := r.GetByID(ctx, merchant.ID)
			if getErr != nil {
				return getErr
			}
			*merchant = *existing
			return nil
		}
		return appErrors.Internal(err, "failed to create merchant")
	}
	return nil
}

func (r *pgMerchantRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Merchant, error) {
	query := `
		SELECT id, user_id, business_name, first_name, last_name, business_email, business_phone, tax_id, status, rejection_reason, created_at, updated_at
		FROM merchants
		WHERE id = $1
	`
	var m model.Merchant
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&m.ID,
		&m.UserID,
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
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("merchant not found")
		}
		return nil, appErrors.Internal(err, "failed to query merchant by id")
	}
	return &m, nil
}

func (r *pgMerchantRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*model.Merchant, error) {
	query := `
		SELECT id, user_id, business_name, first_name, last_name, business_email, business_phone, tax_id, status, rejection_reason, created_at, updated_at
		FROM merchants
		WHERE user_id = $1
	`
	var m model.Merchant
	err := r.db.Pool.QueryRow(ctx, query, userID).Scan(
		&m.ID,
		&m.UserID,
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
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("merchant not found")
		}
		return nil, appErrors.Internal(err, "failed to query merchant by user id")
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
		countQuery = `SELECT COUNT(*) FROM merchants WHERE status = $1`
		countArgs = append(countArgs, status)
		listQuery = `
			SELECT id, user_id, business_name, first_name, last_name, business_email, business_phone, tax_id, status, rejection_reason, created_at, updated_at
			FROM merchants
			WHERE status = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`
		args = append(args, status, limit, offset)
	} else {
		countQuery = `SELECT COUNT(*) FROM merchants`
		listQuery = `
			SELECT id, user_id, business_name, first_name, last_name, business_email, business_phone, tax_id, status, rejection_reason, created_at, updated_at
			FROM merchants
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`
		args = append(args, limit, offset)
	}

	var total int
	err := r.db.Pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, appErrors.Internal(err, "failed to count merchants")
	}

	rows, err := r.db.Pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, appErrors.Internal(err, "failed to list merchants")
	}
	defer rows.Close()

	merchants := make([]*model.Merchant, 0)
	for rows.Next() {
		var m model.Merchant
		if err := rows.Scan(
			&m.ID,
			&m.UserID,
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
		); err != nil {
			return nil, 0, appErrors.Internal(err, "failed to scan merchant row")
		}
		merchants = append(merchants, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, appErrors.Internal(err, "error iterating merchant rows")
	}

	return merchants, total, nil
}

func (r *pgMerchantRepository) Update(ctx context.Context, merchant *model.Merchant) error {
	query := `
		UPDATE merchants
		SET business_name = $1, first_name = $2, last_name = $3, business_email = $4, business_phone = $5, tax_id = $6, updated_at = NOW()
		WHERE id = $7
		RETURNING updated_at
	`
	err := r.db.Pool.QueryRow(ctx, query,
		merchant.BusinessName,
		merchant.FirstName,
		merchant.LastName,
		merchant.BusinessEmail,
		merchant.BusinessPhone,
		merchant.TaxID,
		merchant.ID,
	).Scan(&merchant.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return appErrors.NotFound("merchant not found")
		}
		return appErrors.Internal(err, "failed to update merchant")
	}
	return nil
}

func (r *pgMerchantRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, rejectionReason string) (*model.Merchant, error) {
	query := `
		UPDATE merchants
		SET status = $1, rejection_reason = $2, updated_at = NOW()
		WHERE id = $3
		RETURNING id, user_id, business_name, first_name, last_name, business_email, business_phone, tax_id, status, rejection_reason, created_at, updated_at
	`
	var m model.Merchant
	err := r.db.Pool.QueryRow(ctx, query, status, rejectionReason, id).Scan(
		&m.ID,
		&m.UserID,
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
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("merchant not found")
		}
		return nil, appErrors.Internal(err, "failed to update merchant status")
	}
	return &m, nil
}

func (r *pgMerchantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM merchants WHERE id = $1`
	cmdTag, err := r.db.Pool.Exec(ctx, query, id)
	if err != nil {
		return appErrors.Internal(err, "failed to delete merchant")
	}
	if cmdTag.RowsAffected() == 0 {
		return appErrors.NotFound("merchant not found")
	}
	return nil
}
