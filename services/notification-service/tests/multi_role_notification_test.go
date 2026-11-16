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
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/push"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// TestMultiRole_UserMerchantDeliveryPartner_Notifications tests sample notifications across
// all 3 roles: USER (Customer), MERCHANT, and DELIVERY_PARTNER.
// It verifies:
// 1. Web Push device subscription per role (User Chrome/Safari, Merchant POS, Delivery Partner Mobile).
// 2. Event triggers (OrderCreated, PaymentSuccessful, DeliveryCreated, DeliveryStatusUpdated, etc.).
// 3. RFC 8291/8292 encrypted Web Push delivery to respective device endpoints.
// 4. Persistence of notification records in the Notification DB for each role.
// 5. Read receipt management (MarkNotificationAsRead, MarkAllNotificationsAsRead, GetUnreadCount).
// 6. Strict RBAC & user isolation between roles.
func TestMultiRole_UserMerchantDeliveryPartner_Notifications(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	logger := slog.Default()

	// 1. Setup VAPID keys and Mock Push Gateway
	pubVAPID, privVAPID, err := push.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys failed: %v", err)
	}
	vapidSubj := "mailto:admin@gocart.local"

	pushGateway := newDemoPushGateway(pubVAPID, vapidSubj)
	webPushClient := push.NewWebPushClient(
		config.VAPIDConfig{PublicKey: pubVAPID, PrivateKey: privVAPID, Subject: vapidSubj},
		config.PushConfig{RequestTimeout: 2 * time.Second, MaxRetries: 3, RetryInterval: 5 * time.Millisecond, TTLSeconds: 3600},
		pushGateway.httpClient(t),
		logger,
	)

	// 2. Setup Core Repositories & Services
	repo := newDemoMemRepo()
	tmplSvc := service.NewTemplateService(repo)
	prefSvc := service.NewPreferenceService(repo, repo)
	subSvc := service.NewPushSubscriptionService(repo, config.VAPIDConfig{
		PublicKey:  pubVAPID,
		PrivateKey: privVAPID,
		Subject:    vapidSubj,
	}, logger)
	notifSvc := service.NewNotificationService(repo, repo, repo, repo, tmplSvc, webPushClient, logger)

	// Order lookup mapping order IDs to User IDs
	customerID := uuid.NewString()
	merchantID := uuid.NewString()
	deliveryPartnerID := uuid.NewString()
	orderID := "ORD-" + uuid.NewString()[:8]
	deliveryID := "DEL-" + uuid.NewString()[:8]

	orderLookup := &demoOrderLookup{orderUsers: map[string]string{
		orderID: customerID,
	}}
	pipeline := service.NewPushNotificationPipelineService(tmplSvc, prefSvc, repo, repo, notifSvc, orderLookup, logger)
	evtConsumer := consumer.NewNotificationEventConsumer(pipeline, logger)

	// 3. Setup gRPC Server with Interceptor
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(grpcclient.UnaryServerInterceptor()),
	)
	notificationpb.RegisterNotificationServiceServer(srv, handler.NewNotificationGRPCHandler(notifSvc, subSvc))
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed dialing bufnet: %v", err)
	}
	defer conn.Close()
	client := notificationpb.NewNotificationServiceClient(conn)

	// Role-specific gRPC Contexts
	customerCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-user-id", customerID,
		"x-user-role", "CUSTOMER",
	))
	merchantCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-user-id", merchantID,
		"x-user-role", "MERCHANT",
	))
	deliveryPartnerCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-user-id", deliveryPartnerID,
		"x-user-role", "DELIVERY_PARTNER",
	))

	// =========================================================================
	// PHASE 1: Register Push Subscriptions for User, Merchant, Delivery Partner
	// =========================================================================
	t.Log("--- PHASE 1: Registering Push Subscriptions across User, Merchant & Delivery Partner ---")

	// Customer: Desktop Chrome & Mobile Safari
	custChrome := createDemoBrowser(t, "Customer Chrome", "https://fcm.googleapis.com/fcm/send/cust-chrome-"+uuid.NewString()[:8])
	custSafari := createDemoBrowser(t, "Customer Safari", "https://web.push.apple.com/cust-safari-"+uuid.NewString()[:8])
	pushGateway.registerBrowser(custChrome)
	pushGateway.registerBrowser(custSafari)

	regCust1, err := client.RegisterPushSubscription(customerCtx, &notificationpb.RegisterPushSubscriptionRequest{
		UserId:     customerID,
		Endpoint:   custChrome.endpoint,
		P256Dh:     custChrome.p256dh,
		Auth:       custChrome.auth,
		UserAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/128.0",
		DeviceName: "Customer Laptop (Chrome)",
	})
	if err != nil || regCust1.GetSubscription() == nil {
		t.Fatalf("Customer RegisterPushSubscription Chrome failed: %v", err)
	}

	regCust2, err := client.RegisterPushSubscription(customerCtx, &notificationpb.RegisterPushSubscriptionRequest{
		UserId:     customerID,
		Endpoint:   custSafari.endpoint,
		P256Dh:     custSafari.p256dh,
		Auth:       custSafari.auth,
		UserAgent:  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0) Mobile/15E148 Safari/604.1",
		DeviceName: "Customer iPhone (Safari)",
	})
	if err != nil || regCust2.GetSubscription() == nil {
		t.Fatalf("Customer RegisterPushSubscription Safari failed: %v", err)
	}

	// Merchant: Dashboard POS Browser
	merchPOS := createDemoBrowser(t, "Merchant POS Dashboard", "https://fcm.googleapis.com/fcm/send/merch-pos-"+uuid.NewString()[:8])
	pushGateway.registerBrowser(merchPOS)

	regMerch, err := client.RegisterPushSubscription(merchantCtx, &notificationpb.RegisterPushSubscriptionRequest{
		UserId:     merchantID,
		Endpoint:   merchPOS.endpoint,
		P256Dh:     merchPOS.p256dh,
		Auth:       merchPOS.auth,
		UserAgent:  "Mozilla/5.0 (X11; Linux x86_64) Chrome/126.0",
		DeviceName: "Merchant POS Terminal",
	})
	if err != nil || regMerch.GetSubscription() == nil {
		t.Fatalf("Merchant RegisterPushSubscription failed: %v", err)
	}

	// Delivery Partner: Rider Mobile App
	riderApp := createDemoBrowser(t, "Rider Android App", "https://fcm.googleapis.com/fcm/send/rider-app-"+uuid.NewString()[:8])
	pushGateway.registerBrowser(riderApp)

	regRider, err := client.RegisterPushSubscription(deliveryPartnerCtx, &notificationpb.RegisterPushSubscriptionRequest{
		UserId:     deliveryPartnerID,
		Endpoint:   riderApp.endpoint,
		P256Dh:     riderApp.p256dh,
		Auth:       riderApp.auth,
		UserAgent:  "GoCart-Delivery-Android/2.4.1",
		DeviceName: "Rider Samsung Galaxy A54",
	})
	if err != nil || regRider.GetSubscription() == nil {
		t.Fatalf("Delivery Partner RegisterPushSubscription failed: %v", err)
	}

	t.Log("[SUCCESS] Push subscriptions registered: 2 User devices, 1 Merchant POS, 1 Delivery Partner app")

	// =========================================================================
	// PHASE 2: Trigger Sample Notifications for USER (Customer)
	// =========================================================================
	t.Log("--- PHASE 2: Triggering Sample Notifications for USER (Customer) ---")

	// Sample U1: Customer places an order -> OrderCreatedEvent via Kafka
	envOrderCreated, err := events.NewEventEnvelope(events.EventTypeOrderCreated, "order-service", events.OrderCreatedEvent{
		OrderID:     orderID,
		UserID:      customerID,
		TotalAmount: 2499.00,
		Currency:    "INR",
	})
	if err != nil {
		t.Fatalf("NewEventEnvelope(OrderCreated) failed: %v", err)
	}
	envOrderCreated.CorrelationID = "corr-user-order-1"

	if err := evtConsumer.HandleEvent(ctx, envOrderCreated); err != nil {
		t.Fatalf("evtConsumer.HandleEvent(OrderCreated) failed: %v", err)
	}

	// Sample U2: Payment Successful -> direct SendNotification
	payResp, err := client.SendNotification(customerCtx, &notificationpb.SendNotificationRequest{
		UserId:        customerID,
		Channel:       "PUSH",
		EventType:     "PaymentSuccessful",
		Recipient:     customerID,
		Subject:       fmt.Sprintf("Payment Received - Order #%s", orderID),
		Content:       fmt.Sprintf("Payment of ₹2,499.00 INR for Order #%s was successful.", orderID),
		CorrelationId: "corr-user-pay-1",
		TemplateData: map[string]string{
			"order_id": orderID,
			"amount":   "2499.00",
			"currency": "INR",
		},
	})
	if err != nil || !payResp.GetSuccess() {
		t.Fatalf("SendNotification(PaymentSuccessful) failed: %v", err)
	}

	// Sample U3: Customer Cancels Order -> OrderCancelledEvent via Kafka
	envOrderCancelled, err := events.NewEventEnvelope(events.EventTypeOrderCancelled, "order-service", events.OrderCancelledEvent{
		OrderID: orderID,
		UserID:  customerID,
		Reason:  "Customer requested cancellation before dispatch",
	})
	if err != nil {
		t.Fatalf("NewEventEnvelope(OrderCancelled) failed: %v", err)
	}
	envOrderCancelled.CorrelationID = "corr-user-cancel-1"

	if err := evtConsumer.HandleEvent(ctx, envOrderCancelled); err != nil {
		t.Fatalf("evtConsumer.HandleEvent(OrderCancelled) failed: %v", err)
	}

	t.Log("[SUCCESS] Triggered 3 User notifications: OrderCreated, PaymentSuccessful, OrderCancelled")

	// =========================================================================
	// PHASE 3: Trigger Sample Notifications for MERCHANT
	// =========================================================================
	t.Log("--- PHASE 3: Triggering Sample Notifications for MERCHANT ---")

	// Sample M1: Merchant Store Approved
	merchApproveResp, err := client.SendNotification(merchantCtx, &notificationpb.SendNotificationRequest{
		UserId:        merchantID,
		Channel:       "PUSH",
		EventType:     "MerchantApproved",
		Recipient:     merchantID,
		Subject:       "Merchant Store Approved!",
		Content:       "Congratulations! Your merchant store 'Gadget World' has been approved on GoCart.",
		CorrelationId: "corr-merch-approve-1",
		TemplateData: map[string]string{
			"merchant_id": merchantID,
		},
	})
	if err != nil || !merchApproveResp.GetSuccess() {
		t.Fatalf("SendNotification(MerchantApproved) failed: %v", err)
	}

	// Sample M2: New Order Received for Merchant
	merchOrderAlert, err := client.SendNotification(merchantCtx, &notificationpb.SendNotificationRequest{
		UserId:        merchantID,
		Channel:       "PUSH",
		EventType:     "OrderPlacedForMerchant",
		Recipient:     merchantID,
		Subject:       fmt.Sprintf("New Order Received: #%s", orderID),
		Content:       fmt.Sprintf("You received a new order #%s (₹2,499.00). Please pack the items for courier pickup.", orderID),
		CorrelationId: "corr-merch-order-alert",
		TemplateData: map[string]string{
			"order_id": orderID,
			"amount":   "2499.00",
		},
	})
	if err != nil || !merchOrderAlert.GetSuccess() {
		t.Fatalf("SendNotification(OrderPlacedForMerchant) failed: %v", err)
	}

	// Sample M3: Merchant dispatches delivery for Customer (Merchant operational trigger)
	// Merchant notifies customer that delivery has been booked
	merchDispatchNotif, err := client.SendNotification(merchantCtx, &notificationpb.SendNotificationRequest{
		UserId:        customerID, // target is customer
		Channel:       "PUSH",
		EventType:     "DeliveryCreated",
		Recipient:     customerID,
		Subject:       fmt.Sprintf("Delivery Scheduled - Order #%s", orderID),
		Content:       fmt.Sprintf("Your order #%s has been packed and scheduled for delivery via Express Logistics.", orderID),
		CorrelationId: "corr-merch-dispatch-1",
		TemplateData: map[string]string{
			"order_id":         orderID,
			"delivery_partner": "Express Logistics",
			"tracking_number":  "EXP-770011",
		},
	})
	if err != nil || !merchDispatchNotif.GetSuccess() {
		t.Fatalf("Merchant SendNotification(DeliveryCreated for customer) failed: %v", err)
	}

	t.Log("[SUCCESS] Triggered 3 Merchant notifications: StoreApproved, NewOrderAlert, DispatchForCustomer")

	// =========================================================================
	// PHASE 4: Trigger Sample Notifications for DELIVERY PARTNER
	// =========================================================================
	t.Log("--- PHASE 4: Triggering Sample Notifications for DELIVERY PARTNER ---")

	// Sample D1: Delivery Assignment for Delivery Partner
	riderAssignResp, err := client.SendNotification(deliveryPartnerCtx, &notificationpb.SendNotificationRequest{
		UserId:        deliveryPartnerID,
		Channel:       "PUSH",
		EventType:     "DeliveryAssigned",
		Recipient:     deliveryPartnerID,
		Subject:       fmt.Sprintf("New Delivery Assigned: #%s", deliveryID),
		Content:       fmt.Sprintf("New pickup assigned for Order #%s. Pickup: Merchant Warehouse Bay 4. Drop: Koramangala, Bangalore.", orderID),
		CorrelationId: "corr-rider-assign-1",
		TemplateData: map[string]string{
			"delivery_id": deliveryID,
			"order_id":    orderID,
		},
	})
	if err != nil || !riderAssignResp.GetSuccess() {
		t.Fatalf("SendNotification(DeliveryAssigned for Rider) failed: %v", err)
	}

	// Sample D2: Delivery Partner updates status to OUT_FOR_DELIVERY
	// Triggers notification to customer
	delStatusEnv, err := events.NewEventEnvelope(events.EventTypeDeliveryStatusUpdated, "delivery-service", events.DeliveryStatusUpdatedEvent{
		DeliveryID:     deliveryID,
		OrderID:        orderID,
		PreviousStatus: "DISPATCHED",
		NewStatus:      "OUT_FOR_DELIVERY",
		TrackingNumber: "EXP-770011",
		UserID:         customerID,
	})
	if err != nil {
		t.Fatalf("NewEventEnvelope(DeliveryStatusUpdated) failed: %v", err)
	}
	delStatusEnv.CorrelationID = "corr-rider-out-for-delivery"

	if err := evtConsumer.HandleEvent(ctx, delStatusEnv); err != nil {
		t.Fatalf("evtConsumer.HandleEvent(DeliveryStatusUpdated) failed: %v", err)
	}

	// Sample D3: Delivery Partner marks order DELIVERED
	// Sends confirmation to delivery partner
	riderDeliveredResp, err := client.SendNotification(deliveryPartnerCtx, &notificationpb.SendNotificationRequest{
		UserId:        deliveryPartnerID,
		Channel:       "PUSH",
		EventType:     "DeliveryCompleted",
		Recipient:     deliveryPartnerID,
		Subject:       fmt.Sprintf("Delivery #%s Completed!", deliveryID),
		Content:       fmt.Sprintf("Delivery for Order #%s was successfully delivered and verified with OTP. Great job!", orderID),
		CorrelationId: "corr-rider-delivered-1",
		TemplateData: map[string]string{
			"delivery_id": deliveryID,
			"order_id":    orderID,
		},
	})
	if err != nil || !riderDeliveredResp.GetSuccess() {
		t.Fatalf("SendNotification(DeliveryCompleted for Rider) failed: %v", err)
	}

	t.Log("[SUCCESS] Triggered 3 Delivery Partner notifications: Assignment, StatusUpdate, DeliveryCompleted")

	// =========================================================================
	// PHASE 5: Verify Web Push Delivery to Device Endpoints
	// =========================================================================
	t.Log("--- PHASE 5: Verifying Decrypted Web Push Payloads on Device Endpoints ---")

	deliveredMessages := pushGateway.snapshotDelivered()
	if len(deliveredMessages) == 0 {
		t.Fatalf("expected web push messages to be delivered, got 0")
	}

	custReceivedCount := 0
	merchReceivedCount := 0
	riderReceivedCount := 0

	for _, msg := range deliveredMessages {
		switch {
		case strings.HasPrefix(msg.Endpoint, custChrome.endpoint) || strings.HasPrefix(msg.Endpoint, custSafari.endpoint):
			custReceivedCount++
			t.Logf("  [PUSH -> CUSTOMER DEVICE: %s] title=%q body=%q event=%s",
				msg.DeviceName, msg.Payload.Title, msg.Payload.Body, msg.Payload.Data["event_type"])
		case strings.HasPrefix(msg.Endpoint, merchPOS.endpoint):
			merchReceivedCount++
			t.Logf("  [PUSH -> MERCHANT POS: %s] title=%q body=%q event=%s",
				msg.DeviceName, msg.Payload.Title, msg.Payload.Body, msg.Payload.Data["event_type"])
		case strings.HasPrefix(msg.Endpoint, riderApp.endpoint):
			riderReceivedCount++
			t.Logf("  [PUSH -> RIDER APP: %s] title=%q body=%q event=%s",
				msg.DeviceName, msg.Payload.Title, msg.Payload.Body, msg.Payload.Data["event_type"])
		default:
			t.Errorf("unexpected push message delivered to endpoint: %s", msg.Endpoint)
		}
	}

	if custReceivedCount == 0 {
		t.Errorf("expected customer to receive push messages, got 0")
	}
	if merchReceivedCount == 0 {
		t.Errorf("expected merchant to receive push messages, got 0")
	}
	if riderReceivedCount == 0 {
		t.Errorf("expected delivery partner to receive push messages, got 0")
	}

	t.Logf("[SUCCESS] Web Push delivered: %d Customer pushes, %d Merchant pushes, %d Rider pushes",
		custReceivedCount, merchReceivedCount, riderReceivedCount)

	// =========================================================================
	// PHASE 6: Verify Database Records & Read Management for Each Role
	// =========================================================================
	t.Log("--- PHASE 6: Verifying DB Storage & Read Management for Each Role ---")

	// 1. Customer Notifications in DB
	custNotifResp, err := client.GetNotifications(customerCtx, &notificationpb.GetNotificationsRequest{
		UserId: customerID,
		Limit:  20,
	})
	if err != nil {
		t.Fatalf("Customer GetNotifications failed: %v", err)
	}
	// Customer should have: OrderCreated, PaymentSuccessful, OrderCancelled, DeliveryCreated (from merchant), DeliveryStatusUpdated (from rider)
	if len(custNotifResp.GetNotifications()) < 4 {
		t.Fatalf("expected at least 4 customer notifications in DB, got %d", len(custNotifResp.GetNotifications()))
	}
	t.Logf("Customer has %d notifications stored in DB (total: %d)", len(custNotifResp.GetNotifications()), custNotifResp.GetTotal())

	// Customer marks 1 notification as read
	targetNotif := custNotifResp.GetNotifications()[0]
	markReadResp, err := client.MarkNotificationAsRead(customerCtx, &notificationpb.MarkNotificationAsReadRequest{
		NotificationId: targetNotif.GetId(),
		UserId:         customerID,
	})
	if err != nil || !markReadResp.GetSuccess() {
		t.Fatalf("Customer MarkNotificationAsRead failed: %v", err)
	}

	// Verify unread count decremented
	unreadCust, err := client.GetUnreadCount(customerCtx, &notificationpb.GetUnreadCountRequest{
		UserId: customerID,
	})
	if err != nil {
		t.Fatalf("GetUnreadCount failed: %v", err)
	}
	expectedUnread := int32(len(custNotifResp.GetNotifications()) - 1)
	if unreadCust.GetUnreadCount() != expectedUnread {
		t.Errorf("expected %d unread notifications for customer, got %d", expectedUnread, unreadCust.GetUnreadCount())
	}

	// Customer marks ALL notifications as read
	markAllCust, err := client.MarkAllNotificationsAsRead(customerCtx, &notificationpb.MarkAllNotificationsAsReadRequest{
		UserId: customerID,
	})
	if err != nil || !markAllCust.GetSuccess() {
		t.Fatalf("Customer MarkAllNotificationsAsRead failed: %v", err)
	}

	unreadCustAfter, err := client.GetUnreadCount(customerCtx, &notificationpb.GetUnreadCountRequest{
		UserId: customerID,
	})
	if err != nil || unreadCustAfter.GetUnreadCount() != 0 {
		t.Fatalf("expected 0 unread customer notifications after MarkAllNotificationsAsRead, got %d", unreadCustAfter.GetUnreadCount())
	}
	t.Log("[SUCCESS] Customer notifications: read receipt management 100% verified")

	// 2. Merchant Notifications in DB
	merchNotifResp, err := client.GetNotifications(merchantCtx, &notificationpb.GetNotificationsRequest{
		UserId: merchantID,
		Limit:  20,
	})
	if err != nil {
		t.Fatalf("Merchant GetNotifications failed: %v", err)
	}
	// Merchant should have: MerchantApproved, OrderPlacedForMerchant
	if len(merchNotifResp.GetNotifications()) < 2 {
		t.Fatalf("expected at least 2 merchant notifications in DB, got %d", len(merchNotifResp.GetNotifications()))
	}
	t.Logf("Merchant has %d notifications stored in DB", len(merchNotifResp.GetNotifications()))

	// Merchant marks all as read
	markAllMerch, err := client.MarkAllNotificationsAsRead(merchantCtx, &notificationpb.MarkAllNotificationsAsReadRequest{
		UserId: merchantID,
	})
	if err != nil || !markAllMerch.GetSuccess() {
		t.Fatalf("Merchant MarkAllNotificationsAsRead failed: %v", err)
	}

	unreadMerch, err := client.GetUnreadCount(merchantCtx, &notificationpb.GetUnreadCountRequest{
		UserId: merchantID,
	})
	if err != nil || unreadMerch.GetUnreadCount() != 0 {
		t.Fatalf("expected 0 unread merchant notifications, got %d", unreadMerch.GetUnreadCount())
	}
	t.Log("[SUCCESS] Merchant notifications: read receipt management 100% verified")

	// 3. Delivery Partner Notifications in DB
	riderNotifResp, err := client.GetNotifications(deliveryPartnerCtx, &notificationpb.GetNotificationsRequest{
		UserId: deliveryPartnerID,
		Limit:  20,
	})
	if err != nil {
		t.Fatalf("Delivery Partner GetNotifications failed: %v", err)
	}
	// Delivery Partner should have: DeliveryAssigned, DeliveryCompleted
	if len(riderNotifResp.GetNotifications()) < 2 {
		t.Fatalf("expected at least 2 delivery partner notifications in DB, got %d", len(riderNotifResp.GetNotifications()))
	}
	t.Logf("Delivery Partner has %d notifications stored in DB", len(riderNotifResp.GetNotifications()))

	// Delivery partner marks all as read
	markAllRider, err := client.MarkAllNotificationsAsRead(deliveryPartnerCtx, &notificationpb.MarkAllNotificationsAsReadRequest{
		UserId: deliveryPartnerID,
	})
	if err != nil || !markAllRider.GetSuccess() {
		t.Fatalf("Delivery Partner MarkAllNotificationsAsRead failed: %v", err)
	}

	unreadRider, err := client.GetUnreadCount(deliveryPartnerCtx, &notificationpb.GetUnreadCountRequest{
		UserId: deliveryPartnerID,
	})
	if err != nil || unreadRider.GetUnreadCount() != 0 {
		t.Fatalf("expected 0 unread rider notifications, got %d", unreadRider.GetUnreadCount())
	}
	t.Log("[SUCCESS] Delivery Partner notifications: read receipt management 100% verified")

	// =========================================================================
	// PHASE 7: Strict Cross-Role Security & Isolation Verification
	// =========================================================================
	t.Log("--- PHASE 7: Verifying Cross-Role Security & Isolation ---")

	// 1. Customer CANNOT read Merchant notifications
	_, err = client.GetNotifications(customerCtx, &notificationpb.GetNotificationsRequest{
		UserId: merchantID,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied when Customer accesses Merchant notifications, got code=%v err=%v", status.Code(err), err)
	}

	// 2. Customer CANNOT read Delivery Partner notifications
	_, err = client.GetNotifications(customerCtx, &notificationpb.GetNotificationsRequest{
		UserId: deliveryPartnerID,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied when Customer accesses Delivery Partner notifications, got code=%v err=%v", status.Code(err), err)
	}

	// 3. Customer CANNOT mark Merchant's notifications as read
	_, err = client.MarkAllNotificationsAsRead(customerCtx, &notificationpb.MarkAllNotificationsAsReadRequest{
		UserId: merchantID,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied when Customer calls MarkAllNotificationsAsRead on Merchant, got code=%v err=%v", status.Code(err), err)
	}

	// 4. Customer CANNOT view Delivery Partner's push subscriptions
	_, err = client.GetPushSubscriptions(customerCtx, &notificationpb.GetPushSubscriptionsRequest{
		UserId: deliveryPartnerID,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied when Customer inspects Delivery Partner subscriptions, got code=%v err=%v", status.Code(err), err)
	}

	t.Log("[SUCCESS] Cross-role security & isolation verified: Customer cannot read or mutate Merchant/Rider data")
	t.Log("=== ALL SAMPLE NOTIFICATIONS FOR USER, MERCHANT, AND DELIVERY PARTNER 100% VERIFIED ===")
}
