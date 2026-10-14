package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/dto"
	productEvents "github.com/marees-godev/GoCart-Server/services/product-service/internal/events"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/model"
)

type ProductRepository interface {
	CreateProduct(ctx context.Context, p *model.Product) error
	GetProductByID(ctx context.Context, id string) (*model.Product, error)
	GetProductBySKU(ctx context.Context, storeID, sku string) (*model.Product, error)
	ListProducts(ctx context.Context, filter dto.ListProductsRequest) ([]*model.Product, int32, error)
	UpdateProduct(ctx context.Context, p *model.Product) error
	DeleteProduct(ctx context.Context, id string) error
	DeleteProductVariant(ctx context.Context, productID, variantID string) error
}

type pgProductRepository struct {
	pool        *pgxpool.Pool
	outboxStore *outbox.Store
	logger      *slog.Logger
}

func NewProductRepository(pool *pgxpool.Pool, log ...*slog.Logger) ProductRepository {
	return NewProductRepositoryWithOutbox(pool, outbox.NewStore(), log...)
}

func NewProductRepositoryWithOutbox(pool *pgxpool.Pool, outboxStore *outbox.Store, log ...*slog.Logger) ProductRepository {
	if outboxStore == nil {
		outboxStore = outbox.NewStore()
	}
	var l *slog.Logger
	if len(log) > 0 {
		l = log[0]
	}
	return &pgProductRepository{
		pool:        pool,
		outboxStore: outboxStore,
		logger:      l,
	}
}

