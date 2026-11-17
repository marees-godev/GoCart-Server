package service_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/service"
)

const pixelLogoBase64 = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func TestImageDBIntegration_StoreService(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dbURL := os.Getenv("STORE_SERVICE_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("Skipping Stores DB integration test: STORE_SERVICE_DATABASE_URL or DATABASE_URL not set")
		return
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("Skipping Stores DB integration test: failed to initialize pool: %v", err)
		return
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Skipping Stores DB integration test: database not reachable: %v", err)
		return
	}

	repo := repository.NewStoreRepository(pool)
	merchantID := uuid.New().String()
	merchantCtx := auth.WithUser(ctx, &auth.UserContext{
		UserID: merchantID,
		Role:   auth.RoleMerchant,
	})

	t.Run("PASS_CASE: Successful upload stores Supabase Storage URL in stores table", func(t *testing.T) {
		uploader := &mockStoreUploader{failUpload: false}
		svc := service.NewStoreService(repo, uploader)

		storeName := "DB Verified Store Pass " + uuid.New().String()[:6]
		st, err := svc.CreateStore(merchantCtx, merchantID, dto.CreateStoreRequest{
			Name:          storeName,
			BusinessEmail: "pass@storedb.local",
			BusinessPhone: "+1234567890",
			Address:       "123 Store Lane",
			LogoURL:       pixelLogoBase64,
		})
		if err != nil {
			t.Fatalf("CreateStore failed: %v", err)
		}
		defer func() {
			_, _ = pool.Exec(ctx, "DELETE FROM stores WHERE id = $1", st.ID)
		}()

		// DIRECT DB QUERY: stores table
		var dbLogoURL string
		err = pool.QueryRow(ctx, "SELECT logo_url FROM stores WHERE id = $1", st.ID).Scan(&dbLogoURL)
		if err != nil {
			t.Fatalf("DB query for stores table failed: %v", err)
		}

		t.Logf("[DB_CHECK] Table 'stores', ID='%s', logo_url='%s'", st.ID, dbLogoURL)

		if strings.HasPrefix(dbLogoURL, "data:") {
			t.Errorf("DB cross-check failed: stores.logo_url must NOT be base64, got: %s", dbLogoURL)
		}
		if !strings.HasPrefix(dbLogoURL, "https://storage.supabase.co/") {
			t.Errorf("DB cross-check failed: stores.logo_url must have Supabase storage URL prefix, got: %s", dbLogoURL)
		}
	})

	t.Run("FAIL_CASE_1: Storage unavailable (nil uploader) returns error and prevents Base64 record in DB", func(t *testing.T) {
		svc := service.NewStoreService(repo) // no uploader

		storeName := "DB Verified Store Nil " + uuid.New().String()[:6]
		st, err := svc.CreateStore(merchantCtx, merchantID, dto.CreateStoreRequest{
			Name:          storeName,
			BusinessEmail: "nil@storedb.local",
			BusinessPhone: "+1234567890",
			Address:       "123 Store Lane",
			LogoURL:       pixelLogoBase64,
		})
		if err == nil {
			t.Fatal("expected error when storage uploader is unavailable, got nil")
		}
		if !strings.Contains(err.Error(), "storage service is not configured or unavailable") {
			t.Fatalf("unexpected error message: %v", err)
		}

		// DIRECT DB QUERY: verify no record was written
		var count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM stores WHERE name = $1", storeName).Scan(&count)
		t.Logf("[DB_CHECK] Table 'stores', Name='%s', count=%d (0 expected)", storeName, count)
		if count != 0 {
			t.Errorf("DB cross-check failed: store record should not exist in DB after upload error, found %d", count)
		}
		if st != nil {
			_, _ = pool.Exec(ctx, "DELETE FROM stores WHERE id = $1", st.ID)
		}
	})

	t.Run("FAIL_CASE_2: Storage upload failure returns error and prevents Base64 record in DB", func(t *testing.T) {
		uploader := &mockStoreUploader{failUpload: true}
		svc := service.NewStoreService(repo, uploader)

		storeName := "DB Verified Store Fail " + uuid.New().String()[:6]
		st, err := svc.CreateStore(merchantCtx, merchantID, dto.CreateStoreRequest{
			Name:          storeName,
			BusinessEmail: "fail@storedb.local",
			BusinessPhone: "+1234567890",
			Address:       "123 Store Lane",
			LogoURL:       pixelLogoBase64,
		})
		if err == nil {
			t.Fatal("expected error when storage upload fails, got nil")
		}
		if !strings.Contains(err.Error(), "failed to upload image to storage") {
			t.Fatalf("unexpected error message: %v", err)
		}

		// DIRECT DB QUERY: verify no record was written
		var count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM stores WHERE name = $1", storeName).Scan(&count)
		t.Logf("[DB_CHECK] Table 'stores', Name='%s', count=%d (0 expected)", storeName, count)
		if count != 0 {
			t.Errorf("DB cross-check failed: store record should not exist in DB after upload failure, found %d", count)
		}
		if st != nil {
			_, _ = pool.Exec(ctx, "DELETE FROM stores WHERE id = $1", st.ID)
		}
	})
}
