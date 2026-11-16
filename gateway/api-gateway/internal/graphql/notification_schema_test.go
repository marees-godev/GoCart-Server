package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	notificationpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/notification"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/client"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	gwResolver "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	gatewayGRPC "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

type mockGraphQLNotificationClient struct {
	notificationpb.NotificationServiceClient
	notificationpb.UnimplementedNotificationServiceServer
	lastRegisterReq   *notificationpb.RegisterPushSubscriptionRequest
	lastListSubsReq   *notificationpb.GetPushSubscriptionsRequest
	lastUpdateSubReq  *notificationpb.UpdatePushSubscriptionRequest
	lastDeactSubReq   *notificationpb.DeactivatePushSubscriptionRequest
	lastListNotifsReq *notificationpb.GetNotificationsRequest
	lastMarkReadReq   *notificationpb.MarkNotificationAsReadRequest
	lastMarkAllReq    *notificationpb.MarkAllNotificationsAsReadRequest
	lastUnreadReq     *notificationpb.GetUnreadCountRequest
	lastSendReq       *notificationpb.SendNotificationRequest
	lastCallerUserID  string
	lastCallerRole    string
}

func (m *mockGraphQLNotificationClient) GetVAPIDPublicKey(ctx context.Context, in *notificationpb.GetVAPIDPublicKeyRequest, opts ...grpc.CallOption) (*notificationpb.GetVAPIDPublicKeyResponse, error) {
	return &notificationpb.GetVAPIDPublicKeyResponse{
		PublicKey: "BVapidPubKey123",
		Subject:   "mailto:push@gocart.local",
	}, nil
}

func (m *mockGraphQLNotificationClient) RegisterPushSubscription(ctx context.Context, in *notificationpb.RegisterPushSubscriptionRequest, opts ...grpc.CallOption) (*notificationpb.PushSubscriptionResponse, error) {
	m.lastRegisterReq = in
	return &notificationpb.PushSubscriptionResponse{
		Subscription: &notificationpb.PushSubscription{
			Id:         "sub-gql-1",
			UserId:     in.GetUserId(),
			Endpoint:   in.GetEndpoint(),
			UserAgent:  in.GetUserAgent(),
			DeviceName: in.GetDeviceName(),
			IsActive:   true,
		},
	}, nil
}

func (m *mockGraphQLNotificationClient) GetPushSubscriptions(ctx context.Context, in *notificationpb.GetPushSubscriptionsRequest, opts ...grpc.CallOption) (*notificationpb.GetPushSubscriptionsResponse, error) {
	m.lastListSubsReq = in
	return &notificationpb.GetPushSubscriptionsResponse{
		Subscriptions: []*notificationpb.PushSubscription{
			{Id: "sub-gql-1", UserId: in.GetUserId(), Endpoint: "https://push.example.com/1", IsActive: true},
		},
		Total: 1,
	}, nil
}

func (m *mockGraphQLNotificationClient) UpdatePushSubscription(ctx context.Context, in *notificationpb.UpdatePushSubscriptionRequest, opts ...grpc.CallOption) (*notificationpb.PushSubscriptionResponse, error) {
	m.lastUpdateSubReq = in
	return &notificationpb.PushSubscriptionResponse{
		Subscription: &notificationpb.PushSubscription{
			Id:         in.GetId(),
			UserId:     in.GetUserId(),
			Endpoint:   "https://push.example.com/1",
			DeviceName: in.GetDeviceName(),
			IsActive:   in.GetIsActive(),
		},
	}, nil
}

func (m *mockGraphQLNotificationClient) DeactivatePushSubscription(ctx context.Context, in *notificationpb.DeactivatePushSubscriptionRequest, opts ...grpc.CallOption) (*notificationpb.DeactivatePushSubscriptionResponse, error) {
	m.lastDeactSubReq = in
	return &notificationpb.DeactivatePushSubscriptionResponse{Success: true}, nil
}

func (m *mockGraphQLNotificationClient) SendNotification(ctx context.Context, in *notificationpb.SendNotificationRequest, opts ...grpc.CallOption) (*notificationpb.SendNotificationResponse, error) {
	m.lastSendReq = in
	return &notificationpb.SendNotificationResponse{
		NotificationId: "notif-gql-1",
		Status:         "SENT",
		Success:        true,
	}, nil
}

