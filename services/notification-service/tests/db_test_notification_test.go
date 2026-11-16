package tests_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/push"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
)

func TestSingleNotificationWithDBRecordAndLocalSystemNotification(t *testing.T) {
	_ = godotenv.Load("../.env")
	_ = godotenv.Load(".env")

	dbURL := strings.TrimSpace(os.Getenv("NOTIFICATIONS_SERVICE_DATABASE_URL"))
	if dbURL == "" {
		dbURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dbURL == "" {
		t.Skip("NOTIFICATIONS_SERVICE_DATABASE_URL or DATABASE_URL not set")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("Skipping live DB test: failed to initialize pgxpool: %v", err)
		return
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Skipping live DB test: database unreachable: %v", err)
		return
	}
	t.Log("Connected to Notification PostgreSQL database successfully")

	logger := slog.Default()
	repo := repository.NewNotificationRepository(pool)
	tmplSvc := service.NewTemplateService(repo)

	localNotifier := push.NewLocalSystemNotifier(logger, true)
	notifSvc := service.NewNotificationService(repo, repo, repo, repo, tmplSvc, nil, logger, localNotifier)

	testUserID := uuid.NewString()
	testContent := fmt.Sprintf("Your order #GC-%s has been confirmed and is being prepared!", uuid.NewString()[:6])
	testSubject := "GoCart: Order Confirmed"

	t.Logf("Creating and persisting ONE notification for UserID: %s", testUserID)
	t.Logf("Notification Content: %s", testContent)

	res, err := notifSvc.SendNotification(ctx, dto.SendNotificationInput{
		UserID:        testUserID,
		Channel:       model.ChannelPush,
		Subject:       testSubject,
		Content:       testContent,
		EventType:     "OrderConfirmed",
		CorrelationID: "test-corr-" + uuid.NewString()[:8],
	})
	if err != nil {
		t.Fatalf("SendNotification failed: %v", err)
	}
	if res == nil || res.Notification == nil {
		t.Fatal("expected valid notification result, got nil")
	}

	notifID := res.Notification.ID
	t.Logf("Notification created with ID: %s", notifID)

	dbNotif, err := repo.GetNotificationByID(ctx, notifID)
	if err != nil {
		t.Fatalf("Failed to query created notification from DB: %v", err)
	}
	if dbNotif == nil {
		t.Fatal("Notification not found in DB")
	}

	t.Log("=== VERIFIED PERSISTED DB RECORD ===")
	t.Logf("  ID:             %s", dbNotif.ID)
	t.Logf("  User ID:        %s", dbNotif.UserID)
	t.Logf("  Channel:        %s", dbNotif.ChannelCode)
	t.Logf("  Event Type:     %s", dbNotif.EventType)
	t.Logf("  Title:          %s", dbNotif.Title)
	t.Logf("  Content (Only): %s", dbNotif.Content)
	t.Logf("  Status:         %s", dbNotif.Status)
	t.Logf("  Created At:     %s", dbNotif.CreatedAt.Format(time.RFC3339))
	t.Log("=====================================")

	if dbNotif.Content != testContent {
		t.Fatalf("DB content mismatch: expected %q, got %q", testContent, dbNotif.Content)
	}

	time.Sleep(5 * time.Second)
	t.Log("Local system notification dispatch complete.")
}
