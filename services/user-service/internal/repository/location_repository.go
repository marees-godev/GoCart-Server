package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
)

type LocationRepository interface {
	GetByPincode(ctx context.Context, pincode string) (*model.Location, error)
	Save(ctx context.Context, loc *model.Location) error
}

type locationRepository struct {
	pool *pgxpool.Pool
}

func NewLocationRepository(pool *pgxpool.Pool) LocationRepository {
	return &locationRepository{pool: pool}
}

func (r *locationRepository) GetByPincode(ctx context.Context, pincode string) (*model.Location, error) {
	locQuery := `
		SELECT id, pincode, country, state, district, created_at, updated_at
		FROM locations
		WHERE pincode = $1
	`

	var loc model.Location
	err := r.pool.QueryRow(ctx, locQuery, pincode).Scan(
		&loc.ID,
		&loc.Pincode,
		&loc.Country,
		&loc.State,
		&loc.District,
		&loc.CreatedAt,
		&loc.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query location by pincode: %w", err)
	}

	poQuery := `
		SELECT id, location_id, name, branch_type, delivery_status, circle, district, division, region, state, country, pincode, created_at
		FROM location_post_offices
		WHERE location_id = $1
		ORDER BY name ASC
	`

	rows, err := r.pool.Query(ctx, poQuery, loc.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to query post offices: %w", err)
	}
	defer rows.Close()

	var postOffices []model.PostOffice
	for rows.Next() {
		var po model.PostOffice
		if err := rows.Scan(
			&po.ID,
			&po.LocationID,
			&po.Name,
			&po.BranchType,
			&po.DeliveryStatus,
			&po.Circle,
			&po.District,
			&po.Division,
			&po.Region,
			&po.State,
			&po.Country,
			&po.Pincode,
			&po.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan post office: %w", err)
		}
		postOffices = append(postOffices, po)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating post offices: %w", err)
	}

	loc.PostOffices = postOffices
	return &loc, nil
}

func (r *locationRepository) Save(ctx context.Context, loc *model.Location) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if loc.ID == uuid.Nil {
		loc.ID = uuid.New()
	}
	now := time.Now()
	if loc.CreatedAt.IsZero() {
		loc.CreatedAt = now
	}
	loc.UpdatedAt = now

	locUpsert := `
		INSERT INTO locations (id, pincode, country, state, district, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (pincode) DO UPDATE
		SET country = EXCLUDED.country,
		    state = EXCLUDED.state,
		    district = EXCLUDED.district,
		    updated_at = EXCLUDED.updated_at
		RETURNING id
	`

	err = tx.QueryRow(ctx, locUpsert,
		loc.ID,
		loc.Pincode,
		loc.Country,
		loc.State,
		loc.District,
		loc.CreatedAt,
		loc.UpdatedAt,
	).Scan(&loc.ID)
	if err != nil {
		return fmt.Errorf("failed to upsert location: %w", err)
	}

	deletePOs := `DELETE FROM location_post_offices WHERE location_id = $1`
	if _, err := tx.Exec(ctx, deletePOs, loc.ID); err != nil {
		return fmt.Errorf("failed to clear existing post offices: %w", err)
	}

	insertPO := `
		INSERT INTO location_post_offices (
			id, location_id, name, branch_type, delivery_status, circle, district, division, region, state, country, pincode, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	for i := range loc.PostOffices {
		po := &loc.PostOffices[i]
		if po.ID == uuid.Nil {
			po.ID = uuid.New()
		}
		po.LocationID = loc.ID
		if po.CreatedAt.IsZero() {
			po.CreatedAt = now
		}

		if _, err := tx.Exec(ctx, insertPO,
			po.ID,
			po.LocationID,
			po.Name,
			po.BranchType,
			po.DeliveryStatus,
			po.Circle,
			po.District,
			po.Division,
			po.Region,
			po.State,
			po.Country,
			po.Pincode,
			po.CreatedAt,
		); err != nil {
			return fmt.Errorf("failed to insert post office %s: %w", po.Name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