func (r *pgProductRepository) CreateProduct(ctx context.Context, p *model.Product) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return appErrors.Internal(err, "failed to start database transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	query := `
		INSERT INTO products (
			store_id, category_id, sku, name, description, price, mrp, tax, currency, status, image_url, avg_rating
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		) RETURNING id, created_at, updated_at
	`

	currency := p.Currency
	if currency == "" {
		currency = "USD"
	}

	err = tx.QueryRow(ctx, query,
		p.StoreID, p.CategoryID, p.SKU, p.Name, p.Description,
		p.Price, p.MRP, p.Tax, currency, p.Status, p.ImageURL, p.AvgRating,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		if strings.Contains(err.Error(), "products_store_id_sku_key") || strings.Contains(err.Error(), "duplicate key") {
			return appErrors.AlreadyExists("product SKU already exists in this store")
		}
		return appErrors.Internal(err, "failed to insert product")
	}

	// Insert Images
	for i, img := range p.Images {
		imgQuery := `
			INSERT INTO product_images (product_id, url, alt_text, is_primary, display_order)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, created_at
		`
		err = tx.QueryRow(ctx, imgQuery, p.ID, img.URL, img.AltText, img.IsPrimary, i).Scan(&img.ID, &img.CreatedAt)
		if err != nil {
			return appErrors.Internal(err, "failed to insert product image")
		}
		img.ProductID = p.ID
		p.Images[i] = img
	}

	// Insert Variants
	for i, v := range p.Variants {
		var existingVarID string
		err = tx.QueryRow(ctx, `
			SELECT pv.id
			FROM product_variants pv
			JOIN products p ON pv.product_id = p.id
			WHERE p.store_id = $1 AND pv.sku = $2 AND pv.deleted_at IS NULL
		`, p.StoreID, v.SKU).Scan(&existingVarID)
		if err == nil {
			return appErrors.AlreadyExists(fmt.Sprintf("variant SKU %s already exists in this store", v.SKU))
		}

		vQuery := `
			INSERT INTO product_variants (product_id, sku, name, price, mrp, stock, attributes, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, created_at, updated_at
		`
		attr := v.AttributesJSON
		if attr == "" {
			attr = "{}"
		}

		vStatus := v.Status
		if vStatus == "" {
			vStatus = model.StatusInStock
		}

		err = tx.QueryRow(ctx, vQuery, p.ID, v.SKU, v.Name, v.Price, v.MRP, v.Stock, attr, vStatus).Scan(&v.ID, &v.CreatedAt, &v.UpdatedAt)
		if err != nil {
			if strings.Contains(err.Error(), "product_variants_product_id_sku_key") || strings.Contains(err.Error(), "duplicate key") {
				return appErrors.AlreadyExists(fmt.Sprintf("variant SKU %s already exists for this product", v.SKU))
			}
			return appErrors.Internal(err, "failed to insert product variant")
		}
		v.ProductID = p.ID
		p.Variants[i] = v
	}

	// Insert Outbox Events
	outboxEvt, err := productEvents.BuildProductCreatedOutboxEvent(p)
	if err != nil {
		return appErrors.Internal(err, "failed to build ProductCreated outbox event")
	}
	if err := r.outboxStore.Insert(ctx, tx, outboxEvt); err != nil {
		return appErrors.Internal(err, "failed to record outbox event for ProductCreated")
	}

	if p.Status == model.StatusPublished {
		pubEvt, err := productEvents.BuildProductPublishedOutboxEvent(p, p.CreatedAt)
		if err != nil {
			return appErrors.Internal(err, "failed to build ProductPublished outbox event")
		}
		if err := r.outboxStore.Insert(ctx, tx, pubEvt); err != nil {
			return appErrors.Internal(err, "failed to record outbox event for ProductPublished")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return appErrors.Internal(err, "failed to commit transaction")
	}

	return nil
}

func (r *pgProductRepository) GetProductByID(ctx context.Context, id string) (*model.Product, error) {
	query := `
		SELECT id, store_id, category_id, sku, name, description, price, mrp, tax, currency, status, COALESCE(image_url, ''), avg_rating, created_at, updated_at, deleted_at
		FROM products
		WHERE id = $1 AND deleted_at IS NULL
	`

	var p model.Product
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.StoreID, &p.CategoryID, &p.SKU, &p.Name, &p.Description,
		&p.Price, &p.MRP, &p.Tax, &p.Currency, &p.Status, &p.ImageURL, &p.AvgRating, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("product not found")
		}
		return nil, appErrors.Internal(err, "failed to fetch product")
	}

	// Fetch Images
	imgRows, err := r.pool.Query(ctx, `
		SELECT id, product_id, url, COALESCE(alt_text, ''), is_primary, display_order, created_at
		FROM product_images
		WHERE product_id = $1
		ORDER BY display_order ASC
	`, p.ID)
	if err == nil {
		defer imgRows.Close()
		for imgRows.Next() {
			var img model.ProductImage
			if err := imgRows.Scan(&img.ID, &img.ProductID, &img.URL, &img.AltText, &img.IsPrimary, &img.DisplayOrder, &img.CreatedAt); err == nil {
				p.Images = append(p.Images, img)
			}
		}
	}

	// Fetch Variants
	vRows, err := r.pool.Query(ctx, `
		SELECT id, product_id, sku, name, price, mrp, stock, attributes, status, created_at, updated_at, deleted_at
		FROM product_variants
		WHERE product_id = $1 AND deleted_at IS NULL
		ORDER BY created_at ASC
	`, p.ID)
	if err == nil {
		defer vRows.Close()
		for vRows.Next() {
			var v model.ProductVariant
			var attrBytes []byte
			if err := vRows.Scan(&v.ID, &v.ProductID, &v.SKU, &v.Name, &v.Price, &v.MRP, &v.Stock, &attrBytes, &v.Status, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt); err == nil {
				v.AttributesJSON = string(attrBytes)
				p.Variants = append(p.Variants, v)
			}
		}
	}

	return &p, nil
}

func (r *pgProductRepository) GetProductBySKU(ctx context.Context, storeID, sku string) (*model.Product, error) {
	query := `
		SELECT id, store_id, category_id, sku, name, description, price, mrp, tax, currency, status, COALESCE(image_url, ''), avg_rating, created_at, updated_at, deleted_at
		FROM products
		WHERE store_id = $1 AND sku = $2 AND deleted_at IS NULL
	`

	var p model.Product
	err := r.pool.QueryRow(ctx, query, storeID, sku).Scan(
		&p.ID, &p.StoreID, &p.CategoryID, &p.SKU, &p.Name, &p.Description,
		&p.Price, &p.MRP, &p.Tax, &p.Currency, &p.Status, &p.ImageURL, &p.AvgRating, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("product not found")
		}
		return nil, appErrors.Internal(err, "failed to fetch product by sku")
	}

	return &p, nil
}

func (r *pgProductRepository) ListProducts(ctx context.Context, filter dto.ListProductsRequest) ([]*model.Product, int32, error) {
	whereClauses := []string{"deleted_at IS NULL"}
	args := []interface{}{}
	argPos := 1

	if filter.StoreID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("store_id = $%d", argPos))
		args = append(args, filter.StoreID)
		argPos++
	}

	if filter.CategoryID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("category_id = $%d", argPos))
		args = append(args, filter.CategoryID)
		argPos++
	}

	if filter.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argPos))
		args = append(args, filter.Status)
		argPos++
	}

	whereStmt := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM products WHERE %s", whereStmt)
	var total int32
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, appErrors.Internal(err, "failed to count products")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	listQuery := fmt.Sprintf(`
		SELECT id, store_id, category_id, sku, name, description, price, mrp, tax, currency, status, COALESCE(image_url, ''), avg_rating, created_at, updated_at, deleted_at
		FROM products
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereStmt, argPos, argPos+1)

	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, appErrors.Internal(err, "failed to list products")
	}
	defer rows.Close()

	products := []*model.Product{}
	for rows.Next() {
		var p model.Product
		err := rows.Scan(
			&p.ID, &p.StoreID, &p.CategoryID, &p.SKU, &p.Name, &p.Description,
			&p.Price, &p.MRP, &p.Tax, &p.Currency, &p.Status, &p.ImageURL, &p.AvgRating, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
		)
		if err != nil {
			return nil, 0, appErrors.Internal(err, "failed to scan product row")
		}
		products = append(products, &p)
	}

	// Batch load images & variants for listed products
	for _, p := range products {
		imgRows, err := r.pool.Query(ctx, `
			SELECT id, product_id, url, COALESCE(alt_text, ''), is_primary, display_order, created_at
			FROM product_images
			WHERE product_id = $1
			ORDER BY display_order ASC
		`, p.ID)
		if err == nil {
			for imgRows.Next() {
				var img model.ProductImage
				if err := imgRows.Scan(&img.ID, &img.ProductID, &img.URL, &img.AltText, &img.IsPrimary, &img.DisplayOrder, &img.CreatedAt); err == nil {
					p.Images = append(p.Images, img)
				}
			}
			imgRows.Close()
		}

		vRows, err := r.pool.Query(ctx, `
			SELECT id, product_id, sku, name, price, mrp, stock, attributes, status, created_at, updated_at, deleted_at
			FROM product_variants
			WHERE product_id = $1 AND deleted_at IS NULL
		`, p.ID)
		if err == nil {
			for vRows.Next() {
				var v model.ProductVariant
				var attrBytes []byte
				if err := vRows.Scan(&v.ID, &v.ProductID, &v.SKU, &v.Name, &v.Price, &v.MRP, &v.Stock, &attrBytes, &v.Status, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt); err == nil {
					v.AttributesJSON = string(attrBytes)
					p.Variants = append(p.Variants, v)
				}
			}
			vRows.Close()
		}
	}

	return products, total, nil
}

