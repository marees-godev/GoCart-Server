package repository

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/model"
)

type CategoryRepository interface {
	CreateCategory(ctx context.Context, category *model.Category) error
	GetByID(ctx context.Context, id string) (*model.Category, error)
	GetByNameAndParent(ctx context.Context, name string, parentCategoryID *string) (*model.Category, error)
	ListCategory(ctx context.Context, filter dto.ListCategoriesRequest) ([]*model.Category, int32, error)
	GetChildren(ctx context.Context, parentCategoryID string, limit, offset int32) ([]*model.Category, int32, error)
	UpdateCategory(ctx context.Context, category *model.Category) error
	DeleteCategory(ctx context.Context, id string) error
	IsDescendant(ctx context.Context, candidateDescendantID, ancestorID string) (bool, error)
	HasChildren(ctx context.Context, parentCategoryID string) (bool, error)
}

type pgCategoryRepository struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewCategoryRepository(pool *pgxpool.Pool, log ...*slog.Logger) CategoryRepository {
	var l *slog.Logger
	if len(log) > 0 {
		l = log[0]
	}
	return &pgCategoryRepository{pool: pool, logger: l}
}

func (r *pgCategoryRepository) CreateCategory(ctx context.Context, cat *model.Category) error {
	query := `
		INSERT INTO categories (
			id, parent_category_id, name, description, is_active, created_at, updated_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()),
			NULLIF($2, '')::uuid,
			$3, $4, $5, NOW(), NOW()
		)
		RETURNING id, created_at, updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		cat.ID,
		cat.ParentCategoryID,
		cat.Name,
		cat.Description,
		cat.IsActive,
	).Scan(&cat.ID, &cat.CreatedAt, &cat.UpdatedAt)

	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to insert category", "error", err, "name", cat.Name)
		}
		return handlePGError(err)
	}

	if r.logger != nil {
		r.logger.Debug("Category inserted into db", "id", cat.ID, "name", cat.Name)
	}

	return nil
}

func (r *pgCategoryRepository) GetByID(ctx context.Context, id string) (*model.Category, error) {
	query := `
		SELECT id, parent_category_id, name, COALESCE(description, ''), is_active, created_at, updated_at, deleted_at
		FROM categories
		WHERE id = $1 AND deleted_at IS NULL
	`

	var cat model.Category
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&cat.ID,
		&cat.ParentCategoryID,
		&cat.Name,
		&cat.Description,
		&cat.IsActive,
		&cat.CreatedAt,
		&cat.UpdatedAt,
		&cat.DeletedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("category not found")
		}
		if r.logger != nil {
			r.logger.Error("Failed to get category by ID", "error", err, "id", id)
		}
		return nil, appErrors.Internal(err, "failed to get category by ID")
	}

	return &cat, nil
}

func (r *pgCategoryRepository) GetByNameAndParent(ctx context.Context, name string, parentCategoryID *string) (*model.Category, error) {
	var query string
	var args []any

	if parentCategoryID == nil || *parentCategoryID == "" {
		query = `
			SELECT id, parent_category_id, name, COALESCE(description, ''), is_active, created_at, updated_at, deleted_at
			FROM categories
			WHERE LOWER(name) = LOWER($1) AND parent_category_id IS NULL AND deleted_at IS NULL
		`
		args = []any{name}
	} else {
		query = `
			SELECT id, parent_category_id, name, COALESCE(description, ''), is_active, created_at, updated_at, deleted_at
			FROM categories
			WHERE LOWER(name) = LOWER($1) AND parent_category_id = $2 AND deleted_at IS NULL
		`
		args = []any{name, *parentCategoryID}
	}

	var cat model.Category
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&cat.ID,
		&cat.ParentCategoryID,
		&cat.Name,
		&cat.Description,
		&cat.IsActive,
		&cat.CreatedAt,
		&cat.UpdatedAt,
		&cat.DeletedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("category not found")
		}
		if r.logger != nil {
			r.logger.Error("Failed to get category by name and parent", "error", err, "name", name)
		}
		return nil, appErrors.Internal(err, "failed to get category by name and parent")
	}

	return &cat, nil
}

func (r *pgCategoryRepository) ListCategory(ctx context.Context, filter dto.ListCategoriesRequest) ([]*model.Category, int32, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	countQuery := `
		SELECT COUNT(*)
		FROM categories
		WHERE deleted_at IS NULL
		  AND ($1::uuid IS NULL OR parent_category_id = $1)
		  AND (NOT $2::bool OR parent_category_id IS NULL)
	`

	var total int64
	err := r.pool.QueryRow(ctx, countQuery, filter.ParentCategoryID, filter.RootOnly).Scan(&total)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to count categories", "error", err)
		}
		return nil, 0, appErrors.Internal(err, "failed to count categories")
	}

	query := `
		SELECT id, parent_category_id, name, COALESCE(description, ''), is_active, created_at, updated_at, deleted_at
		FROM categories
		WHERE deleted_at IS NULL
		  AND ($1::uuid IS NULL OR parent_category_id = $1)
		  AND (NOT $2::bool OR parent_category_id IS NULL)
		ORDER BY created_at ASC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.pool.Query(ctx, query, filter.ParentCategoryID, filter.RootOnly, limit, offset)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to list categories", "error", err)
		}
		return nil, 0, appErrors.Internal(err, "failed to list categories")
	}
	defer rows.Close()

	var categories []*model.Category
	for rows.Next() {
		var cat model.Category
		if err := rows.Scan(
			&cat.ID,
			&cat.ParentCategoryID,
			&cat.Name,
			&cat.Description,
			&cat.IsActive,
			&cat.CreatedAt,
			&cat.UpdatedAt,
			&cat.DeletedAt,
		); err != nil {
			if r.logger != nil {
				r.logger.Error("Failed to scan category row", "error", err)
			}
			return nil, 0, appErrors.Internal(err, "failed to scan category row")
		}
		categories = append(categories, &cat)
	}

	return categories, int32(total), nil
}

