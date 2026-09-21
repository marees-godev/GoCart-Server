package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
)

type AddressRepository interface {
	Create(ctx context.Context, address *model.Address) error
	GetByID(ctx context.Context, id string) (*model.Address, error)
	ListByUserID(ctx context.Context, userID string) ([]*model.Address, error)
	Update(ctx context.Context, address *model.Address) error
	Delete(ctx context.Context, id string, userID string) error
	ClearDefaultAddresses(ctx context.Context, userID string) error
	SetDefaultAddress(ctx context.Context, id string, userID string) error
	CountByUserID(ctx context.Context, userID string) (int, error)
}

type pgAddressRepository struct {
	pool *pgxpool.Pool
}

func NewAddressRepository(pool *pgxpool.Pool) AddressRepository {
	return &pgAddressRepository{pool: pool}
}

func (r *pgAddressRepository) Create(ctx context.Context, address *model.Address) error {
	query := `
		INSERT INTO user_addresses (user_id, country_id, label, full_name, phone_number, email_address, address_line, city, state, postal_code, country, is_default)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at
	`
	err := r.pool.QueryRow(ctx, query,
		address.UserID,
		address.CountryID,
		address.Label,
		address.FullName,
		address.PhoneNumber,
		address.EmailAddress,
		address.AddressLine,
		address.City,
		address.State,
		address.PostalCode,
		address.Country,
		address.IsDefault,
	).Scan(&address.ID, &address.CreatedAt, &address.UpdatedAt)
	if err != nil {
		return appErrors.Internal(err, "failed to create user address")
	}
	return nil
}

func (r *pgAddressRepository) GetByID(ctx context.Context, id string) (*model.Address, error) {
	query := `
		SELECT id, user_id, country_id, label, full_name, phone_number, email_address, COALESCE(address_line, address_line1, ''), city, state, postal_code, COALESCE(country, ''), is_default, created_at, updated_at
		FROM user_addresses
		WHERE id = $1
	`
	var a model.Address
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&a.ID,
		&a.UserID,
		&a.CountryID,
		&a.Label,
		&a.FullName,
		&a.PhoneNumber,
		&a.EmailAddress,
		&a.AddressLine,
		&a.City,
		&a.State,
		&a.PostalCode,
		&a.Country,
		&a.IsDefault,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("address not found")
		}
		return nil, appErrors.Internal(err, "failed to query address by id")
	}
	return &a, nil
}

func (r *pgAddressRepository) ListByUserID(ctx context.Context, userID string) ([]*model.Address, error) {
	query := `
		SELECT id, user_id, country_id, label, full_name, phone_number, email_address, COALESCE(address_line, address_line1, ''), city, state, postal_code, COALESCE(country, ''), is_default, created_at, updated_at
		FROM user_addresses
		WHERE user_id = $1
		ORDER BY is_default DESC, created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to list user addresses")
	}
	defer rows.Close()

	addresses := make([]*model.Address, 0)
	for rows.Next() {
		var a model.Address
		if err := rows.Scan(
			&a.ID,
			&a.UserID,
			&a.CountryID,
			&a.Label,
			&a.FullName,
			&a.PhoneNumber,
			&a.EmailAddress,
			&a.AddressLine,
			&a.City,
			&a.State,
			&a.PostalCode,
			&a.Country,
			&a.IsDefault,
			&a.CreatedAt,
			&a.UpdatedAt,
		); err != nil {
			return nil, appErrors.Internal(err, "failed to scan address row")
		}
		addresses = append(addresses, &a)
	}

	if err := rows.Err(); err != nil {
		return nil, appErrors.Internal(err, "error reading address rows")
	}

	return addresses, nil
}

func (r *pgAddressRepository) Update(ctx context.Context, address *model.Address) error {
	query := `
		UPDATE user_addresses
		SET label = $1, full_name = $2, phone_number = $3, email_address = $4, address_line = $5, city = $6, state = $7, postal_code = $8, country = $9, is_default = $10, country_id = $11, updated_at = NOW()
		WHERE id = $12 AND user_id = $13
		RETURNING updated_at
	`
	err := r.pool.QueryRow(ctx, query,
		address.Label,
		address.FullName,
		address.PhoneNumber,
		address.EmailAddress,
		address.AddressLine,
		address.City,
		address.State,
		address.PostalCode,
		address.Country,
		address.IsDefault,
		address.CountryID,
		address.ID,
		address.UserID,
	).Scan(&address.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return appErrors.NotFound("address not found")
		}
		return appErrors.Internal(err, "failed to update address")
	}
	return nil
}

func (r *pgAddressRepository) Delete(ctx context.Context, id string, userID string) error {
	query := `
		DELETE FROM user_addresses
		WHERE id = $1 AND user_id = $2
	`
	ct, err := r.pool.Exec(ctx, query, id, userID)
	if err != nil {
		return appErrors.Internal(err, "failed to delete address")
	}
	if ct.RowsAffected() == 0 {
		return appErrors.NotFound("address not found")
	}
	return nil
}

func (r *pgAddressRepository) ClearDefaultAddresses(ctx context.Context, userID string) error {
	query := `
		UPDATE user_addresses
		SET is_default = FALSE
		WHERE user_id = $1 AND is_default = TRUE
	`
	_, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return appErrors.Internal(err, "failed to clear default addresses")
	}
	return nil
}

func (r *pgAddressRepository) SetDefaultAddress(ctx context.Context, id string, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	clearQuery := `UPDATE user_addresses SET is_default = FALSE WHERE user_id = $1 AND is_default = TRUE`
	if _, err := tx.Exec(ctx, clearQuery, userID); err != nil {
		return appErrors.Internal(err, "failed to clear existing default address")
	}

	setQuery := `UPDATE user_addresses SET is_default = TRUE, updated_at = NOW() WHERE id = $1 AND user_id = $2`
	ct, err := tx.Exec(ctx, setQuery, id, userID)
	if err != nil {
		return appErrors.Internal(err, "failed to set default address")
	}
	if ct.RowsAffected() == 0 {
		return appErrors.NotFound("address not found")
	}

	if err := tx.Commit(ctx); err != nil {
		return appErrors.Internal(err, "failed to commit transaction")
	}
	return nil
}

func (r *pgAddressRepository) CountByUserID(ctx context.Context, userID string) (int, error) {
	query := `SELECT COUNT(*) FROM user_addresses WHERE user_id = $1`
	var count int
	err := r.pool.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, appErrors.Internal(err, "failed to count user addresses")
	}
	return count, nil
}
