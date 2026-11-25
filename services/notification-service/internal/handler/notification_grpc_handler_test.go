package handler_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	notificationpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/notification"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/push"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type memGRPCTestRepo struct {
	channels    map[string]*model.Channel
	prefs       map[string]*model.Preference
	notifs      map[string]*model.Notification
	byEventChan map[string]string
	subsByID    map[string]*model.PushSubscription
	subsByEnd   map[string]string
}

func newMemGRPCTestRepo() *memGRPCTestRepo {
	r := &memGRPCTestRepo{
		channels:    make(map[string]*model.Channel),
		prefs:       make(map[string]*model.Preference),
		notifs:      make(map[string]*model.Notification),
		byEventChan: make(map[string]string),
		subsByID:    make(map[string]*model.PushSubscription),
		subsByEnd:   make(map[string]string),
	}
	for _, code := range []string{model.ChannelPush, model.ChannelEmail, model.ChannelInApp} {
		c := &model.Channel{ID: uuid.NewString(), Code: code, Name: code, IsActive: true}
		r.channels[code] = c
		r.channels[c.ID] = c
	}
	return r
}

func (r *memGRPCTestRepo) GetByCode(ctx context.Context, code string) (*model.Channel, error) {
	return r.channels[code], nil
}
func (r *memGRPCTestRepo) GetByID(ctx context.Context, id string) (*model.Channel, error) {
	return r.channels[id], nil
}
func (r *memGRPCTestRepo) GetPreference(ctx context.Context, userID, channelID, category string) (*model.Preference, error) {
	return r.prefs[userID+":"+channelID+":"+category], nil
}
func (r *memGRPCTestRepo) UpsertPreference(ctx context.Context, pref *model.Preference) error {
	r.prefs[pref.UserID+":"+pref.ChannelID+":"+pref.Category] = pref
	return nil
}
func (r *memGRPCTestRepo) GetPreferencesByUserID(ctx context.Context, userID string) ([]*model.Preference, error) {
	var res []*model.Preference
	for _, p := range r.prefs {
		if p.UserID == userID {
			res = append(res, p)
		}
	}
	return res, nil
}
func (r *memGRPCTestRepo) CreateNotification(ctx context.Context, n *model.Notification) error {
	if n.ID == "" {
		n.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	n.CreatedAt = now
	n.UpdatedAt = now
	cp := *n
	r.notifs[cp.ID] = &cp
	if n.EventID != nil && *n.EventID != "" {
		r.byEventChan[*n.EventID+":"+n.ChannelID] = cp.ID
	}
	return nil
}
func (r *memGRPCTestRepo) GetNotificationByID(ctx context.Context, id string) (*model.Notification, error) {
	if n, ok := r.notifs[id]; ok {
		cp := *n
		return &cp, nil
	}
	return nil, nil
}
func (r *memGRPCTestRepo) GetNotificationByEventIDAndChannel(ctx context.Context, eventID, channelID string) (*model.Notification, error) {
	if id, ok := r.byEventChan[eventID+":"+channelID]; ok {
		cp := *r.notifs[id]
		return &cp, nil
	}
	return nil, nil
}
func (r *memGRPCTestRepo) ListNotifications(ctx context.Context, filter model.NotificationFilter) ([]*model.Notification, int, int, error) {
	var res []*model.Notification
	unread := 0
	for _, n := range r.notifs {
		if filter.UserID != "" && n.UserID != filter.UserID {
			continue
		}
		if !n.IsRead {
			unread++
		}
		if filter.Status != "" && string(n.Status) != filter.Status {
			continue
		}
		if filter.Channel != "" && n.ChannelCode != filter.Channel {
			continue
		}
		if filter.EventType != "" && n.EventType != filter.EventType {
			continue
		}
		if filter.IsRead != nil && n.IsRead != *filter.IsRead {
			continue
		}
		cp := *n
		res = append(res, &cp)
	}
	return res, len(res), unread, nil
}
func (r *memGRPCTestRepo) UpdateDeliveryStatus(ctx context.Context, id string, st model.DeliveryStatus, errMsg *string, inc bool) error {
	if n, ok := r.notifs[id]; ok {
		n.Status = st
		n.ErrorMessage = errMsg
		if st == model.DeliveryStatusSent {
			now := time.Now().UTC()
			n.SentAt = &now
		}
	}
	return nil
}
func (r *memGRPCTestRepo) MarkAsRead(ctx context.Context, id, userID string, isRead bool) (*model.Notification, error) {
	n, ok := r.notifs[id]
	if !ok || (userID != "" && n.UserID != userID) {
		return nil, nil
	}
	n.IsRead = isRead
	now := time.Now().UTC()
	if isRead {
		n.ReadAt = &now
	} else {
		n.ReadAt = nil
	}
	cp := *n
	return &cp, nil
}
func (r *memGRPCTestRepo) MarkAllAsRead(ctx context.Context, userID string) (int, error) {
	cnt := 0
	now := time.Now().UTC()
	for _, n := range r.notifs {
		if n.UserID == userID && !n.IsRead {
			n.IsRead = true
			n.ReadAt = &now
			cnt++
		}
	}
	return cnt, nil
}
func (r *memGRPCTestRepo) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	cnt := 0
	for _, n := range r.notifs {
		if n.UserID == userID && !n.IsRead {
			cnt++
		}
	}
	return cnt, nil
}
func (r *memGRPCTestRepo) UpsertSubscription(ctx context.Context, sub *model.PushSubscription) (*model.PushSubscription, error) {
	if existingID, ok := r.subsByEnd[sub.Endpoint]; ok {
		ex := r.subsByID[existingID]
		ex.UserID = sub.UserID
		ex.P256DH = sub.P256DH
		ex.Auth = sub.Auth
		ex.DeviceName = sub.DeviceName
		ex.IsActive = true
		cp := *ex
		return &cp, nil
	}
	sub.ID = uuid.NewString()
	now := time.Now().UTC()
	sub.CreatedAt = now
	sub.UpdatedAt = now
	sub.IsActive = true
	cp := *sub
	r.subsByID[cp.ID] = &cp
	r.subsByEnd[cp.Endpoint] = cp.ID
	return &cp, nil
}
func (r *memGRPCTestRepo) GetSubscriptionByID(ctx context.Context, id string) (*model.PushSubscription, error) {
	if s, ok := r.subsByID[id]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, nil
}
func (r *memGRPCTestRepo) GetSubscriptionByEndpoint(ctx context.Context, endpoint string) (*model.PushSubscription, error) {
	if id, ok := r.subsByEnd[endpoint]; ok {
		cp := *r.subsByID[id]
		return &cp, nil
	}
	return nil, nil
}
func (r *memGRPCTestRepo) ListSubscriptionsByUserID(ctx context.Context, userID string, activeOnly bool) ([]*model.PushSubscription, error) {
	var res []*model.PushSubscription
	for _, s := range r.subsByID {
		if s.UserID == userID && (!activeOnly || s.IsActive) {
			cp := *s
			res = append(res, &cp)
		}
	}
	return res, nil
}
func (r *memGRPCTestRepo) UpdateSubscription(ctx context.Context, sub *model.PushSubscription) (*model.PushSubscription, error) {
	ex, ok := r.subsByID[sub.ID]
	if !ok {
		return nil, nil
	}
	if sub.DeviceName != "" {
		ex.DeviceName = sub.DeviceName
	}
	ex.IsActive = sub.IsActive
	cp := *ex
	return &cp, nil
}
func (r *memGRPCTestRepo) DeactivateSubscription(ctx context.Context, id, userID, endpoint string) (bool, error) {
	var s *model.PushSubscription
	if id != "" {
		s = r.subsByID[id]
	} else if subID, ok := r.subsByEnd[endpoint]; ok {
		s = r.subsByID[subID]
	}
	if s == nil || (userID != "" && s.UserID != userID) {
		return false, nil
	}
	s.IsActive = false
	now := time.Now().UTC()
	s.DeactivatedAt = &now
	return true, nil
}
func (r *memGRPCTestRepo) MarkSubscriptionGone(ctx context.Context, id string, errMsg string) error {
	return nil
}
func (r *memGRPCTestRepo) RecordSubscriptionFailure(ctx context.Context, id string, errMsg string) error {
	return nil
}
func (r *memGRPCTestRepo) MarkSubscriptionUsed(ctx context.Context, id string) error {
	now := time.Now().UTC()
	if s, ok := r.subsByID[id]; ok {
		s.LastUsedAt = &now
	}
	return nil
}

