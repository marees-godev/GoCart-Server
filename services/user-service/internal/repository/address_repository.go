package repository

import (
	"context"
	"errors"
	"log/slog"

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
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin transaction in Create", "user_id", address.UserID, "error", err)
		return appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('user_default_address'), hashtext($1))`, address.UserID); err != nil {
		slog.ErrorContext(ctx, "failed to acquire advisory lock in Create", "user_id", address.UserID, "error", err)
		return appErrors.Internal(err, "failed to acquire advisory lock")
	}

	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM user_addresses WHERE user_id = $1`, address.UserID).Scan(&count); err != nil {
		slog.ErrorContext(ctx, "failed to count user addresses in Create", "user_id", address.UserID, "error", err)
		return appErrors.Internal(err, "failed to count user addresses")
	}

	if count == 0 {
		address.IsDefault = true
	}

	if address.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE user_addresses SET is_default = FALSE WHERE user_id = $1 AND is_default = TRUE`, address.UserID); err != nil {
			slog.ErrorContext(ctx, "failed to clear existing default address in Create", "user_id", address.UserID, "error", err)
			return appErrors.Internal(err, "failed to clear existing default address")
		}
	}

	query := `
		INSERT INTO user_addresses (user_id, country_id, label, full_name, phone_number, email_address, address_line, city, state, postal_code, country, is_default)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at
	`
	err = tx.QueryRow(ctx, query,
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
		slog.ErrorContext(ctx, "failed to insert user address in Create", "user_id", address.UserID, "error", err)
		return appErrors.Internal(err, "failed to create user address")
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit transaction in Create", "user_id", address.UserID, "error", err)
		return appErrors.Internal(err, "failed to commit transaction")
	}
	slog.InfoContext(ctx, "user address created in repository", "address_id", address.ID, "user_id", address.UserID, "is_default", address.IsDefault)
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
			slog.WarnContext(ctx, "address not found in repository", "address_id", id)
			return nil, appErrors.NotFound("address not found")
		}
		slog.ErrorContext(ctx, "failed to query address by id in repository", "address_id", id, "error", err)
		return nil, appErrors.Internal(err, "failed to query address by id")
	}
	slog.InfoContext(ctx, "address retrieved in repository", "address_id", a.ID, "user_id", a.UserID)
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
		slog.ErrorContext(ctx, "failed to list user addresses in repository", "user_id", userID, "error", err)
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
			slog.ErrorContext(ctx, "failed to scan address row in repository", "user_id", userID, "error", err)
			return nil, appErrors.Internal(err, "failed to scan address row")
		}
		addresses = append(addresses, &a)
	}

	if err := rows.Err(); err != nil {
		slog.ErrorContext(ctx, "error reading address rows in repository", "user_id", userID, "error", err)
		return nil, appErrors.Internal(err, "error reading address rows")
	}

	slog.InfoContext(ctx, "user addresses listed in repository", "user_id", userID, "count", len(addresses))
	return addresses, nil
}

func (r *pgAddressRepository) Update(ctx context.Context, address *model.Address) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin transaction in Update", "address_id", address.ID, "user_id", address.UserID, "error", err)
		return appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('user_default_address'), hashtext($1))`, address.UserID); err != nil {
		slog.ErrorContext(ctx, "failed to acquire advisory lock in Update", "address_id", address.ID, "user_id", address.UserID, "error", err)
		return appErrors.Internal(err, "failed to acquire advisory lock")
	}

	if address.IsDefault {
		clearQuery := `UPDATE user_addresses SET is_default = FALSE WHERE user_id = $1 AND is_default = TRUE AND id != $2`
		if _, err := tx.Exec(ctx, clearQuery, address.UserID, address.ID); err != nil {
			slog.ErrorContext(ctx, "failed to clear existing default address in Update", "address_id", address.ID, "user_id", address.UserID, "error", err)
			return appErrors.Internal(err, "failed to clear existing default address")
		}
	}

	query := `
		UPDATE user_addresses
		SET label = $1, full_name = $2, phone_number = $3, email_address = $4, address_line = $5, city = $6, state = $7, postal_code = $8, country = $9, is_default = $10, country_id = $11, updated_at = NOW()
		WHERE id = $12 AND user_id = $13
		RETURNING updated_at
	`
	err = tx.QueryRow(ctx, query,
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
			slog.WarnContext(ctx, "address not found for update in repository", "address_id", address.ID, "user_id", address.UserID)
			return appErrors.NotFound("address not found")
		}
		slog.ErrorContext(ctx, "failed to update address in repository", "address_id", address.ID, "user_id", address.UserID, "error", err)
		return appErrors.Internal(err, "failed to update address")
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit transaction in Update", "address_id", address.ID, "user_id", address.UserID, "error", err)
		return appErrors.Internal(err, "failed to commit transaction")
	}
	slog.InfoContext(ctx, "address updated in repository", "address_id", address.ID, "user_id", address.UserID, "is_default", address.IsDefault)
	return nil
}

