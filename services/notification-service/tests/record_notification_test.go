package tests_test

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	notificationpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/notification"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// TestRecord_OrderAndDeliveryNotificationsPersistedAndRetrieved tests that when an order or delivery
// is created or cancelled, notification records are reliably persisted in the notification DB and retrievable.
func TestRecord_OrderAndDeliveryNotificationsPersistedAndRetrieved(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	logger := slog.Default()
	memRepo := newDemoMemRepo()
	tmplSvc := service.NewTemplateService(memRepo)
	prefSvc := service.NewPreferenceService(memRepo, memRepo)

	vapidCfg := config.VAPIDConfig{
		PublicKey:  "BIxjxgoRM1_80EZW7vJe8b-eMmr3SdpFiUGEVKoacI_W_XWjyW_JzdUva5Mql5eOR3qZAXhTvA_KTg2kG59J8GY",
		PrivateKey: "rBItcVEjmtp3tTdWz8BbYEUsr09pmJ4fdcIHnv0rDJY",
		Subject:    "mailto:notifications@gocart.local",
	}

	notifSvc := service.NewNotificationService(memRepo, memRepo, memRepo, memRepo, tmplSvc, nil, logger)
	subSvc := service.NewPushSubscriptionService(memRepo, vapidCfg, logger)
	grpcHandler := handler.NewNotificationGRPCHandler(notifSvc, subSvc)

	// gRPC in-memory listener
	lis := bufconn.Listen(1024 * 1024)
	defer lis.Close()

	srv := grpc.NewServer(
		grpc.UnaryInterceptor(grpcclient.UnaryServerInterceptor()),
	)
	notificationpb.RegisterNotificationServiceServer(srv, grpcHandler)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed dialing bufnet: %v", err)
	}
	defer conn.Close()
	client := notificationpb.NewNotificationServiceClient(conn)

	customerID := uuid.NewString()
	merchantID := uuid.NewString()
	orderID := "ord-" + uuid.NewString()[:8]
	deliveryID := "del-" + uuid.NewString()[:8]

	customerCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-user-id", customerID,
		"x-user-role", "CUSTOMER",
	))
	merchantCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-user-id", merchantID,
		"x-user-role", "MERCHANT",
	))

	// ==========================================
	// Sample 1: Customer creates Order -> OrderCreated
	// ==========================================
	t.Log("[SAMPLE 1] Creating Order -> Triggering OrderCreated notification")
	orderResp, err := client.SendNotification(customerCtx, &notificationpb.SendNotificationRequest{
		UserId:    customerID,
		Channel:   "PUSH",
		EventType: "OrderCreated",
		Recipient: customerID,
		Subject:   fmt.Sprintf("Order Placed - #%s", orderID),
		Content:   fmt.Sprintf("Your order #%s (₹1,499.00 INR) has been placed.", orderID),
		TemplateData: map[string]string{
			"order_id":     orderID,
			"user_id":      customerID,
			"total_amount": "1499.00",
			"currency":     "INR",
			"event_id":     "order-created-" + orderID,
		},
		CorrelationId: orderID,
	})
	if err != nil {
		t.Fatalf("SendNotification(OrderCreated) failed: %v", err)
	}
	if !orderResp.GetSuccess() {
		t.Fatalf("expected OrderCreated success, got %+v", orderResp)
	}

	// ==========================================
	// Sample 2: Merchant creates Delivery -> DeliveryCreated
	// ==========================================
	t.Log("[SAMPLE 2] Creating Delivery -> Merchant triggers DeliveryCreated notification for Customer")
	delResp, err := client.SendNotification(merchantCtx, &notificationpb.SendNotificationRequest{
		UserId:    customerID,
		Channel:   "PUSH",
		EventType: "DeliveryCreated",
		Recipient: customerID,
		Subject:   fmt.Sprintf("Delivery Created - Order #%s", orderID),
		Content:   fmt.Sprintf("Delivery for order #%s has been created via BlueDart. Tracking: BD-998877", orderID),
		TemplateData: map[string]string{
			"order_id":         orderID,
			"delivery_id":      deliveryID,
			"user_id":          customerID,
			"delivery_partner": "BlueDart",
			"tracking_number":  "BD-998877",
			"event_id":         "delivery-created-" + deliveryID,
		},
		CorrelationId: orderID,
	})
	if err != nil {
		t.Fatalf("SendNotification(DeliveryCreated by Merchant) failed: %v", err)
	}
	if !delResp.GetSuccess() {
		t.Fatalf("expected DeliveryCreated success, got %+v", delResp)
	}

	// ==========================================
	// Sample 3: Customer cancels Order -> OrderCancelled
	// ==========================================
	t.Log("[SAMPLE 3] Cancelling Order -> Triggering OrderCancelled notification")
	cancelOrderResp, err := client.SendNotification(customerCtx, &notificationpb.SendNotificationRequest{
		UserId:    customerID,
		Channel:   "PUSH",
		EventType: "OrderCancelled",
		Recipient: customerID,
		Subject:   fmt.Sprintf("Order Cancelled - #%s", orderID),
		Content:   fmt.Sprintf("Your order #%s has been cancelled. Reason: Customer requested cancellation.", orderID),
		TemplateData: map[string]string{
			"order_id": orderID,
			"user_id":  customerID,
			"reason":   "Customer requested cancellation",
			"event_id": "order-cancelled-" + orderID,
		},
		CorrelationId: orderID,
	})
	if err != nil {
		t.Fatalf("SendNotification(OrderCancelled) failed: %v", err)
	}
	if !cancelOrderResp.GetSuccess() {
		t.Fatalf("expected OrderCancelled success, got %+v", cancelOrderResp)
	}

	// ==========================================
	// Sample 4: Cancelling Delivery -> DeliveryStatusUpdated with CANCELLED
	// ==========================================
	t.Log("[SAMPLE 4] Cancelling Delivery -> Triggering DeliveryStatusUpdated (CANCELLED)")
	cancelDelResp, err := client.SendNotification(customerCtx, &notificationpb.SendNotificationRequest{
		UserId:    customerID,
		Channel:   "PUSH",
		EventType: "DeliveryStatusUpdated",
		Recipient: customerID,
		Subject:   fmt.Sprintf("Delivery Update - Order #%s", orderID),
		Content:   fmt.Sprintf("Delivery status for order #%s (Tracking: BD-998877) updated to CANCELLED.", orderID),
		TemplateData: map[string]string{
			"order_id":         orderID,
			"delivery_id":      deliveryID,
			"user_id":          customerID,
			"delivery_partner": "BlueDart",
			"tracking_number":  "BD-998877",
			"new_status":       "CANCELLED",
			"event_id":         "delivery-status-" + deliveryID + "-cancelled",
		},
		CorrelationId: orderID,
	})
	if err != nil {
		t.Fatalf("SendNotification(DeliveryStatusUpdated CANCELLED) failed: %v", err)
	}
	if !cancelDelResp.GetSuccess() {
		t.Fatalf("expected DeliveryStatusUpdated success, got %+v", cancelDelResp)
	}

	// ==========================================
	// Sample 5: Kafka Consumer Domain Event Pipeline for DeliveryStatusUpdated
	// ==========================================
	t.Log("[SAMPLE 5] Kafka Consumer receiving DeliveryStatusUpdatedEvent (IN_TRANSIT)")
	pipeline := service.NewPushNotificationPipelineService(tmplSvc, prefSvc, memRepo, memRepo, notifSvc, nil, logger)
	eventConsumer := consumer.NewNotificationEventConsumer(pipeline, logger)

	kafkaUpdateEnv, err := events.NewEventEnvelopeWithAggregate(
		events.EventTypeDeliveryStatusUpdated,
		"delivery-service",
		deliveryID,
		events.DeliveryStatusUpdatedEvent{
			DeliveryID:     deliveryID,
			OrderID:        orderID,
			UserID:         customerID, // Populated from delivery.ShippingAddress.UserID!
			PreviousStatus: "CREATED",
			NewStatus:      "IN_TRANSIT",
			TrackingNumber: "BD-998877",
			UpdatedAt:      time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("failed constructing kafka DeliveryStatusUpdated envelope: %v", err)
	}
	kafkaUpdateEnv.EventID = "delivery-status-" + deliveryID + "-intransit"

	if err := eventConsumer.HandleEvent(ctx, kafkaUpdateEnv); err != nil {
		t.Fatalf("HandleEvent(DeliveryStatusUpdatedEvent) failed: %v", err)
	}

	// ==========================================
	// Verification: Check all 5 records are stored in Notification DB
	// ==========================================
	t.Log("[VERIFICATION] Fetching notifications from DB for customer to ensure ALL 5 events were stored")
	listResp, err := client.GetNotifications(customerCtx, &notificationpb.GetNotificationsRequest{
		UserId: customerID,
		Limit:  20,
	})
	if err != nil {
		t.Fatalf("GetNotifications failed: %v", err)
	}

	if listResp.GetTotal() != 5 {
		t.Fatalf("expected 5 notification records in DB, got %d", listResp.GetTotal())
	}
	if listResp.GetUnreadCount() != 5 {
		t.Fatalf("expected 5 unread notifications, got %d", listResp.GetUnreadCount())
	}

	eventCounts := make(map[string]int)
	var cancelDelNotifID string
	for _, n := range listResp.GetNotifications() {
		if n.GetUserId() != customerID {
			t.Errorf("expected user_id %s, got %s", customerID, n.GetUserId())
		}
		if n.GetIsRead() {
			t.Errorf("notification should initially be unread, got true")
		}
		eventCounts[n.GetEventType()]++
		if n.GetEventType() == "DeliveryStatusUpdated" && strings.Contains(n.GetContent(), "CANCELLED") {
			cancelDelNotifID = n.GetId()
		}
	}

	// Verify counts by event type
	if eventCounts["OrderCreated"] != 1 {
		t.Errorf("expected 1 OrderCreated in DB, got %d", eventCounts["OrderCreated"])
	}
	if eventCounts["DeliveryCreated"] != 1 {
		t.Errorf("expected 1 DeliveryCreated in DB, got %d", eventCounts["DeliveryCreated"])
	}
	if eventCounts["OrderCancelled"] != 1 {
		t.Errorf("expected 1 OrderCancelled in DB, got %d", eventCounts["OrderCancelled"])
	}
	if eventCounts["DeliveryStatusUpdated"] != 2 { // 1 from direct resolver, 1 from Kafka pipeline
		t.Errorf("expected 2 DeliveryStatusUpdated in DB, got %d", eventCounts["DeliveryStatusUpdated"])
	}

	// ==========================================
	// Mark single notification read and check DB persistence
	// ==========================================
	t.Log("[VERIFICATION] Testing MarkNotificationAsRead in DB")
	readTrue := true
	markResp, err := client.MarkNotificationAsRead(customerCtx, &notificationpb.MarkNotificationAsReadRequest{
		NotificationId: cancelDelNotifID,
		UserId:         customerID,
		IsRead:         &readTrue,
	})
	if err != nil || !markResp.GetSuccess() || !markResp.GetNotification().GetIsRead() {
		t.Fatalf("MarkNotificationAsRead failed: %v", err)
	}

	unreadResp, err := client.GetUnreadCount(customerCtx, &notificationpb.GetUnreadCountRequest{UserId: customerID})
	if err != nil || unreadResp.GetUnreadCount() != 4 {
		t.Fatalf("expected 4 unread notifications after reading 1, got %d", unreadResp.GetUnreadCount())
	}

	// Mark all read
	t.Log("[VERIFICATION] Testing MarkAllNotificationsAsRead in DB")
	markAllResp, err := client.MarkAllNotificationsAsRead(customerCtx, &notificationpb.MarkAllNotificationsAsReadRequest{UserId: customerID})
	if err != nil || !markAllResp.GetSuccess() || markAllResp.GetUpdatedCount() != 4 {
		t.Fatalf("MarkAllNotificationsAsRead failed: updated=%d err=%v", markAllResp.GetUpdatedCount(), err)
	}

	finalUnread, err := client.GetUnreadCount(customerCtx, &notificationpb.GetUnreadCountRequest{UserId: customerID})
	if err != nil || finalUnread.GetUnreadCount() != 0 {
		t.Fatalf("expected 0 unread notifications, got %d", finalUnread.GetUnreadCount())
	}

	// Verify authorization security
	t.Log("[VERIFICATION] Testing unauthorized user access is denied")
	otherCustomerCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-user-id", uuid.NewString(),
		"x-user-role", "CUSTOMER",
	))
	_, err = client.GetNotifications(otherCustomerCtx, &notificationpb.GetNotificationsRequest{
		UserId: customerID,
	})
	if err == nil || status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied when another customer attempts access, got %v", err)
	}

	t.Log("[SUCCESS] Creating and cancelling order and delivery notifications are 100% verified and stored in Notification DB!")
}