func (m *mockGraphQLNotificationClient) GetNotifications(ctx context.Context, in *notificationpb.GetNotificationsRequest, opts ...grpc.CallOption) (*notificationpb.GetNotificationsResponse, error) {
	m.lastListNotifsReq = in
	m.lastCallerUserID = grpcclient.GetUserID(ctx)
	m.lastCallerRole = grpcclient.GetUserRole(ctx)
	return &notificationpb.GetNotificationsResponse{
		Notifications: []*notificationpb.Notification{
			{
				Id:        "notif-gql-1",
				UserId:    in.GetUserId(),
				Channel:   "PUSH",
				EventType: "OrderConfirmed",
				Subject:   "Order Confirmed",
				Content:   "Confirmed!",
				Status:    "SENT",
				IsRead:    false,
			},
		},
		Total:       1,
		UnreadCount: 1,
	}, nil
}

func (m *mockGraphQLNotificationClient) GetUnreadCount(ctx context.Context, in *notificationpb.GetUnreadCountRequest, opts ...grpc.CallOption) (*notificationpb.GetUnreadCountResponse, error) {
	m.lastUnreadReq = in
	return &notificationpb.GetUnreadCountResponse{UnreadCount: 2}, nil
}

func (m *mockGraphQLNotificationClient) MarkNotificationAsRead(ctx context.Context, in *notificationpb.MarkNotificationAsReadRequest, opts ...grpc.CallOption) (*notificationpb.MarkNotificationAsReadResponse, error) {
	m.lastMarkReadReq = in
	isRead := true
	if in.IsRead != nil {
		isRead = in.GetIsRead()
	}
	return &notificationpb.MarkNotificationAsReadResponse{
		Notification: &notificationpb.Notification{
			Id:     in.GetNotificationId(),
			UserId: in.GetUserId(),
			IsRead: isRead,
			Status: "SENT",
		},
		Success: true,
	}, nil
}

func (m *mockGraphQLNotificationClient) MarkAllNotificationsAsRead(ctx context.Context, in *notificationpb.MarkAllNotificationsAsReadRequest, opts ...grpc.CallOption) (*notificationpb.MarkAllNotificationsAsReadResponse, error) {
	m.lastMarkAllReq = in
	return &notificationpb.MarkAllNotificationsAsReadResponse{
		UpdatedCount: 2,
		Success:      true,
	}, nil
}

type mockNotificationBufconnServer struct {
	notificationpb.UnimplementedNotificationServiceServer
	inner *mockGraphQLNotificationClient
}

func (s *mockNotificationBufconnServer) GetVAPIDPublicKey(ctx context.Context, in *notificationpb.GetVAPIDPublicKeyRequest) (*notificationpb.GetVAPIDPublicKeyResponse, error) {
	return s.inner.GetVAPIDPublicKey(ctx, in)
}

func (s *mockNotificationBufconnServer) RegisterPushSubscription(ctx context.Context, in *notificationpb.RegisterPushSubscriptionRequest) (*notificationpb.PushSubscriptionResponse, error) {
	return s.inner.RegisterPushSubscription(ctx, in)
}

func (s *mockNotificationBufconnServer) GetPushSubscriptions(ctx context.Context, in *notificationpb.GetPushSubscriptionsRequest) (*notificationpb.GetPushSubscriptionsResponse, error) {
	return s.inner.GetPushSubscriptions(ctx, in)
}

func (s *mockNotificationBufconnServer) UpdatePushSubscription(ctx context.Context, in *notificationpb.UpdatePushSubscriptionRequest) (*notificationpb.PushSubscriptionResponse, error) {
	return s.inner.UpdatePushSubscription(ctx, in)
}

func (s *mockNotificationBufconnServer) DeactivatePushSubscription(ctx context.Context, in *notificationpb.DeactivatePushSubscriptionRequest) (*notificationpb.DeactivatePushSubscriptionResponse, error) {
	return s.inner.DeactivatePushSubscription(ctx, in)
}

func (s *mockNotificationBufconnServer) SendNotification(ctx context.Context, in *notificationpb.SendNotificationRequest) (*notificationpb.SendNotificationResponse, error) {
	return s.inner.SendNotification(ctx, in)
}

func (s *mockNotificationBufconnServer) GetNotifications(ctx context.Context, in *notificationpb.GetNotificationsRequest) (*notificationpb.GetNotificationsResponse, error) {
	return s.inner.GetNotifications(ctx, in)
}

func (s *mockNotificationBufconnServer) GetUnreadCount(ctx context.Context, in *notificationpb.GetUnreadCountRequest) (*notificationpb.GetUnreadCountResponse, error) {
	return s.inner.GetUnreadCount(ctx, in)
}

