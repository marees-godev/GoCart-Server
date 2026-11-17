package service_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/service"
)

const pixelAvatarBase64 = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func TestImageDBIntegration_UserService(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dbURL := os.Getenv("USER_SERVICE_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("Skipping Users DB integration test: USER_SERVICE_DATABASE_URL or DATABASE_URL not set")
		return
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("Skipping Users DB integration test: failed to initialize pool: %v", err)
		return
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Skipping Users DB integration test: database not reachable: %v", err)
		return
	}

	repo := repository.NewUserRepository(pool)
	userID := uuid.New().String()
	userEmail := "dbuser_" + uuid.New().String()[:8] + "@gocart.local"

	initialAvatar := "https://example.com/initial-avatar.png"
	seedUser := &model.User{
		ID:        userID,
		Email:     userEmail,
		FirstName: "DBTest",
		LastName:  "AvatarUser",
		Status:    "active",
		AvatarURL: &initialAvatar,
	}
	err = repo.CreateUser(ctx, seedUser)
	if err != nil {
		t.Fatalf("Failed to create seed user in DB: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	}()

	t.Run("PASS_CASE: Successful upload stores Supabase Storage URL in users table", func(t *testing.T) {
		imgProc := &mockCustomImageProcessor{
			processFunc: func(ctx context.Context, uID string, input service.AvatarInput) (string, error) {
				return fmt.Sprintf("https://storage.supabase.co/storage/v1/object/public/avatars/%s/avatar.png", uID), nil
			},
		}
		svc := service.NewUserService(repo, imgProc)

		b64 := pixelAvatarBase64
		updated, err := svc.UpdateUser(ctx, userID, userID, dto.UpdateUserRequest{
			AvatarURL: &b64,
		})
		if err != nil {
			t.Fatalf("UpdateUser failed: %v", err)
		}

		// DIRECT DB QUERY: users table
		var dbAvatarURL string
		err = pool.QueryRow(ctx, "SELECT avatar_url FROM users WHERE id = $1", userID).Scan(&dbAvatarURL)
		if err != nil {
			t.Fatalf("DB query for users table failed: %v", err)
		}

		t.Logf("[DB_CHECK] Table 'users', ID='%s', avatar_url='%s'", updated.ID, dbAvatarURL)

		if strings.HasPrefix(dbAvatarURL, "data:") {
			t.Errorf("DB cross-check failed: users.avatar_url must NOT be base64, got: %s", dbAvatarURL)
		}
		if !strings.HasPrefix(dbAvatarURL, "https://storage.supabase.co/") {
			t.Errorf("DB cross-check failed: users.avatar_url must have Supabase Storage URL prefix, got: %s", dbAvatarURL)
		}
	})

	t.Run("FAIL_CASE_1: Storage unavailable (nil processor) returns error and prevents Base64 record in DB", func(t *testing.T) {
		svc := service.NewUserService(repo, nil) // nil processor

		b64 := pixelAvatarBase64
		_, err := svc.UpdateUser(ctx, userID, userID, dto.UpdateUserRequest{
			AvatarURL: &b64,
		})
		if err == nil {
			t.Fatal("expected error when processor is nil, got nil")
		}
		if !strings.Contains(err.Error(), "storage uploader is not configured") {
			t.Fatalf("unexpected error message: %v", err)
		}

		// DIRECT DB QUERY: verify avatar_url did NOT become base64
		var dbAvatarURL string
		err = pool.QueryRow(ctx, "SELECT avatar_url FROM users WHERE id = $1", userID).Scan(&dbAvatarURL)
		if err != nil {
			t.Fatalf("DB query for users table failed: %v", err)
		}

		t.Logf("[DB_CHECK] Table 'users', ID='%s', avatar_url='%s'", userID, dbAvatarURL)
		if strings.HasPrefix(dbAvatarURL, "data:") {
			t.Errorf("DB cross-check failed: users.avatar_url became Base64, got: %s", dbAvatarURL)
		}
	})

	t.Run("FAIL_CASE_2: Storage upload failure returns error and prevents Base64 record in DB", func(t *testing.T) {
		imgProc := &mockCustomImageProcessor{
			processFunc: func(ctx context.Context, uID string, input service.AvatarInput) (string, error) {
				return "", appErrors.Internal(nil, "supabase storage: upload failed")
			},
		}
		svc := service.NewUserService(repo, imgProc)

		b64 := pixelAvatarBase64
		_, err := svc.UpdateUser(ctx, userID, userID, dto.UpdateUserRequest{
			AvatarURL: &b64,
		})
		if err == nil {
			t.Fatal("expected error when storage upload fails, got nil")
		}
		if !strings.Contains(err.Error(), "supabase storage: upload failed") {
			t.Fatalf("unexpected error message: %v", err)
		}

		// DIRECT DB QUERY: verify avatar_url did NOT become base64
		var dbAvatarURL string
		err = pool.QueryRow(ctx, "SELECT avatar_url FROM users WHERE id = $1", userID).Scan(&dbAvatarURL)
		if err != nil {
			t.Fatalf("DB query for users table failed: %v", err)
		}

		t.Logf("[DB_CHECK] Table 'users', ID='%s', avatar_url='%s'", userID, dbAvatarURL)
		if strings.HasPrefix(dbAvatarURL, "data:") {
			t.Errorf("DB cross-check failed: users.avatar_url became Base64, got: %s", dbAvatarURL)
		}
	})
}
