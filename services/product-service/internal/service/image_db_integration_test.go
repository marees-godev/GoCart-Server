package service_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/service"
)

const pixelBase64 = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func TestImageDBIntegration_ProductService(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dbURL := os.Getenv("PRODUCT_SERVICE_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("Skipping Products DB integration test: PRODUCT_SERVICE_DATABASE_URL or DATABASE_URL not set")
		return
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("Skipping Products DB integration test: failed to initialize pool: %v", err)
		return
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Skipping Products DB integration test: database not reachable: %v", err)
		return
	}

	repo := repository.NewProductRepository(pool)
	validStoreID := uuid.New().String()
	validCatID := uuid.New().String()

	t.Run("PASS_CASE: Successful upload stores Supabase Storage URL in products & product_images tables", func(t *testing.T) {
		uploader := &mockProductUploader{failUpload: false}
		svc := service.NewProductService(repo, nil, nil, uploader)

		sku := "DB-PROD-PASS-" + uuid.New().String()[:8]
		prod, err := svc.CreateProduct(ctx, dto.CreateProductRequest{
			StoreID:    validStoreID,
			CategoryID: validCatID,
			SKU:        sku,
			Name:       "DB Verified Product - Pass Case",
			Price:      149.99,
			MRP:        199.99,
			ImageURL:   pixelBase64,
			Images:     []string{pixelBase64},
		})
		if err != nil {
			t.Fatalf("CreateProduct failed: %v", err)
		}
		defer func() {
			_, _ = pool.Exec(ctx, "DELETE FROM product_images WHERE product_id = $1", prod.ID)
			_, _ = pool.Exec(ctx, "DELETE FROM products WHERE id = $1", prod.ID)
		}()

		// DIRECT DB QUERY: products table
		var dbImageURL string
		err = pool.QueryRow(ctx, "SELECT image_url FROM products WHERE id = $1", prod.ID).Scan(&dbImageURL)
		if err != nil {
			t.Fatalf("DB query for products table failed: %v", err)
		}

		// DIRECT DB QUERY: product_images table
		var galleryURL string
		err = pool.QueryRow(ctx, "SELECT url FROM product_images WHERE product_id = $1 LIMIT 1", prod.ID).Scan(&galleryURL)
		if err != nil {
			t.Fatalf("DB query for product_images table failed: %v", err)
		}

		t.Logf("[DB_CHECK] Table 'products', ID='%s', image_url='%s'", prod.ID, dbImageURL)
		t.Logf("[DB_CHECK] Table 'product_images', product_id='%s', url='%s'", prod.ID, galleryURL)

		if strings.HasPrefix(dbImageURL, "data:") {
			t.Errorf("DB cross-check failed: products.image_url must NOT be base64, got: %s", dbImageURL)
		}
		if !strings.HasPrefix(dbImageURL, "https://storage.supabase.co/") {
			t.Errorf("DB cross-check failed: products.image_url must have Supabase storage URL prefix, got: %s", dbImageURL)
		}
		if strings.HasPrefix(galleryURL, "data:") {
			t.Errorf("DB cross-check failed: product_images.url must NOT be base64, got: %s", galleryURL)
		}
	})

	t.Run("FAIL_CASE_1: Storage unavailable (nil uploader) returns error and prevents Base64 record in DB", func(t *testing.T) {
		svc := service.NewProductService(repo, nil, nil, nil) // uploader = nil

		sku := "DB-PROD-FAIL1-" + uuid.New().String()[:8]
		prod, err := svc.CreateProduct(ctx, dto.CreateProductRequest{
			StoreID:    validStoreID,
			CategoryID: validCatID,
			SKU:        sku,
			Name:       "DB Verified Product - Nil Uploader",
			Price:      149.99,
			MRP:        199.99,
			ImageURL:   pixelBase64,
		})
		if err == nil {
			t.Fatal("expected error when storage uploader is unavailable, got nil")
		}
		if !strings.Contains(err.Error(), "storage service is not configured or unavailable") {
			t.Fatalf("unexpected error message: %v", err)
		}

		// DIRECT DB QUERY: verify no record was written
		var count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM products WHERE sku = $1", sku).Scan(&count)
		t.Logf("[DB_CHECK] Table 'products', SKU='%s', count=%d (0 expected)", sku, count)
		if count != 0 {
			t.Errorf("DB cross-check failed: product record should not exist in DB after upload error, found %d", count)
		}
		if prod != nil {
			_, _ = pool.Exec(ctx, "DELETE FROM products WHERE id = $1", prod.ID)
		}
	})

	t.Run("FAIL_CASE_2: Storage upload failure returns error and prevents Base64 record in DB", func(t *testing.T) {
		uploader := &mockProductUploader{failUpload: true}
		svc := service.NewProductService(repo, nil, nil, uploader)

		sku := "DB-PROD-FAIL2-" + uuid.New().String()[:8]
		prod, err := svc.CreateProduct(ctx, dto.CreateProductRequest{
			StoreID:    validStoreID,
			CategoryID: validCatID,
			SKU:        sku,
			Name:       "DB Verified Product - Upload Failure",
			Price:      149.99,
			MRP:        199.99,
			ImageURL:   pixelBase64,
		})
		if err == nil {
			t.Fatal("expected error when storage upload fails, got nil")
		}
		if !strings.Contains(err.Error(), "failed to upload image to storage") {
			t.Fatalf("unexpected error message: %v", err)
		}

		// DIRECT DB QUERY: verify no record was written
		var count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM products WHERE sku = $1", sku).Scan(&count)
		t.Logf("[DB_CHECK] Table 'products', SKU='%s', count=%d (0 expected)", sku, count)
		if count != 0 {
			t.Errorf("DB cross-check failed: product record should not exist in DB after upload failure, found %d", count)
		}
		if prod != nil {
			_, _ = pool.Exec(ctx, "DELETE FROM products WHERE id = $1", prod.ID)
		}
	})
}