func (r *pgAddressRepository) Delete(ctx context.Context, id string, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin transaction in Delete", "address_id", id, "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('user_default_address'), hashtext($1))`, userID); err != nil {
		slog.ErrorContext(ctx, "failed to acquire advisory lock in Delete", "address_id", id, "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to acquire advisory lock")
	}

	var wasDefault bool
	err = tx.QueryRow(ctx, `SELECT is_default FROM user_addresses WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, userID).Scan(&wasDefault)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "address not found for deletion in repository", "address_id", id, "user_id", userID)
			return appErrors.NotFound("address not found")
		}
		slog.ErrorContext(ctx, "failed to query address before deletion in repository", "address_id", id, "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to query address before deletion")
	}

	deleteQuery := `DELETE FROM user_addresses WHERE id = $1 AND user_id = $2`
	if _, err := tx.Exec(ctx, deleteQuery, id, userID); err != nil {
		slog.ErrorContext(ctx, "failed to delete address in repository", "address_id", id, "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to delete address")
	}

	if wasDefault {
		var candidateID string
		candQuery := `
			SELECT id FROM user_addresses
			WHERE user_id = $1
			ORDER BY updated_at DESC, created_at DESC
			LIMIT 1
			FOR UPDATE
		`
		candErr := tx.QueryRow(ctx, candQuery, userID).Scan(&candidateID)
		if candErr == nil {
			promoteQuery := `UPDATE user_addresses SET is_default = TRUE, updated_at = NOW() WHERE id = $1`
			if _, err := tx.Exec(ctx, promoteQuery, candidateID); err != nil {
				slog.ErrorContext(ctx, "failed to promote new default address in repository", "address_id", candidateID, "user_id", userID, "error", err)
				return appErrors.Internal(err, "failed to promote new default address")
			}
		} else if !errors.Is(candErr, pgx.ErrNoRows) {
			slog.ErrorContext(ctx, "failed to find candidate default address in repository", "user_id", userID, "error", candErr)
			return appErrors.Internal(candErr, "failed to find candidate default address")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit transaction in Delete", "address_id", id, "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to commit transaction")
	}
	slog.InfoContext(ctx, "address deleted in repository", "address_id", id, "user_id", userID, "was_default", wasDefault)
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
		slog.ErrorContext(ctx, "failed to clear default addresses in repository", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to clear default addresses")
	}
	slog.InfoContext(ctx, "default addresses cleared in repository", "user_id", userID)
	return nil
}

func (r *pgAddressRepository) SetDefaultAddress(ctx context.Context, id string, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin transaction in SetDefaultAddress", "address_id", id, "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('user_default_address'), hashtext($1))`, userID); err != nil {
		slog.ErrorContext(ctx, "failed to acquire advisory lock in SetDefaultAddress", "address_id", id, "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to acquire advisory lock")
	}

	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_addresses WHERE id = $1 AND user_id = $2)`, id, userID).Scan(&exists)
	if err != nil {
		slog.ErrorContext(ctx, "failed to check address existence in SetDefaultAddress", "address_id", id, "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to check address existence")
	}
	if !exists {
		slog.WarnContext(ctx, "address not found in SetDefaultAddress", "address_id", id, "user_id", userID)
		return appErrors.NotFound("address not found")
	}

	clearQuery := `UPDATE user_addresses SET is_default = FALSE WHERE user_id = $1 AND is_default = TRUE`
	if _, err := tx.Exec(ctx, clearQuery, userID); err != nil {
		slog.ErrorContext(ctx, "failed to clear existing default address in SetDefaultAddress", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to clear existing default address")
	}

	setQuery := `UPDATE user_addresses SET is_default = TRUE, updated_at = NOW() WHERE id = $1 AND user_id = $2`
	if _, err := tx.Exec(ctx, setQuery, id, userID); err != nil {
		slog.ErrorContext(ctx, "failed to set default address in SetDefaultAddress", "address_id", id, "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to set default address")
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit transaction in SetDefaultAddress", "address_id", id, "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to commit transaction")
	}
	slog.InfoContext(ctx, "default address set in repository", "address_id", id, "user_id", userID)
	return nil
}

func (r *pgAddressRepository) CountByUserID(ctx context.Context, userID string) (int, error) {
	query := `SELECT COUNT(*) FROM user_addresses WHERE user_id = $1`
	var count int
	err := r.pool.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		slog.ErrorContext(ctx, "failed to count user addresses in repository", "user_id", userID, "error", err)
		return 0, appErrors.Internal(err, "failed to count user addresses")
	}
	slog.DebugContext(ctx, "counted user addresses in repository", "user_id", userID, "count", count)
	return count, nil
}