func TestNotificationGRPCHandler_AllRPCsAndAuthorization(t *testing.T) {
	pubVAPID, privVAPID, err := push.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys failed: %v", err)
	}

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			rec.WriteHeader(http.StatusCreated)
			return rec.Result(), nil
		}),
	}

	repo := newMemGRPCTestRepo()
	logger := slog.New(slog.DiscardHandler)
	tmplSvc := service.NewTemplateService()
	pushClient := push.NewWebPushClient(
		config.VAPIDConfig{PublicKey: pubVAPID, PrivateKey: privVAPID, Subject: "mailto:push@gocart.local"},
		config.PushConfig{RequestTimeout: time.Second, MaxRetries: 1, RetryInterval: 5 * time.Millisecond},
		httpClient,
		logger,
	)
	subSvc := service.NewPushSubscriptionService(repo, config.VAPIDConfig{PublicKey: pubVAPID, PrivateKey: privVAPID, Subject: "mailto:push@gocart.local"}, logger)
	prefSvc := service.NewPreferenceService(repo, repo)
	notifSvc := service.NewNotificationService(repo, repo, repo, repo, tmplSvc, pushClient, logger)
	grpcHandler := handler.NewNotificationGRPCHandler(notifSvc, subSvc, prefSvc)

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(grpcclient.UnaryServerInterceptor()))
	notificationpb.RegisterNotificationServiceServer(srv, grpcHandler)
	go func() { _ = srv.Serve(lis) }()
	defer srv.GracefulStop()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient failed: %v", err)
	}
	defer conn.Close()

	client := notificationpb.NewNotificationServiceClient(conn)

	customerID := uuid.NewString()
	otherCustomerID := uuid.NewString()
	adminID := uuid.NewString()

	customerCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-user-id", customerID,
		"x-user-role", "CUSTOMER",
	))
	adminCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-user-id", adminID,
		"x-user-role", "ADMIN",
	))

	// 1. GetVAPIDPublicKey
	vapidResp, err := client.GetVAPIDPublicKey(customerCtx, &notificationpb.GetVAPIDPublicKeyRequest{})
	if err != nil || vapidResp.GetPublicKey() != pubVAPID {
		t.Fatalf("GetVAPIDPublicKey failed: %v (key=%s)", err, vapidResp.GetPublicKey())
	}

	// 2. RegisterPushSubscription
	priv, _ := ecdh.P256().GenerateKey(rand.Reader)
	authSec := make([]byte, 16)
	_, _ = rand.Read(authSec)
	regResp, err := client.RegisterPushSubscription(customerCtx, &notificationpb.RegisterPushSubscriptionRequest{
		UserId:     customerID,
		Endpoint:   "https://fcm.googleapis.com/fcm/send/grpc-test",
		P256Dh:     base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()),
		Auth:       base64.RawURLEncoding.EncodeToString(authSec),
		UserAgent:  "Edge/130",
		DeviceName: "Desktop Edge",
	})
	if err != nil || regResp.GetSubscription().GetId() == "" {
		t.Fatalf("RegisterPushSubscription failed: %v", err)
	}
	subID := regResp.GetSubscription().GetId()

	// 3. GetPushSubscriptions (owner succeeds, cross-user fails with PermissionDenied)
	listSubsResp, err := client.GetPushSubscriptions(customerCtx, &notificationpb.GetPushSubscriptionsRequest{UserId: customerID, ActiveOnly: true})
	if err != nil || listSubsResp.GetTotal() != 1 {
		t.Fatalf("GetPushSubscriptions failed: %v (total=%d)", err, listSubsResp.GetTotal())
	}
	if _, err := client.GetPushSubscriptions(customerCtx, &notificationpb.GetPushSubscriptionsRequest{UserId: otherCustomerID}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied for cross-user GetPushSubscriptions, got %v", err)
	}

	// 4. UpdatePushSubscription
	updatedResp, err := client.UpdatePushSubscription(customerCtx, &notificationpb.UpdatePushSubscriptionRequest{
		Id:         subID,
		UserId:     customerID,
		DeviceName: "Renamed Edge",
	})
	if err != nil || updatedResp.GetSubscription().GetDeviceName() != "Renamed Edge" {
		t.Fatalf("UpdatePushSubscription failed: %v (%+v)", err, updatedResp)
	}

	// 5. Internal SendNotification (no x-user-id metadata) & history management
	sendResp, err := client.SendNotification(context.Background(), &notificationpb.SendNotificationRequest{
		UserId:    customerID,
		Channel:   "PUSH",
		EventType: "OrderConfirmed",
		Subject:   "Order Confirmed",
		Content:   "Your order is confirmed",
	})
	if err != nil || !sendResp.GetSuccess() || sendResp.GetStatus() != "SENT" {
		t.Fatalf("SendNotification failed: %v (%+v)", err, sendResp)
	}

	// 6. GetNotifications (customer + admin on target user)
	notifsResp, err := client.GetNotifications(customerCtx, &notificationpb.GetNotificationsRequest{UserId: customerID, Status: "SENT"})
	if err != nil || notifsResp.GetTotal() != 1 || notifsResp.GetUnreadCount() != 1 {
		t.Fatalf("GetNotifications failed: %v (%+v)", err, notifsResp)
	}
	adminNotifsResp, err := client.GetNotifications(adminCtx, &notificationpb.GetNotificationsRequest{UserId: customerID})
	if err != nil || adminNotifsResp.GetTotal() != 1 {
		t.Fatalf("admin GetNotifications failed: %v", err)
	}

	// 7. GetUnreadCount & MarkNotificationAsRead & MarkAllNotificationsAsRead
	unreadResp, err := client.GetUnreadCount(customerCtx, &notificationpb.GetUnreadCountRequest{UserId: customerID})
	if err != nil || unreadResp.GetUnreadCount() != 1 {
		t.Fatalf("GetUnreadCount failed: %v (%+v)", err, unreadResp)
	}
	isRead := true
	markResp, err := client.MarkNotificationAsRead(customerCtx, &notificationpb.MarkNotificationAsReadRequest{
		NotificationId: sendResp.GetNotificationId(),
		UserId:         customerID,
		IsRead:         &isRead,
	})
	if err != nil || !markResp.GetSuccess() || !markResp.GetNotification().GetIsRead() {
		t.Fatalf("MarkNotificationAsRead failed: %v", err)
	}
	markAllResp, err := client.MarkAllNotificationsAsRead(customerCtx, &notificationpb.MarkAllNotificationsAsReadRequest{UserId: customerID})
	if err != nil || !markAllResp.GetSuccess() || markAllResp.GetUpdatedCount() != 0 {
		t.Fatalf("MarkAllNotificationsAsRead failed: %v", err)
	}

	// 8. DeactivatePushSubscription
	deactResp, err := client.DeactivatePushSubscription(customerCtx, &notificationpb.DeactivatePushSubscriptionRequest{
		Id:     subID,
		UserId: customerID,
	})
	if err != nil || !deactResp.GetSuccess() {
		t.Fatalf("DeactivatePushSubscription failed: %v (%+v)", err, deactResp)
	}
}