func (r *pgCategoryRepository) GetChildren(ctx context.Context, parentCategoryID string, limit, offset int32) ([]*model.Category, int32, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	countQuery := `SELECT COUNT(*) FROM categories WHERE parent_category_id = $1 AND deleted_at IS NULL`
	var total int64
	err := r.pool.QueryRow(ctx, countQuery, parentCategoryID).Scan(&total)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to count child categories", "error", err, "parent_id", parentCategoryID)
		}
		return nil, 0, appErrors.Internal(err, "failed to count child categories")
	}

	query := `
		SELECT id, parent_category_id, name, COALESCE(description, ''), is_active, created_at, updated_at, deleted_at
		FROM categories
		WHERE parent_category_id = $1 AND deleted_at IS NULL
		ORDER BY created_at ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(ctx, query, parentCategoryID, limit, offset)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to list child categories", "error", err, "parent_id", parentCategoryID)
		}
		return nil, 0, appErrors.Internal(err, "failed to list child categories")
	}
	defer rows.Close()

	var categories []*model.Category
	for rows.Next() {
		var cat model.Category
		if err := rows.Scan(
			&cat.ID,
			&cat.ParentCategoryID,
			&cat.Name,
			&cat.Description,
			&cat.IsActive,
			&cat.CreatedAt,
			&cat.UpdatedAt,
			&cat.DeletedAt,
		); err != nil {
			if r.logger != nil {
				r.logger.Error("Failed to scan child category row", "error", err)
			}
			return nil, 0, appErrors.Internal(err, "failed to scan child category row")
		}
		categories = append(categories, &cat)
	}

	return categories, int32(total), nil
}

func (r *pgCategoryRepository) UpdateCategory(ctx context.Context, cat *model.Category) error {
	query := `
		UPDATE categories
		SET name = $2,
		    parent_category_id = NULLIF($3, '')::uuid,
		    description = $4,
		    is_active = $5,
		    updated_at = NOW()
		WHERE id = $1::uuid AND deleted_at IS NULL
		RETURNING updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		cat.ID,
		cat.Name,
		cat.ParentCategoryID,
		cat.Description,
		cat.IsActive,
	).Scan(&cat.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return appErrors.NotFound("category not found")
		}
		if r.logger != nil {
			r.logger.Error("Failed to update category", "error", err, "id", cat.ID)
		}
		return handlePGError(err)
	}

	if r.logger != nil {
		r.logger.Debug("Category updated in db", "id", cat.ID)
	}

	return nil
}

func (r *pgCategoryRepository) DeleteCategory(ctx context.Context, id string) error {
	query := `
		UPDATE categories
		SET deleted_at = NOW(),
		    is_active = false,
		    updated_at = NOW()
		WHERE id = $1::uuid AND deleted_at IS NULL
	`
	cmdTag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to soft-delete category", "error", err, "id", id)
		}
		return appErrors.Internal(err, "failed to delete category")
	}

	if cmdTag.RowsAffected() == 0 {
		return appErrors.NotFound("category not found")
	}

	if r.logger != nil {
		r.logger.Debug("Category soft-deleted from db", "id", id)
	}

	return nil
}

func (r *pgCategoryRepository) HasChildren(ctx context.Context, parentCategoryID string) (bool, error) {
	query := `SELECT EXISTS (SELECT 1 FROM categories WHERE parent_category_id = $1::uuid AND deleted_at IS NULL)`
	var hasChildren bool
	err := r.pool.QueryRow(ctx, query, parentCategoryID).Scan(&hasChildren)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to check child categories", "error", err, "parent_id", parentCategoryID)
		}
		return false, appErrors.Internal(err, "failed to check child categories")
	}

	return hasChildren, nil
}

func (r *pgCategoryRepository) IsDescendant(ctx context.Context, candidateDescendantID, ancestorID string) (bool, error) {
	if candidateDescendantID == "" || ancestorID == "" {
		return false, nil
	}
	if candidateDescendantID == ancestorID {
		return true, nil
	}

	query := `
		WITH RECURSIVE cat_ancestors AS (
			SELECT id, parent_category_id FROM categories WHERE id = $1 AND deleted_at IS NULL
			UNION ALL
			SELECT c.id, c.parent_category_id FROM categories c
			INNER JOIN cat_ancestors ca ON c.id = ca.parent_category_id
			WHERE c.deleted_at IS NULL
		)
		SELECT EXISTS (SELECT 1 FROM cat_ancestors WHERE id = $2);
	`

	var isDescendant bool
	err := r.pool.QueryRow(ctx, query, candidateDescendantID, ancestorID).Scan(&isDescendant)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to check ancestor hierarchy", "error", err, "candidate", candidateDescendantID, "ancestor", ancestorID)
		}
		return false, appErrors.Internal(err, "failed to check category ancestor hierarchy")
	}

	return isDescendant, nil
}

func handlePGError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return appErrors.Conflict("category name already exists in this parent scope")
		case "23503": // foreign_key_violation
			return appErrors.NotFound("parent category not found")
		}
	}
	return appErrors.Internal(err, "database error")
}
