package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/model"
)

type StoreRepository interface {
	Create(ctx context.Context, store *model.Store) error
	GetByID(ctx context.Context, id string) (*model.Store, error)
	GetByMerchantID(ctx context.Context, merchantID string) (*model.Store, error)
	GetBySlug(ctx context.Context, slug string) (*model.Store, error)
	List(ctx context.Context, merchantID string, limit, offset int) ([]*model.Store, int, error)
	Update(ctx context.Context, store *model.Store) error
	UpdateStatus(ctx context.Context, id string, expectedStatus, newStatus string, rejectionReason *string) (*model.Store, error)
	IsSlugAvailable(ctx context.Context, slug string, excludeID string) (bool, error)
}

type pgStoreRepository struct {
	pool *pgxpool.Pool
}

func NewStoreRepository(pool *pgxpool.Pool) StoreRepository {
	return &pgStoreRepository{pool: pool}
}

func (r *pgStoreRepository) Create(ctx context.Context, s *model.Store) error {
	if s.ApprovalStatus == "" {
		s.ApprovalStatus = model.StoreStatusDraft
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return appErrors.Internal(err, "failed to start transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	query := `
		INSERT INTO stores (
			merchant_id, name, slug, business_email, business_phone, description,
			logo_url, address, is_vacation_mode, approval_status, rejection_reason,
			avg_store_rating, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, NOW(), NOW()
		)
		RETURNING id, created_at, updated_at
	`

	err = tx.QueryRow(ctx, query,
		s.MerchantID,
		s.Name,
		s.Slug,
		s.BusinessEmail,
		s.BusinessPhone,
		s.Description,
		s.LogoURL,
		s.Address,
		s.IsVacationMode,
		s.ApprovalStatus,
		s.RejectionReason,
		s.AvgStoreRating,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return appErrors.Conflict("store slug or identity already exists")
		}
		return appErrors.Internal(err, "failed to insert store")
	}

	// Insert bank account into normalized store_bank_accounts table if present
	ba := s.BankAccount
	if ba == nil && s.BankAccountDetails != nil && *s.BankAccountDetails != "" {
		var parsed model.StoreBankAccount
		if jsonErr := json.Unmarshal([]byte(*s.BankAccountDetails), &parsed); jsonErr == nil && parsed.AccountNumber != "" {
			ba = &parsed
		}
	}

	if ba != nil && (ba.AccountNumber != "" || ba.BankName != "") {
		bankQuery := `
			INSERT INTO store_bank_accounts (
				store_id, account_holder_name, account_number, routing_number,
				bank_name, tax_id, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, NOW(), NOW()
			)
			RETURNING id, created_at, updated_at
		`
		err = tx.QueryRow(ctx, bankQuery,
			s.ID,
			ba.AccountHolderName,
			ba.AccountNumber,
			ba.RoutingNumber,
			ba.BankName,
			ba.TaxID,
		).Scan(&ba.ID, &ba.CreatedAt, &ba.UpdatedAt)
		if err != nil {
			return appErrors.Internal(err, "failed to insert store bank account")
		}
		ba.StoreID = s.ID
		s.BankAccount = ba
	}

	if err := tx.Commit(ctx); err != nil {
		return appErrors.Internal(err, "failed to commit create store transaction")
	}

	return nil
}

const selectStoreWithBankSQL = `
	SELECT
		s.id, s.merchant_id, s.name, s.slug, s.business_email, s.business_phone,
		s.description, s.logo_url, s.address, s.is_vacation_mode, s.approval_status,
		s.rejection_reason, s.avg_store_rating, s.created_at, s.updated_at,
		b.id, b.account_holder_name, b.account_number, b.routing_number,
		b.bank_name, b.tax_id, b.created_at, b.updated_at
	FROM stores s
	LEFT JOIN store_bank_accounts b ON s.id = b.store_id
`

func (r *pgStoreRepository) scanStoreRow(row pgx.Row) (*model.Store, error) {
	var s model.Store
	var bID, bHolder, bNumber, bRouting, bBank, bTaxID *string
	var bCreatedAt, bUpdatedAt *time.Time

	err := row.Scan(
		&s.ID,
		&s.MerchantID,
		&s.Name,
		&s.Slug,
		&s.BusinessEmail,
		&s.BusinessPhone,
		&s.Description,
		&s.LogoURL,
		&s.Address,
		&s.IsVacationMode,
		&s.ApprovalStatus,
		&s.RejectionReason,
		&s.AvgStoreRating,
		&s.CreatedAt,
		&s.UpdatedAt,
		&bID,
		&bHolder,
		&bNumber,
		&bRouting,
		&bBank,
		&bTaxID,
		&bCreatedAt,
		&bUpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	if bID != nil && *bID != "" {
		holder := ""
		if bHolder != nil {
			holder = *bHolder
		}
		number := ""
		if bNumber != nil {
			number = *bNumber
		}
		bankName := ""
		if bBank != nil {
			bankName = *bBank
		}

		ba := &model.StoreBankAccount{
			ID:                *bID,
			StoreID:           s.ID,
			AccountHolderName: holder,
			AccountNumber:     number,
			RoutingNumber:     bRouting,
			BankName:          bankName,
			TaxID:             bTaxID,
		}
		if bCreatedAt != nil {
			ba.CreatedAt = *bCreatedAt
		}
		if bUpdatedAt != nil {
			ba.UpdatedAt = *bUpdatedAt
		}
		s.BankAccount = ba

		if bytes, jsonErr := json.Marshal(ba); jsonErr == nil {
			str := string(bytes)
			s.BankAccountDetails = &str
		}
	}

	return &s, nil
}

func (r *pgStoreRepository) GetByID(ctx context.Context, id string) (*model.Store, error) {
	query := selectStoreWithBankSQL + " WHERE s.id = $1"
	row := r.pool.QueryRow(ctx, query, id)

	s, err := r.scanStoreRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("store not found")
		}
		return nil, appErrors.Internal(err, "failed to query store by id")
	}

	return s, nil
}

func (r *pgStoreRepository) GetByMerchantID(ctx context.Context, merchantID string) (*model.Store, error) {
	query := selectStoreWithBankSQL + " WHERE s.merchant_id = $1 ORDER BY s.created_at DESC LIMIT 1"
	row := r.pool.QueryRow(ctx, query, merchantID)

	s, err := r.scanStoreRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("store not found for this merchant")
		}
		return nil, appErrors.Internal(err, "failed to query store by merchant_id")
	}

	return s, nil
}

func (r *pgStoreRepository) GetBySlug(ctx context.Context, slug string) (*model.Store, error) {
	query := selectStoreWithBankSQL + " WHERE s.slug = $1"
	row := r.pool.QueryRow(ctx, query, slug)

	s, err := r.scanStoreRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("store not found")
		}
		return nil, appErrors.Internal(err, "failed to query store by slug")
	}

	return s, nil
}

func (r *pgStoreRepository) List(ctx context.Context, merchantID string, limit, offset int) ([]*model.Store, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	baseWhere := "WHERE 1=1"
	args := make([]any, 0)
	argIdx := 1

	if merchantID != "" {
		baseWhere += fmt.Sprintf(" AND s.merchant_id = $%d", argIdx)
		args = append(args, merchantID)
		argIdx++
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM stores s %s", baseWhere)
	var total int
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, appErrors.Internal(err, "failed to count stores")
	}

	listQuery := fmt.Sprintf(`
		%s
		%s
		ORDER BY s.created_at DESC
		LIMIT $%d OFFSET $%d
	`, selectStoreWithBankSQL, baseWhere, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, appErrors.Internal(err, "failed to list stores")
	}
	defer rows.Close()

	stores := make([]*model.Store, 0)
	for rows.Next() {
		var s model.Store
		var bID, bHolder, bNumber, bRouting, bBank, bTaxID *string
		var bCreatedAt, bUpdatedAt *time.Time

		if err := rows.Scan(
			&s.ID,
			&s.MerchantID,
			&s.Name,
			&s.Slug,
			&s.BusinessEmail,
			&s.BusinessPhone,
			&s.Description,
			&s.LogoURL,
			&s.Address,
			&s.IsVacationMode,
			&s.ApprovalStatus,
			&s.RejectionReason,
			&s.AvgStoreRating,
			&s.CreatedAt,
			&s.UpdatedAt,
			&bID,
			&bHolder,
			&bNumber,
			&bRouting,
			&bBank,
			&bTaxID,
			&bCreatedAt,
			&bUpdatedAt,
		); err != nil {
			return nil, 0, appErrors.Internal(err, "failed to scan store row")
		}

		if bID != nil && *bID != "" {
			holder := ""
			if bHolder != nil {
				holder = *bHolder
			}
			number := ""
			if bNumber != nil {
				number = *bNumber
			}
			bankName := ""
			if bBank != nil {
				bankName = *bBank
			}

			ba := &model.StoreBankAccount{
				ID:                *bID,
				StoreID:           s.ID,
				AccountHolderName: holder,
				AccountNumber:     number,
				RoutingNumber:     bRouting,
				BankName:          bankName,
				TaxID:             bTaxID,
			}
			if bCreatedAt != nil {
				ba.CreatedAt = *bCreatedAt
			}
			if bUpdatedAt != nil {
				ba.UpdatedAt = *bUpdatedAt
			}
			s.BankAccount = ba
			if bytes, jsonErr := json.Marshal(ba); jsonErr == nil {
				str := string(bytes)
				s.BankAccountDetails = &str
			}
		}

		stores = append(stores, &s)
	}

	return stores, total, nil
}

func (r *pgStoreRepository) Update(ctx context.Context, s *model.Store) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return appErrors.Internal(err, "failed to start transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	query := `
		UPDATE stores
		SET
			name = $1,
			slug = $2,
			business_email = $3,
			business_phone = $4,
			description = $5,
			logo_url = $6,
			address = $7,
			is_vacation_mode = $8,
			approval_status = $9,
			rejection_reason = $10,
			avg_store_rating = $11,
			updated_at = NOW()
		WHERE id = $12
		RETURNING updated_at
	`

	err = tx.QueryRow(ctx, query,
		s.Name,
		s.Slug,
		s.BusinessEmail,
		s.BusinessPhone,
		s.Description,
		s.LogoURL,
		s.Address,
		s.IsVacationMode,
		s.ApprovalStatus,
		s.RejectionReason,
		s.AvgStoreRating,
		s.ID,
	).Scan(&s.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return appErrors.NotFound("store not found")
		}
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return appErrors.Conflict("store slug already in use")
		}
		return appErrors.Internal(err, "failed to update store")
	}

	// Upsert bank account if present
	ba := s.BankAccount
	if ba == nil && s.BankAccountDetails != nil && *s.BankAccountDetails != "" {
		var parsed model.StoreBankAccount
		if jsonErr := json.Unmarshal([]byte(*s.BankAccountDetails), &parsed); jsonErr == nil && parsed.AccountNumber != "" {
			ba = &parsed
		}
	}

	if ba != nil && (ba.AccountNumber != "" || ba.BankName != "") {
		// If existing bank account doesn't have ID, check if exists
		if ba.ID == "" {
			var existingID string
			_ = tx.QueryRow(ctx, "SELECT id FROM store_bank_accounts WHERE store_id = $1 LIMIT 1", s.ID).Scan(&existingID)
			if existingID != "" {
				ba.ID = existingID
			}
		}

		if ba.ID != "" {
			updateBankSQL := `
				UPDATE store_bank_accounts
				SET
					account_holder_name = $1,
					account_number = $2,
					routing_number = $3,
					bank_name = $4,
					tax_id = $5,
					updated_at = NOW()
				WHERE id = $6 AND store_id = $7
				RETURNING updated_at
			`
			err = tx.QueryRow(ctx, updateBankSQL,
				ba.AccountHolderName,
				ba.AccountNumber,
				ba.RoutingNumber,
				ba.BankName,
				ba.TaxID,
				ba.ID,
				s.ID,
			).Scan(&ba.UpdatedAt)
		} else {
			insertBankSQL := `
				INSERT INTO store_bank_accounts (
					store_id, account_holder_name, account_number, routing_number,
					bank_name, tax_id, created_at, updated_at
				) VALUES (
					$1, $2, $3, $4, $5, $6, NOW(), NOW()
				)
				RETURNING id, created_at, updated_at
			`
			err = tx.QueryRow(ctx, insertBankSQL,
				s.ID,
				ba.AccountHolderName,
				ba.AccountNumber,
				ba.RoutingNumber,
				ba.BankName,
				ba.TaxID,
			).Scan(&ba.ID, &ba.CreatedAt, &ba.UpdatedAt)
		}

		if err != nil {
			return appErrors.Internal(err, "failed to update store bank account")
		}
		ba.StoreID = s.ID
		s.BankAccount = ba
	}

	if err := tx.Commit(ctx); err != nil {
		return appErrors.Internal(err, "failed to commit update store transaction")
	}

	return nil
}

func bankQueryWithoutConflict(sql string) string {
	return `
		INSERT INTO store_bank_accounts (
			store_id, account_holder_name, account_number, routing_number,
			bank_name, account_type, branch_code, is_primary, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW()
		)
		RETURNING id, created_at, updated_at
	`
}

func (r *pgStoreRepository) IsSlugAvailable(ctx context.Context, slug string, excludeID string) (bool, error) {
	query := "SELECT COUNT(*) FROM stores WHERE slug = $1"
	args := []any{slug}
	if excludeID != "" {
		query += " AND id != $2"
		args = append(args, excludeID)
	}

	var count int
	if err := r.pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return false, appErrors.Internal(err, "failed to check slug availability")
	}

	return count == 0, nil
}

func (r *pgStoreRepository) UpdateStatus(ctx context.Context, id string, expectedStatus, newStatus string, rejectionReason *string) (*model.Store, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to start transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	checkQuery := `SELECT approval_status FROM stores WHERE id = $1 FOR UPDATE`
	err = tx.QueryRow(ctx, checkQuery, id).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("store not found")
		}
		return nil, appErrors.Internal(err, "failed to query store status")
	}

	if currentStatus != expectedStatus {
		return nil, appErrors.UnprocessableEntity(fmt.Sprintf("invalid state transition: store in status %s cannot transition to %s (expected %s)", currentStatus, newStatus, expectedStatus))
	}

	updateQuery := `
		UPDATE stores
		SET approval_status = $1, rejection_reason = $2, updated_at = NOW()
		WHERE id = $3
	`
	_, err = tx.Exec(ctx, updateQuery, newStatus, rejectionReason, id)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to update store status")
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, appErrors.Internal(err, "failed to commit status update transaction")
	}

	return r.GetByID(ctx, id)
}