func (r *pgProductRepository) UpdateProduct(ctx context.Context, p *model.Product) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return appErrors.Internal(err, "failed to start database transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var oldStatus model.ProductStatus
	var oldCurrency, oldSKU string
	err = tx.QueryRow(ctx, `
		SELECT status, COALESCE(currency, 'USD'), sku
		FROM products
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, p.ID).Scan(&oldStatus, &oldCurrency, &oldSKU)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return appErrors.NotFound("product not found")
		}
		return appErrors.Internal(err, "failed to query product for update")
	}

	query := `
		UPDATE products
		SET category_id = $1, name = $2, description = $3, price = $4, mrp = $5, tax = $6, status = $7, image_url = $8, updated_at = NOW()
		WHERE id = $9 AND deleted_at IS NULL
		RETURNING updated_at
	`

	err = tx.QueryRow(ctx, query,
		p.CategoryID, p.Name, p.Description, p.Price, p.MRP, p.Tax, p.Status, p.ImageURL, p.ID,
	).Scan(&p.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return appErrors.NotFound("product not found")
		}
		return appErrors.Internal(err, "failed to update product")
	}

	// Update Images if provided
	if len(p.Images) > 0 {
		_, _ = tx.Exec(ctx, "DELETE FROM product_images WHERE product_id = $1", p.ID)
		for i, img := range p.Images {
			imgQuery := `
				INSERT INTO product_images (product_id, url, alt_text, is_primary, display_order)
				VALUES ($1, $2, $3, $4, $5)
				RETURNING id, created_at
			`
			_ = tx.QueryRow(ctx, imgQuery, p.ID, img.URL, img.AltText, img.IsPrimary, i).Scan(&img.ID, &img.CreatedAt)
			img.ProductID = p.ID
			p.Images[i] = img
		}
	}

	// Update Variants if provided
	if len(p.Variants) > 0 {
		for i, v := range p.Variants {
			if v.ID != "" {
				if v.SKU != "" {
					var existingVarID string
					err = tx.QueryRow(ctx, `
						SELECT pv.id
						FROM product_variants pv
						JOIN products p ON pv.product_id = p.id
						WHERE p.store_id = $1 AND pv.sku = $2 AND pv.deleted_at IS NULL AND pv.id != $3
					`, p.StoreID, v.SKU, v.ID).Scan(&existingVarID)
					if err == nil {
						return appErrors.AlreadyExists(fmt.Sprintf("variant SKU %s already exists in this store", v.SKU))
					}
				}

				vQuery := `
					UPDATE product_variants
					SET sku = $1, name = $2, price = $3, mrp = $4, stock = $5, attributes = $6, status = $7, updated_at = NOW()
					WHERE id = $8 AND product_id = $9 AND deleted_at IS NULL
				`
				attr := v.AttributesJSON
				if attr == "" {
					attr = "{}"
				}
				vStatus := v.Status
				if vStatus == "" {
					vStatus = model.StatusInStock
				}
				res, err := tx.Exec(ctx, vQuery, v.SKU, v.Name, v.Price, v.MRP, v.Stock, attr, vStatus, v.ID, p.ID)
				if err != nil {
					if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "product_variants_product_id_sku_key") {
						return appErrors.AlreadyExists(fmt.Sprintf("variant SKU %s already exists", v.SKU))
					}
					return appErrors.Internal(err, "failed to update variant")
				}
				if res.RowsAffected() == 0 {
					return appErrors.InvalidArgument(fmt.Sprintf("variant %s does not belong to product %s or does not exist", v.ID, p.ID))
				}
			} else {
				if v.SKU != "" {
					var existingVarID string
					err = tx.QueryRow(ctx, `
						SELECT pv.id
						FROM product_variants pv
						JOIN products p ON pv.product_id = p.id
						WHERE p.store_id = $1 AND pv.sku = $2 AND pv.deleted_at IS NULL
					`, p.StoreID, v.SKU).Scan(&existingVarID)
					if err == nil {
						return appErrors.AlreadyExists(fmt.Sprintf("variant SKU %s already exists in this store", v.SKU))
					}
				}

				vQuery := `
					INSERT INTO product_variants (product_id, sku, name, price, mrp, stock, attributes, status)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
					RETURNING id, created_at, updated_at
				`
				attr := v.AttributesJSON
				if attr == "" {
					attr = "{}"
				}
				vStatus := v.Status
				if vStatus == "" {
					vStatus = model.StatusInStock
				}
				err = tx.QueryRow(ctx, vQuery, p.ID, v.SKU, v.Name, v.Price, v.MRP, v.Stock, attr, vStatus).Scan(&v.ID, &v.CreatedAt, &v.UpdatedAt)
				if err != nil {
					if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "product_variants_product_id_sku_key") {
						return appErrors.AlreadyExists(fmt.Sprintf("variant SKU %s already exists", v.SKU))
					}
					return appErrors.Internal(err, "failed to insert variant during update")
				}
				v.ProductID = p.ID
				p.Variants[i] = v
			}
		}
	}

	if p.Currency == "" {
		p.Currency = oldCurrency
	}
	if p.SKU == "" {
		p.SKU = oldSKU
	}

	// Record outbox event for ProductUpdated
	outboxEvt, err := productEvents.BuildProductUpdatedOutboxEvent(p)
	if err != nil {
		return appErrors.Internal(err, "failed to build ProductUpdated outbox event")
	}
	if err := r.outboxStore.Insert(ctx, tx, outboxEvt); err != nil {
		return appErrors.Internal(err, "failed to record outbox event for ProductUpdated")
	}

	// If product transitioned to PUBLISHED, publish ProductPublished event
	if p.Status == model.StatusPublished && oldStatus != model.StatusPublished {
		pubEvt, err := productEvents.BuildProductPublishedOutboxEvent(p, p.UpdatedAt)
		if err != nil {
			return appErrors.Internal(err, "failed to build ProductPublished outbox event")
		}
		if err := r.outboxStore.Insert(ctx, tx, pubEvt); err != nil {
			return appErrors.Internal(err, "failed to record outbox event for ProductPublished")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return appErrors.Internal(err, "failed to commit transaction")
	}

	return nil
}

func (r *pgProductRepository) DeleteProduct(ctx context.Context, id string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return appErrors.Internal(err, "failed to start database transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now()
	// Soft delete setting status = 'DELETED' and deleted_at = NOW()
	query := `
		UPDATE products
		SET status = 'DELETED', deleted_at = $2, updated_at = $2
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING store_id
	`
	var storeID string
	err = tx.QueryRow(ctx, query, id, now).Scan(&storeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return appErrors.NotFound("product not found")
		}
		return appErrors.Internal(err, "failed to delete product")
	}

	// Also soft delete variants setting status = 'DELETED' and deleted_at = NOW()
	_, _ = tx.Exec(ctx, "UPDATE product_variants SET status = 'DELETED', deleted_at = $2, updated_at = $2 WHERE product_id = $1 AND deleted_at IS NULL", id, now)

	// Outbox event for ProductDeleted
	delEvt, err := productEvents.BuildProductDeletedOutboxEvent(id, storeID, now)
	if err != nil {
		return appErrors.Internal(err, "failed to build ProductDeleted outbox event")
	}
	if err := r.outboxStore.Insert(ctx, tx, delEvt); err != nil {
		return appErrors.Internal(err, "failed to record outbox event for ProductDeleted")
	}

	if err := tx.Commit(ctx); err != nil {
		return appErrors.Internal(err, "failed to commit transaction")
	}

	return nil
}

func (r *pgProductRepository) DeleteProductVariant(ctx context.Context, productID, variantID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return appErrors.Internal(err, "failed to start database transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now()
	query := `
		UPDATE product_variants
		SET status = 'DELETED', deleted_at = $3, updated_at = $3
		WHERE id = $1 AND product_id = $2 AND deleted_at IS NULL
		RETURNING sku
	`
	var sku string
	err = tx.QueryRow(ctx, query, variantID, productID, now).Scan(&sku)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return appErrors.NotFound("product variant not found for this product")
		}
		return appErrors.Internal(err, "failed to delete product variant")
	}

	eventPayload, _ := json.Marshal(map[string]interface{}{
		"variant_id": variantID,
		"product_id": productID,
		"sku":        sku,
		"status":     "discontinued",
		"deleted_at": now.Format(time.RFC3339),
	})
	outboxQuery := `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, status)
		VALUES ($1, $2, $3, $4, 'PENDING')
	`
	_, _ = tx.Exec(ctx, outboxQuery, "PRODUCT_VARIANT", variantID, "product_variant.deleted", eventPayload)

	if err := tx.Commit(ctx); err != nil {
		return appErrors.Internal(err, "failed to commit transaction")
	}

	return nil
}
