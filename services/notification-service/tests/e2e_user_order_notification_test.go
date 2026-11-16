package tests

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
)

func TestE2E_OrderCreatedAndCancelled_GeneratesNotification(t *testing.T) {
	dbURL := strings.TrimSpace(os.Getenv("NOTIFICATIONS_SERVICE_DATABASE_URL"))
	if dbURL == "" {
		dbURL = strings.TrimSpace(os.Getenv("NOTIFICATION_TEST_DATABASE_URL"))
	}
	if dbURL == "" {
		t.Skip("skipping live db test: NOTIFICATIONS_SERVICE_DATABASE_URL / NOTIFICATION_TEST_DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("skipping live db test: could not connect to db: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skipping live db test: ping failed: %v", err)
	}

	logger := slog.Default()
	repo := repository.NewNotificationRepository(pool)
	tmplSvc := service.NewTemplateService(repo)
	prefSvc := service.NewPreferenceService(repo, repo)

	notifSvc := service.NewNotificationService(repo, repo, repo, repo, tmplSvc, nil, logger)
	pipeline := service.NewPushNotificationPipelineService(tmplSvc, prefSvc, repo, repo, notifSvc, nil, logger)

	orderID := "7b63aa20-03ce-48c1-8df5-4cee57c4b407"
	userID := "01a1820f-30b0-72ef-911c-de6eb5cbdfbb"
	now := time.Now().UTC()

	// 1. Process OrderCreated
	createdEnv, err := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderCreated, "order-service", orderID, events.OrderCreatedEvent{
		OrderID:     orderID,
		UserID:      userID,
		TotalAmount: 790.98,
		Currency:    "INR",
		Status:      "CREATED",
	})
	if err != nil {
		t.Fatalf("failed to create OrderCreated envelope: %v", err)
	}
	createdEnv.EventID = "e2e-order-created-" + now.Format("20060102150405.000")

	jobCreated, err := pipeline.ProcessEvent(ctx, createdEnv)
	if err != nil {
		t.Fatalf("ProcessEvent(OrderCreated) failed: %v", err)
	}
	if jobCreated == nil {
		t.Fatal("expected non-nil job for OrderCreated")
	}

	// 2. Process OrderCancelled
	cancelledEnv, err := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderCancelled, "order-service", orderID, events.OrderCancelledEvent{
		OrderID:     orderID,
		UserID:      userID,
		Reason:      "order wrong product",
		CancelledAt: now,
	})
	if err != nil {
		t.Fatalf("failed to create OrderCancelled envelope: %v", err)
	}
	cancelledEnv.EventID = "e2e-order-cancelled-" + now.Format("20060102150405.000")

	jobCancelled, err := pipeline.ProcessEvent(ctx, cancelledEnv)
	if err != nil {
		t.Fatalf("ProcessEvent(OrderCancelled) failed: %v", err)
	}
	if jobCancelled == nil {
		t.Fatal("expected non-nil job for OrderCancelled")
	}

	// 3. Process OrderConfirmed
	confirmedEnv, err := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderConfirmed, "order-service", orderID, events.OrderConfirmedEvent{
		OrderID:       orderID,
		UserID:        userID,
		PaymentStatus: "PAID",
		ConfirmedAt:   now,
	})
	if err != nil {
		t.Fatalf("failed to create OrderConfirmed envelope: %v", err)
	}
	confirmedEnv.EventID = "e2e-order-confirmed-" + now.Format("20060102150405.000")

	jobConfirmed, err := pipeline.ProcessEvent(ctx, confirmedEnv)
	if err != nil {
		t.Fatalf("ProcessEvent(OrderConfirmed) failed: %v", err)
	}
	if jobConfirmed == nil {
		t.Fatal("expected non-nil job for OrderConfirmed")
	}

	// 4. Process OrderShipped
	shippedEnv, err := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderShipped, "order-service", orderID, events.OrderShippedEvent{
		OrderID:         orderID,
		UserID:          userID,
		DeliveryPartner: "BlueDart",
		TrackingNumber:  "BD-778899",
		ShippedAt:       now,
	})
	if err != nil {
		t.Fatalf("failed to create OrderShipped envelope: %v", err)
	}
	shippedEnv.EventID = "e2e-order-shipped-" + now.Format("20060102150405.000")

	jobShipped, err := pipeline.ProcessEvent(ctx, shippedEnv)
	if err != nil {
		t.Fatalf("ProcessEvent(OrderShipped) failed: %v", err)
	}
	if jobShipped == nil {
		t.Fatal("expected non-nil job for OrderShipped")
	}

	// 5. Process OrderDelivered
	deliveredEnv, err := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderDelivered, "order-service", orderID, events.OrderDeliveredEvent{
		OrderID:     orderID,
		UserID:      userID,
		DeliveredAt: now,
	})
	if err != nil {
		t.Fatalf("failed to create OrderDelivered envelope: %v", err)
	}
	deliveredEnv.EventID = "e2e-order-delivered-" + now.Format("20060102150405.000")

	jobDelivered, err := pipeline.ProcessEvent(ctx, deliveredEnv)
	if err != nil {
		t.Fatalf("ProcessEvent(OrderDelivered) failed: %v", err)
	}
	if jobDelivered == nil {
		t.Fatal("expected non-nil job for OrderDelivered")
	}

	// 6. Verify notifications table has records for all order lifecycle events
	for _, eventType := range []string{
		events.EventTypeOrderCreated,
		events.EventTypeOrderCancelled,
		events.EventTypeOrderConfirmed,
		events.EventTypeOrderShipped,
		events.EventTypeOrderDelivered,
	} {
		var count int
		err = pool.QueryRow(ctx, "SELECT count(*) FROM notifications WHERE user_id = $1 AND event_type = $2", userID, eventType).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query %s notifications: %v", eventType, err)
		}
		if count == 0 {
			t.Errorf("expected at least 1 %s notification in DB for user %s, got 0", eventType, userID)
		}
	}

	t.Logf("Successfully verified notifications for all order lifecycle events for user %s", userID)
}