func (s *mockNotificationBufconnServer) MarkNotificationAsRead(ctx context.Context, in *notificationpb.MarkNotificationAsReadRequest) (*notificationpb.MarkNotificationAsReadResponse, error) {
	return s.inner.MarkNotificationAsRead(ctx, in)
}

func (s *mockNotificationBufconnServer) MarkAllNotificationsAsRead(ctx context.Context, in *notificationpb.MarkAllNotificationsAsReadRequest) (*notificationpb.MarkAllNotificationsAsReadResponse, error) {
	return s.inner.MarkAllNotificationsAsRead(ctx, in)
}

func setupNotificationGraphQLApp() (*fiber.App, *mockGraphQLNotificationClient) {
	mockNotif := &mockGraphQLNotificationClient{}
	clients := gatewayGRPC.NewClientsWithServices(
		&mockUserClient{},
		&mockAuthClient{},
		mockNotif,
	)
	resolver := gwResolver.NewResolver(clients)
	cfg := &config.Config{
		GraphQLIntrospectionEnabled: true,
		AdminAPIKey:                 testAdminKey,
		AdminUserID:                 testAdminUserID,
		JWT: config.JWTConfig{
			Secret: testJWTSecret,
			Issuer: "gocart-api-gateway",
		},
	}
	schema, _ := NewSchema(resolver, cfg)
	h := NewHandler(schema, cfg)
	app := fiber.New()
	h.RegisterRoutes(app)
	return app, mockNotif
}

func execGraphQLWithHeaders(t *testing.T, app *fiber.App, headers map[string]string, query string) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"query": query})
	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	var res map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&res)
	return res
}

func execGraphQL(t *testing.T, app *fiber.App, token string, query string) map[string]any {
	t.Helper()
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	return execGraphQLWithHeaders(t, app, headers, query)
}

