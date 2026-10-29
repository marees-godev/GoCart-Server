package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/model"
)

type WarehouseRepository interface {
	CreateWarehouse(ctx context.Context, w *model.Warehouse) error
	GetWarehouseByID(ctx context.Context, partnerID, id uuid.UUID) (*model.Warehouse, error)
	GetWarehousesByPartnerID(ctx context.Context, partnerID uuid.UUID) ([]*model.Warehouse, error)
	GetAvailableWarehouses(ctx context.Context) ([]*model.Warehouse, error)
	UpdateWarehouse(ctx context.Context, w *model.Warehouse) (*model.Warehouse, error)
	DeleteWarehouse(ctx context.Context, partnerID, id uuid.UUID) error
}

type pgWarehouseRepository struct {
	pool *pgxpool.Pool
}

func NewWarehouseRepository(pool *pgxpool.Pool) WarehouseRepository {
	return &pgWarehouseRepository{pool: pool}
}

func (r *pgWarehouseRepository) CreateWarehouse(ctx context.Context, w *model.Warehouse) error {
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
	_, err := r.pool.Exec(ctx, query, w.ID, w.DeliveryPartnerID, w.Name, w.Code, w.Street, w.City, w.State, w.PostalCode, w.IsActive, w.CreatedAt, w.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return appErrors.Conflict("warehouse code already exists for this delivery partner")
		}
		return appErrors.Internal(err, "failed to create warehouse")
	}
	return nil
}

func (r *pgWarehouseRepository) GetWarehouseByID(ctx context.Context, partnerID, id uuid.UUID) (*model.Warehouse, error) {
	if id == uuid.Nil {
		return nil, appErrors.BadRequest("warehouse id is required")
	}

	var query string
	var args []any
	if partnerID != uuid.Nil {
		query = `
			SELECT id, delivery_partner_id, name, code, street, city, state, postal_code, is_active, created_at, updated_at
			FROM warehouses
			WHERE id = $1 AND delivery_partner_id = $2
		`
		args = []any{id, partnerID}
	} else {
		query = `
			SELECT id, delivery_partner_id, name, code, street, city, state, postal_code, is_active, created_at, updated_at
			FROM warehouses
			WHERE id = $1
		`
		args = []any{id}
	}

	var w model.Warehouse
	err := r.pool.QueryRow(ctx, query, args...).Scan(
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

func (r *pgWarehouseRepository) GetWarehousesByPartnerID(ctx context.Context, partnerID uuid.UUID) ([]*model.Warehouse, error) {
	var query string
	var args []any
	if partnerID != uuid.Nil {
		query = `
			SELECT id, delivery_partner_id, name, code, street, city, state, postal_code, is_active, created_at, updated_at
			FROM warehouses
			WHERE delivery_partner_id = $1
			ORDER BY name ASC
		`
		args = []any{partnerID}
	} else {
		query = `
			SELECT id, delivery_partner_id, name, code, street, city, state, postal_code, is_active, created_at, updated_at
			FROM warehouses
			ORDER BY name ASC
		`
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to query warehouses")
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
			return nil, appErrors.Internal(err, "failed to scan warehouse")
		}
		warehouses = append(warehouses, &w)
	}
	if warehouses == nil {
		warehouses = make([]*model.Warehouse, 0)
	}
	return warehouses, nil
}

func (r *pgWarehouseRepository) GetAvailableWarehouses(ctx context.Context) ([]*model.Warehouse, error) {
	query := `
		SELECT id, delivery_partner_id, name, code, street, city, state, postal_code, is_active, created_at, updated_at
		FROM warehouses
		WHERE is_active = TRUE
		ORDER BY name ASC
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
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
			return nil, appErrors.Internal(err, "failed to scan warehouse")
		}
		warehouses = append(warehouses, &w)
	}
	if warehouses == nil {
		warehouses = make([]*model.Warehouse, 0)
	}
	return warehouses, nil
}

func (r *pgWarehouseRepository) UpdateWarehouse(ctx context.Context, w *model.Warehouse) (*model.Warehouse, error) {
	if w.ID == uuid.Nil {
		return nil, appErrors.BadRequest("warehouse id is required")
	}
	w.UpdatedAt = time.Now().UTC()

	var query string
	var args []any
	if w.DeliveryPartnerID != uuid.Nil {
		query = `
			UPDATE warehouses
			SET name = $1, code = $2, street = $3, city = $4, state = $5, postal_code = $6, is_active = $7, updated_at = $8
			WHERE id = $9 AND delivery_partner_id = $10
			RETURNING id, delivery_partner_id, name, code, street, city, state, postal_code, is_active, created_at, updated_at
		`
		args = []any{
			w.Name, w.Code, w.Street, w.City, w.State, w.PostalCode, w.IsActive, w.UpdatedAt,
			w.ID, w.DeliveryPartnerID,
		}
	} else {
		query = `
			UPDATE warehouses
			SET name = $1, code = $2, street = $3, city = $4, state = $5, postal_code = $6, is_active = $7, updated_at = $8
			WHERE id = $9
			RETURNING id, delivery_partner_id, name, code, street, city, state, postal_code, is_active, created_at, updated_at
		`
		args = []any{
			w.Name, w.Code, w.Street, w.City, w.State, w.PostalCode, w.IsActive, w.UpdatedAt,
			w.ID,
		}
	}

	var updated model.Warehouse
	err := r.pool.QueryRow(ctx, query, args...).Scan(
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
		return nil, appErrors.Internal(err, "failed to update warehouse")
	}
	return &updated, nil
}

func (r *pgWarehouseRepository) DeleteWarehouse(ctx context.Context, partnerID, id uuid.UUID) error {
	if id == uuid.Nil {
		return appErrors.BadRequest("warehouse id is required")
	}

	var query string
	var args []any
	if partnerID != uuid.Nil {
		query = `DELETE FROM warehouses WHERE id = $1 AND delivery_partner_id = $2`
		args = []any{id, partnerID}
	} else {
		query = `DELETE FROM warehouses WHERE id = $1`
		args = []any{id}
	}

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return appErrors.Internal(err, "failed to delete warehouse")
	}
	if tag.RowsAffected() == 0 {
		return appErrors.NotFound("warehouse not found")
	}
	return nil
}