func TestE2E_RecentOrderAndDelivery_GeneratesNotification(t *testing.T) {
	dbURL := strings.TrimSpace(os.Getenv("NOTIFICATIONS_SERVICE_DATABASE_URL"))
	if dbURL == "" {
		dbURL = strings.TrimSpace(os.Getenv("NOTIFICATION_TEST_DATABASE_URL"))
	}
	if dbURL == "" {
		t.Skip("skipping live db test: NOTIFICATIONS_SERVICE_DATABASE_URL / NOTIFICATION_TEST_DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("skipping live db test: could not connect to db: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skipping live db test: ping failed: %v", err)
	}

	logger := slog.Default()
	repo := repository.NewNotificationRepository(pool)
	tmplSvc := service.NewTemplateService(repo)
	prefSvc := service.NewPreferenceService(repo, repo)
	notifSvc := service.NewNotificationService(repo, repo, repo, repo, tmplSvc, nil, logger)
	pipeline := service.NewPushNotificationPipelineService(tmplSvc, prefSvc, repo, repo, notifSvc, nil, logger)

	orderID := "1648f99e-360e-4d4b-8ac9-364f0e77a449"
	userID := "01a1820f-30b0-72ef-911c-de6eb5cbdfbb"
	deliveryID := "356fb314-1ecf-4010-a480-efa360aaf2aa"
	partner := "Lakshimi trader"
	tracking := "GC-LAK-5074C9BFB88C"

	// 1. Process OrderCreated for order 1648f99e-360e-4d4b-8ac9-364f0e77a449
	createdEnv, err := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderCreated, "order-service", orderID, events.OrderCreatedEvent{
		OrderID:     orderID,
		UserID:      userID,
		TotalAmount: 170.00,
		Currency:    "INR",
		Status:      "CREATED",
	})
	if err != nil {
		t.Fatalf("failed to create OrderCreated envelope: %v", err)
	}
	createdEnv.EventID = "order-created-" + orderID

	jobCreated, err := pipeline.ProcessEvent(ctx, createdEnv)
	if err != nil {
		t.Fatalf("ProcessEvent(OrderCreated) failed: %v", err)
	}
	if jobCreated == nil {
		t.Fatal("expected non-nil job for OrderCreated")
	}

	// 2. Process DeliveryCreated for delivery 356fb314-1ecf-4010-a480-efa360aaf2aa
	deliveryEnv, err := events.NewEventEnvelopeWithAggregate(events.EventTypeDeliveryCreated, "delivery-service", deliveryID, events.DeliveryCreatedEvent{
		DeliveryID:      deliveryID,
		OrderID:         orderID,
		UserID:          userID,
		DeliveryPartner: partner,
		TrackingNumber:  tracking,
		Status:          "CREATED",
		CreatedAt:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to create DeliveryCreated envelope: %v", err)
	}
	deliveryEnv.EventID = "delivery-created-" + deliveryID

	jobDelivery, err := pipeline.ProcessEvent(ctx, deliveryEnv)
	if err != nil {
		t.Fatalf("ProcessEvent(DeliveryCreated) failed: %v", err)
	}
	if jobDelivery == nil {
		t.Fatal("expected non-nil job for DeliveryCreated")
	}

	// 3. Verify both notifications are in DB
	var notifCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM notifications WHERE user_id = $1 AND event_id IN ($2, $3)", userID, "order-created-"+orderID, "delivery-created-"+deliveryID).Scan(&notifCount)
	if err != nil {
		t.Fatalf("failed to count notifications: %v", err)
	}
	if notifCount < 2 {
		t.Errorf("expected 2 notifications for order and delivery, got %d", notifCount)
	}
	t.Logf("Successfully verified notifications for order %s and delivery %s in DB: count=%d", orderID, deliveryID, notifCount)
}
