package service_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/service"
)

const pixelProofBase64 = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func TestImageDBIntegration_ReturnService(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dbURL := os.Getenv("RETURN_SERVICE_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("Skipping Returns DB integration test: RETURN_SERVICE_DATABASE_URL or DATABASE_URL not set")
		return
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("Skipping Returns DB integration test: failed to initialize pool: %v", err)
		return
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Skipping Returns DB integration test: database not reachable: %v", err)
		return
	}

	repo := repository.NewReturnRepository(&database.DB{Pool: pool}, outbox.NewStore(), slog.Default())

	createSeedReturn := func(retID string) {
		now := time.Now().UTC()
		_, _ = pool.Exec(ctx, `INSERT INTO returns (id, order_id, order_item_id, user_id, reason, status, refund_amount, requested_at, created_at, updated_at, type)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			retID, uuid.New().String(), uuid.New().String(), uuid.New().String(),
			"Defective item for DB test", "RECEIVED", 75.0, now, now, now, "RETURN")
	}

	cleanSeedReturn := func(retID string) {
		_, _ = pool.Exec(ctx, "DELETE FROM return_verification_images WHERE verification_id IN (SELECT id FROM return_verifications WHERE return_id = $1)", retID)
		_, _ = pool.Exec(ctx, "DELETE FROM return_verifications WHERE return_id = $1", retID)
		_, _ = pool.Exec(ctx, "DELETE FROM returns WHERE id = $1", retID)
	}

	t.Run("PASS_CASE: Successful upload stores Supabase Storage URL in return_verification_images table", func(t *testing.T) {
		retID := uuid.New().String()
		createSeedReturn(retID)
		defer cleanSeedReturn(retID)

		uploader := &mockReturnStorageUploader{}
		svc := service.NewReturnService(repo, nil, nil, 7, uploader)

		verifiedBy := uuid.New().String()
		ret, v, err := svc.VerifyProduct(ctx, &dto.VerifyProductDTO{
			ReturnID:    retID,
			VerifiedBy:  verifiedBy,
			Condition:   "GOOD",
			IsApproved:  true,
			ProofImages: []string{pixelProofBase64},
		})
		if err != nil {
			t.Fatalf("VerifyProduct failed: %v", err)
		}

		// DIRECT DB QUERY: return_verification_images table
		var dbProofURL string
		err = pool.QueryRow(ctx, "SELECT image_url FROM return_verification_images WHERE verification_id = $1 LIMIT 1", v.ID).Scan(&dbProofURL)
		if err != nil {
			t.Fatalf("DB query for return_verification_images failed: %v", err)
		}

		t.Logf("[DB_CHECK] Table 'return_verification_images', verification_id='%s', image_url='%s'", v.ID, dbProofURL)

		if strings.HasPrefix(dbProofURL, "data:") {
			t.Errorf("DB cross-check failed: return_verification_images.image_url must NOT be base64, got: %s", dbProofURL)
		}
		if !strings.HasPrefix(dbProofURL, "https://storage.supabase.co/") {
			t.Errorf("DB cross-check failed: return_verification_images.image_url must have Supabase Storage URL prefix, got: %s", dbProofURL)
		}
		if ret.Status != model.ReturnStatusVerified {
			t.Errorf("expected return status VERIFIED, got: %s", ret.Status)
		}
	})

	t.Run("FAIL_CASE_1: Storage unavailable (nil uploader) returns error and prevents Base64 record in DB", func(t *testing.T) {
		retID := uuid.New().String()
		createSeedReturn(retID)
		defer cleanSeedReturn(retID)

		svc := service.NewReturnService(repo, nil, nil, 7) // nil uploader

		verifiedBy := uuid.New().String()
		_, _, err := svc.VerifyProduct(ctx, &dto.VerifyProductDTO{
			ReturnID:    retID,
			VerifiedBy:  verifiedBy,
			Condition:   "GOOD",
			IsApproved:  true,
			ProofImages: []string{pixelProofBase64},
		})
		if err == nil {
			t.Fatal("expected error when storage uploader is unavailable, got nil")
		}
		if !strings.Contains(err.Error(), "storage service is unavailable") {
			t.Fatalf("unexpected error message: %v", err)
		}

		// DIRECT DB QUERY: verify status remains RECEIVED and images count is 0
		var status string
		_ = pool.QueryRow(ctx, "SELECT status FROM returns WHERE id = $1", retID).Scan(&status)
		var imgCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM return_verification_images WHERE verification_id IN (SELECT id FROM return_verifications WHERE return_id = $1)", retID).Scan(&imgCount)

		t.Logf("[DB_CHECK] Table 'returns', ID='%s', status='%s', images_count=%d (0 expected)", retID, status, imgCount)

		if status != "RECEIVED" {
			t.Errorf("DB cross-check failed: status should remain RECEIVED, got: %s", status)
		}
		if imgCount != 0 {
			t.Errorf("DB cross-check failed: images should not be written to DB, found %d", imgCount)
		}
	})

	t.Run("FAIL_CASE_2: Storage upload failure returns error and prevents Base64 record in DB", func(t *testing.T) {
		retID := uuid.New().String()
		createSeedReturn(retID)
		defer cleanSeedReturn(retID)

		uploader := &mockReturnStorageUploader{
			uploadBytesFunc: func(ctx context.Context, key string, data []byte, contentType string) (string, error) {
				return "", errors.New("supabase storage: upload failed")
			},
		}
		svc := service.NewReturnService(repo, nil, nil, 7, uploader)

		verifiedBy := uuid.New().String()
		_, _, err := svc.VerifyProduct(ctx, &dto.VerifyProductDTO{
			ReturnID:    retID,
			VerifiedBy:  verifiedBy,
			Condition:   "GOOD",
			IsApproved:  true,
			ProofImages: []string{pixelProofBase64},
		})
		if err == nil {
			t.Fatal("expected error when storage upload fails, got nil")
		}
		if !strings.Contains(err.Error(), "failed to upload image to storage") {
			t.Fatalf("unexpected error message: %v", err)
		}

		// DIRECT DB QUERY: verify status remains RECEIVED and images count is 0
		var status string
		_ = pool.QueryRow(ctx, "SELECT status FROM returns WHERE id = $1", retID).Scan(&status)
		var imgCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM return_verification_images WHERE verification_id IN (SELECT id FROM return_verifications WHERE return_id = $1)", retID).Scan(&imgCount)

		t.Logf("[DB_CHECK] Table 'returns', ID='%s', status='%s', images_count=%d (0 expected)", retID, status, imgCount)

		if status != "RECEIVED" {
			t.Errorf("DB cross-check failed: status should remain RECEIVED, got: %s", status)
		}
		if imgCount != 0 {
			t.Errorf("DB cross-check failed: images should not be written to DB, found %d", imgCount)
		}
	})
}