func TestGraphQL_NotificationsAndPushSubscriptions(t *testing.T) {
	app, mockNotif := setupNotificationGraphQLApp()

	// Unauthenticated query fails with UNAUTHORIZED
	unauthRes := execGraphQL(t, app, "", `query { unreadNotificationCount }`)
	errs, ok := unauthRes["errors"].([]any)
	if !ok || len(errs) == 0 {
		t.Fatalf("expected authorization error for unauthenticated GraphQL query, got %v", unauthRes)
	}

	token, err := auth.GenerateToken(auth.UserContext{
		UserID: "user-gql-1",
		Role:   auth.RoleCustomer,
		Email:  "customer@gocart.com",
	}, testJWTSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	// VAPID public key
	vapidRes := execGraphQL(t, app, token, `query { vapidPublicKey { publicKey subject } }`)
	if vapidRes["errors"] != nil {
		t.Fatalf("vapidPublicKey failed: %v", vapidRes["errors"])
	}
	vData := vapidRes["data"].(map[string]any)["vapidPublicKey"].(map[string]any)
	if vData["publicKey"] != "BVapidPubKey123" || vData["subject"] != "mailto:push@gocart.local" {
		t.Errorf("unexpected vapidPublicKey: %v", vData)
	}

	// Register + List + Update + Deactivate subscription (by ID and by endpoint)
	regRes := execGraphQL(t, app, token, `mutation { registerPushSubscription(input: { endpoint: "https://push.example.com/1", p256dh: "k1", auth: "a1", userAgent: "Mozilla/5.0 Chrome/134", deviceName: "Chrome" }) { id endpoint userAgent isActive } }`)
	if regRes["errors"] != nil {
		t.Fatalf("registerPushSubscription failed: %v", regRes["errors"])
	}
	if mockNotif.lastRegisterReq.GetUserId() != "user-gql-1" ||
		mockNotif.lastRegisterReq.GetUserAgent() != "Mozilla/5.0 Chrome/134" {
		t.Errorf("unexpected register request forwarded: %+v", mockNotif.lastRegisterReq)
	}

	listSubsRes := execGraphQL(t, app, token, `query { pushSubscriptions(activeOnly: true) { total subscriptions { id endpoint isActive } } }`)
	if listSubsRes["errors"] != nil {
		t.Fatalf("pushSubscriptions failed: %v", listSubsRes["errors"])
	}
	if !mockNotif.lastListSubsReq.GetActiveOnly() || mockNotif.lastListSubsReq.GetUserId() != "user-gql-1" {
		t.Errorf("unexpected pushSubscriptions request: %+v", mockNotif.lastListSubsReq)
	}

	updRes := execGraphQL(t, app, token, `mutation { updatePushSubscription(input: { id: "sub-gql-1", deviceName: "Updated Chrome", isActive: false }) { id deviceName isActive } }`)
	if updRes["errors"] != nil {
		t.Fatalf("updatePushSubscription failed: %v", updRes["errors"])
	}
	if mockNotif.lastUpdateSubReq.GetId() != "sub-gql-1" || mockNotif.lastUpdateSubReq.GetIsActive() {
		t.Errorf("unexpected updatePushSubscription request: %+v", mockNotif.lastUpdateSubReq)
	}

	deactByID := execGraphQL(t, app, token, `mutation { deactivatePushSubscription(id: "sub-gql-1") }`)
	if deactByID["errors"] != nil || deactByID["data"].(map[string]any)["deactivatePushSubscription"] != true {
		t.Fatalf("deactivatePushSubscription by id failed: %v", deactByID)
	}
	if mockNotif.lastDeactSubReq.GetId() != "sub-gql-1" {
		t.Errorf("expected deactivate id 'sub-gql-1', got %+v", mockNotif.lastDeactSubReq)
	}

	deactByEndpoint := execGraphQL(t, app, token, `mutation { deactivatePushSubscription(endpoint: "https://push.example.com/1") }`)
	if deactByEndpoint["errors"] != nil || deactByEndpoint["data"].(map[string]any)["deactivatePushSubscription"] != true {
		t.Fatalf("deactivatePushSubscription by endpoint failed: %v", deactByEndpoint)
	}
	if mockNotif.lastDeactSubReq.GetEndpoint() != "https://push.example.com/1" {
		t.Errorf("expected deactivate endpoint, got %+v", mockNotif.lastDeactSubReq)
	}

	// Notifications query with filter/pagination, unread count, mark read/unread, mark all read
	notifListRes := execGraphQL(t, app, token, `query { notifications(filter: { status: "SENT", channel: "PUSH", eventType: "OrderConfirmed", isRead: false }, limit: 10, offset: 5) { total unreadCount notifications { id status isRead eventType } } }`)
	if notifListRes["errors"] != nil {
		t.Fatalf("notifications query failed: %v", notifListRes["errors"])
	}
	if mockNotif.lastListNotifsReq.GetLimit() != 10 ||
		mockNotif.lastListNotifsReq.GetOffset() != 5 ||
		mockNotif.lastListNotifsReq.GetStatus() != "SENT" ||
		mockNotif.lastListNotifsReq.GetChannel() != "PUSH" ||
		mockNotif.lastListNotifsReq.GetEventType() != "OrderConfirmed" ||
		mockNotif.lastListNotifsReq.IsRead == nil ||
		mockNotif.lastListNotifsReq.GetIsRead() {
		t.Errorf("filters/pagination not forwarded accurately: %+v", mockNotif.lastListNotifsReq)
	}

	unreadRes := execGraphQL(t, app, token, `query { unreadNotificationCount }`)
	if unreadRes["errors"] != nil || mockNotif.lastUnreadReq.GetUserId() != "user-gql-1" {
		t.Fatalf("unreadNotificationCount failed: %v", unreadRes)
	}

	markOneRes := execGraphQL(t, app, token, `mutation { markNotificationAsRead(notificationId: "notif-gql-1", isRead: false) { id isRead } }`)
	if markOneRes["errors"] != nil {
		t.Fatalf("markNotificationAsRead failed: %v", markOneRes["errors"])
	}
	if mockNotif.lastMarkReadReq.IsRead == nil || mockNotif.lastMarkReadReq.GetIsRead() {
		t.Errorf("expected isRead=false to be forwarded, got %+v", mockNotif.lastMarkReadReq)
	}

	markAllRes := execGraphQL(t, app, token, `mutation { markAllNotificationsAsRead { updatedCount success } }`)
	if markAllRes["errors"] != nil || mockNotif.lastMarkAllReq.GetUserId() != "user-gql-1" {
		t.Fatalf("markAllNotificationsAsRead failed: %v", markAllRes)
	}

	// Customer cannot view another user's notifications or send notification to another user
	crossRes := execGraphQL(t, app, token, `query { notifications(userId: "other-user") { total } }`)
	if crossRes["errors"] == nil {
		t.Errorf("expected FORBIDDEN error for cross-user notifications query")
	}

	crossSendRes := execGraphQL(t, app, token, `mutation { sendNotification(input: { userId: "other-user", channel: "PUSH", subject: "Hi", content: "Body" }) }`)
	if crossSendRes["errors"] == nil {
		t.Errorf("expected FORBIDDEN error for cross-user sendNotification")
	}

	// Admin via X-Admin-Key can query another user's notifications and send notifications
	adminListRes := execGraphQLWithHeaders(t, app, map[string]string{
		HeaderXAdminKey: testAdminKey,
	}, `query { notifications(userId: "other-user") { total } }`)
	if adminListRes["errors"] != nil || mockNotif.lastListNotifsReq.GetUserId() != "other-user" {
		t.Fatalf("expected X-Admin-Key cross-user access to succeed, got res=%v req=%+v", adminListRes, mockNotif.lastListNotifsReq)
	}

	adminSendRes := execGraphQLWithHeaders(t, app, map[string]string{
		HeaderXAdminKey: testAdminKey,
	}, `mutation { sendNotification(input: { userId: "other-user", channel: "PUSH", subject: "Admin Alert", content: "Hello" }) }`)
	if adminSendRes["errors"] != nil || mockNotif.lastSendReq.GetUserId() != "other-user" {
		t.Fatalf("expected X-Admin-Key sendNotification to succeed, got res=%v req=%+v", adminSendRes, mockNotif.lastSendReq)
	}
}

func TestGraphQL_Notifications_ClientManagerBufconn(t *testing.T) {
	innerMock := &mockGraphQLNotificationClient{}
	srv := grpc.NewServer(grpc.UnaryInterceptor(grpcclient.UnaryServerInterceptor()))
	notificationpb.RegisterNotificationServiceServer(srv, &mockNotificationBufconnServer{inner: innerMock})

	lis := bufconn.Listen(1024 * 1024)
	go func() {
		_ = srv.Serve(lis)
	}()
	defer func() {
		srv.Stop()
		_ = lis.Close()
	}()

	cfg := &config.Config{
		GraphQLIntrospectionEnabled: true,
		AdminAPIKey:                 testAdminKey,
		AdminUserID:                 testAdminUserID,
		GRPC: config.GRPCConfig{
			NotificationServiceAddr: "passthrough://bufnet",
			DefaultTimeout:          2 * time.Second,
		},
		JWT: config.JWTConfig{
			Secret: testJWTSecret,
			Issuer: "gocart-api-gateway",
		},
	}

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
	clientMgr, err := client.NewClientManager(cfg, grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("NewClientManager failed: %v", err)
	}
	defer clientMgr.Close()

	resolver := gwResolver.NewResolverWithManager(nil, clientMgr, "1.0.0")
	schema, _ := NewSchema(resolver, cfg)
	h := NewHandler(schema, cfg)
	app := fiber.New()
	h.RegisterRoutes(app)

	token, err := auth.GenerateToken(auth.UserContext{
		UserID: "user-bufconn-1",
		Role:   auth.RoleCustomer,
		Email:  "bufconn@gocart.com",
	}, testJWTSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	res := execGraphQL(t, app, token, `query { notifications(limit: 5, offset: 0) { total notifications { id userId } } }`)
	if res["errors"] != nil {
		t.Fatalf("unexpected GraphQL error over bufconn ClientManager: %v", res["errors"])
	}
	if innerMock.lastCallerUserID != "user-bufconn-1" || innerMock.lastCallerRole != auth.RoleCustomer {
		t.Errorf("expected gRPC metadata propagation user=user-bufconn-1 role=%s, got user=%q role=%q", auth.RoleCustomer, innerMock.lastCallerUserID, innerMock.lastCallerRole)
	}
}

func TestGraphQL_Notifications_ServiceUnavailableWhenNoClient(t *testing.T) {
	resolver := gwResolver.NewResolver(nil)
	cfg := &config.Config{
		GraphQLIntrospectionEnabled: true,
		JWT: config.JWTConfig{
			Secret: testJWTSecret,
			Issuer: "gocart-api-gateway",
		},
	}
	schema, _ := NewSchema(resolver, cfg)
	h := NewHandler(schema, cfg)
	app := fiber.New()
	h.RegisterRoutes(app)

	token, _ := auth.GenerateToken(auth.UserContext{
		UserID: "user-1",
		Role:   auth.RoleCustomer,
	}, testJWTSecret, time.Hour)

	res := execGraphQL(t, app, token, `query { unreadNotificationCount }`)
	errs, ok := res["errors"].([]any)
	if !ok || len(errs) == 0 {
		t.Fatalf("expected SERVICE_UNAVAILABLE error, got %v", res)
	}
	ext, _ := errs[0].(map[string]any)["extensions"].(map[string]any)
	if ext["code"] != "SERVICE_UNAVAILABLE" {
		t.Errorf("expected SERVICE_UNAVAILABLE code, got %v", ext["code"])
	}
}

