package service_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/push"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type memHistoryRepo struct {
	mu            sync.RWMutex
	notifications map[string]*model.Notification
	byEventChan   map[string]string
	seq           int
}

func newMemHistoryRepo() *memHistoryRepo {
	return &memHistoryRepo{
		notifications: make(map[string]*model.Notification),
		byEventChan:   make(map[string]string),
	}
}

func cloneNotification(n *model.Notification) *model.Notification {
	if n == nil {
		return nil
	}
	cp := *n
	if n.EventID != nil {
		v := *n.EventID
		cp.EventID = &v
	}
	if n.ErrorMessage != nil {
		v := *n.ErrorMessage
		cp.ErrorMessage = &v
	}
	if n.ReadAt != nil {
		v := *n.ReadAt
		cp.ReadAt = &v
	}
	if n.SentAt != nil {
		v := *n.SentAt
		cp.SentAt = &v
	}
	return &cp
}

func (r *memHistoryRepo) CreateNotification(ctx context.Context, n *model.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if n.EventID != nil && *n.EventID != "" {
		k := *n.EventID + ":" + n.ChannelID
		if _, exists := r.byEventChan[k]; exists {
			return appErrors.Conflict("notification already exists")
		}
		r.byEventChan[k] = n.ID
	}

	if n.ID == "" {
		n.ID = uuid.NewString()
	}
	r.seq++
	now := time.Now().UTC().Add(time.Duration(r.seq) * time.Millisecond)
	if n.CreatedAt.IsZero() {
		n.CreatedAt = now
	}
	n.UpdatedAt = n.CreatedAt
	r.notifications[n.ID] = cloneNotification(n)
	return nil
}

func (r *memHistoryRepo) GetNotificationByID(ctx context.Context, id string) (*model.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneNotification(r.notifications[id]), nil
}

func (r *memHistoryRepo) GetNotificationByEventIDAndChannel(ctx context.Context, eventID, channelID string) (*model.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byEventChan[eventID+":"+channelID]
	if !ok {
		return nil, nil
	}
	return cloneNotification(r.notifications[id]), nil
}

func (r *memHistoryRepo) ListNotifications(ctx context.Context, filter model.NotificationFilter) ([]*model.Notification, int, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matching []*model.Notification
	unreadCount := 0
	for _, n := range r.notifications {
		if filter.UserID != "" && n.UserID != filter.UserID {
			continue
		}
		if !n.IsRead {
			unreadCount++
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
		matching = append(matching, cloneNotification(n))
	}

	sort.Slice(matching, func(i, j int) bool {
		return matching[i].CreatedAt.After(matching[j].CreatedAt)
	})

	total := len(matching)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []*model.Notification{}, total, unreadCount, nil
	}
	end := offset + filter.Limit
	if filter.Limit <= 0 || end > total {
		end = total
	}
	return matching[offset:end], total, unreadCount, nil
}

func (r *memHistoryRepo) UpdateDeliveryStatus(ctx context.Context, id string, status model.DeliveryStatus, errMsg *string, incrementRetry bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n, ok := r.notifications[id]; ok {
		n.Status = status
		if errMsg != nil {
			v := *errMsg
			n.ErrorMessage = &v
		} else {
			n.ErrorMessage = nil
		}
		if incrementRetry {
			n.RetryCount++
		}
		now := time.Now().UTC()
		if status == model.DeliveryStatusSent && n.SentAt == nil {
			n.SentAt = &now
		}
		n.UpdatedAt = now
	}
	return nil
}

func (r *memHistoryRepo) MarkAsRead(ctx context.Context, id, userID string, isRead bool) (*model.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.notifications[id]
	if !ok || (userID != "" && n.UserID != userID) {
		return nil, nil
	}
	now := time.Now().UTC()
	n.IsRead = isRead
	if isRead {
		if n.ReadAt == nil {
			n.ReadAt = &now
		}
	} else {
		n.ReadAt = nil
	}
	n.UpdatedAt = now
	return cloneNotification(n), nil
}

func (r *memHistoryRepo) MarkAllAsRead(ctx context.Context, userID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	updated := 0
	for _, n := range r.notifications {
		if n.UserID == userID && !n.IsRead {
			n.IsRead = true
			n.ReadAt = &now
			n.UpdatedAt = now
			updated++
		}
	}
	return updated, nil
}

func (r *memHistoryRepo) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cnt := 0
	for _, n := range r.notifications {
		if n.UserID == userID && !n.IsRead {
			cnt++
		}
	}
	return cnt, nil
}

func setupNotificationServiceFixture(t *testing.T, handler http.HandlerFunc) (
	service.NotificationService,
	*mockChannelRepo,
	*mockPreferenceRepo,
	*memHistoryRepo,
	*memPushSubRepo,
) {
	t.Helper()

	pubVAPID, privVAPID, err := push.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys failed: %v", err)
	}

	pushChan := &model.Channel{ID: uuid.NewString(), Code: model.ChannelPush, Name: "Web Push", IsActive: true}
	emailChan := &model.Channel{ID: uuid.NewString(), Code: model.ChannelEmail, Name: "Email", IsActive: true}
	inAppChan := &model.Channel{ID: uuid.NewString(), Code: model.ChannelInApp, Name: "In-App", IsActive: true}

	chanRepo := &mockChannelRepo{
		channels: map[string]*model.Channel{
			model.ChannelPush:  pushChan,
			pushChan.ID:        pushChan,
			model.ChannelEmail: emailChan,
			emailChan.ID:       emailChan,
			model.ChannelInApp: inAppChan,
			inAppChan.ID:       inAppChan,
		},
	}
	prefRepo := &mockPreferenceRepo{preferences: make(map[string]*model.Preference)}
	historyRepo := newMemHistoryRepo()
	subRepo := newMemPushSubRepo()
	tmplSvc := service.NewTemplateService()

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			handler(rec, r)
			return rec.Result(), nil
		}),
	}

	logger := slog.New(slog.DiscardHandler)
	webPushClient := push.NewWebPushClient(
		config.VAPIDConfig{
			PublicKey:  pubVAPID,
			PrivateKey: privVAPID,
			Subject:    "mailto:alerts@gocart.local",
		},
		config.PushConfig{
			RequestTimeout: 2 * time.Second,
			MaxRetries:     3,
			RetryInterval:  5 * time.Millisecond,
			TTLSeconds:     3600,
		},
		httpClient,
		logger,
	)

	notifSvc := service.NewNotificationService(chanRepo, prefRepo, historyRepo, subRepo, tmplSvc, webPushClient, logger)
	return notifSvc, chanRepo, prefRepo, historyRepo, subRepo
}

func TestNotificationService_SendPushNoSubscriptionsGraceful(t *testing.T) {
	var httpCalled atomic.Int32
	notifSvc, _, _, historyRepo, _ := setupNotificationServiceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		httpCalled.Add(1)
		w.WriteHeader(http.StatusCreated)
	})

	userID := uuid.NewString()
	res, err := notifSvc.SendNotification(context.Background(), dto.SendNotificationInput{
		UserID:        userID,
		Channel:       model.ChannelPush,
		EventType:     "OrderConfirmed",
		EventID:       "evt-no-sub-1",
		Subject:       "Order Confirmed",
		Content:       "Your order #ORD-10 is confirmed.",
		CorrelationID: "corr-no-sub-1",
	})
	if err != nil {
		t.Fatalf("expected graceful handling without error, got: %v", err)
	}
	if !res.Success || res.Notification == nil {
		t.Fatalf("expected success and saved notification, got: %+v", res)
	}
	if res.Notification.Status != model.DeliveryStatusPending {
		t.Errorf("expected PENDING status when skipping push without subscriptions, got %s", res.Notification.Status)
	}
	if httpCalled.Load() != 0 {
		t.Errorf("expected zero HTTP push calls when user has no subscriptions, got %d", httpCalled.Load())
	}

	saved, _ := historyRepo.GetNotificationByID(context.Background(), res.Notification.ID)
	if saved == nil || saved.Title != "Order Confirmed" {
		t.Fatalf("expected notification persisted in history, got: %+v", saved)
	}
}

func TestNotificationService_MultiDeviceDeliveryAndExpiredCleanup(t *testing.T) {
	ctx := context.Background()
	privChrome, p256Chrome, authChrome := generateTestSubKeys(t)
	_, p256Firefox, authFirefox := generateTestSubKeys(t)

	var decryptedPayload []byte
	var gotVAPIDHeader string

	notifSvc, _, _, _, subRepo := setupNotificationServiceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		if strings.Contains(r.URL.Path, "expired-firefox") {
			w.WriteHeader(http.StatusGone)
			_, _ = w.Write([]byte("subscription expired"))
			return
		}
		gotVAPIDHeader = r.Header.Get("Authorization")
		plain, err := push.DecryptPayload(bodyBytes, privChrome, authChrome)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		decryptedPayload = plain
		w.WriteHeader(http.StatusCreated)
	})

	userID := uuid.NewString()
	activeSub, _ := subRepo.UpsertSubscription(ctx, &model.PushSubscription{
		UserID:     userID,
		Endpoint:   "https://fcm.googleapis.com/fcm/send/active-chrome",
		P256DH:     p256Chrome,
		Auth:       authChrome,
		DeviceName: "Chrome",
	})
	goneSub, _ := subRepo.UpsertSubscription(ctx, &model.PushSubscription{
		UserID:     userID,
		Endpoint:   "https://push.mozilla.org/expired-firefox",
		P256DH:     p256Firefox,
		Auth:       authFirefox,
		DeviceName: "Firefox",
	})

	res, err := notifSvc.SendNotification(ctx, dto.SendNotificationInput{
		UserID:        userID,
		Channel:       model.ChannelPush,
		EventType:     "DeliveryCreated",
		EventID:       "evt-multi-1",
		CorrelationID: "corr-delivery-1",
		TemplateData: map[string]string{
			"OrderID":         "ORD-77",
			"DeliveryPartner": "BlueDart",
			"TrackingNumber":  "TRK-77",
			"order_id":        "ORD-77",
		},
	})
	if err != nil {
		t.Fatalf("SendNotification failed: %v", err)
	}
	if !res.Success || res.Notification.Status != model.DeliveryStatusSent {
		t.Fatalf("expected SENT status after partial success, got success=%v status=%s msg=%s", res.Success, res.Notification.Status, res.Message)
	}
	if !strings.HasPrefix(gotVAPIDHeader, "vapid t=") {
		t.Errorf("expected signed VAPID header, got %q", gotVAPIDHeader)
	}

	var decodedPush dto.PushPayload
	if err := json.Unmarshal(decryptedPayload, &decodedPush); err != nil {
		t.Fatalf("failed decoding decrypted push payload: %v", err)
	}
	if decodedPush.EventType != "DeliveryCreated" || decodedPush.Data["order_id"] != "ORD-77" || decodedPush.CorrelationID != "corr-delivery-1" {
		t.Errorf("unexpected decrypted push payload: %+v", decodedPush)
	}
	if decodedPush.Title != "Delivery Created - Order #ORD-77" || !strings.Contains(decodedPush.Body, "ORD-77") {
		t.Errorf("expected rendered push template title 'Delivery Created - Order #ORD-77' and body with ORD-77, got title=%q body=%q", decodedPush.Title, decodedPush.Body)
	}

	updatedGone, _ := subRepo.GetSubscriptionByID(ctx, goneSub.ID)
	if updatedGone.IsActive || updatedGone.DeactivatedAt == nil {
		t.Errorf("expected 410 Gone subscription to be auto-deactivated, got %+v", updatedGone)
	}
	updatedActive, _ := subRepo.GetSubscriptionByID(ctx, activeSub.ID)
	if !updatedActive.IsActive || updatedActive.LastUsedAt == nil {
		t.Errorf("expected active subscription last_used_at recorded, got %+v", updatedActive)
	}
}

func TestNotificationService_AllGoneRetriesIdempotencyAndOptOut(t *testing.T) {
	ctx := context.Background()

	t.Run("all subscriptions 404/410 marks notification FAILED and deactivates all", func(t *testing.T) {
		notifSvc, _, _, _, subRepo := setupNotificationServiceFixture(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		userID := uuid.NewString()
		_, p256, auth := generateTestSubKeys(t)
		sub, _ := subRepo.UpsertSubscription(ctx, &model.PushSubscription{
			UserID:   userID,
			Endpoint: "https://push.example.com/404",
			P256DH:   p256,
			Auth:     auth,
		})

		res, err := notifSvc.SendNotification(ctx, dto.SendNotificationInput{
			UserID:  userID,
			Channel: model.ChannelPush,
			Subject: "Payment Failed",
			Content: "Your payment failed",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Success || res.Notification.Status != model.DeliveryStatusFailed {
			t.Fatalf("expected FAILED status, got %+v", res.Notification)
		}
		after, _ := subRepo.GetSubscriptionByID(ctx, sub.ID)
		if after.IsActive {
			t.Errorf("expected 404 subscription deactivated")
		}
	})

	t.Run("transient 503 retries and recovers", func(t *testing.T) {
		var attempts atomic.Int32
		notifSvc, _, _, _, subRepo := setupNotificationServiceFixture(t, func(w http.ResponseWriter, r *http.Request) {
			if attempts.Add(1) < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusCreated)
		})
		userID := uuid.NewString()
		_, p256, auth := generateTestSubKeys(t)
		_, _ = subRepo.UpsertSubscription(ctx, &model.PushSubscription{
			UserID:   userID,
			Endpoint: "https://push.example.com/retry-ok",
			P256DH:   p256,
			Auth:     auth,
		})

		res, err := notifSvc.SendNotification(ctx, dto.SendNotificationInput{
			UserID:  userID,
			Channel: model.ChannelPush,
			Subject: "Retry Recovery",
			Content: "Should deliver on 3rd try",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Success || res.Notification.Status != model.DeliveryStatusSent {
			t.Fatalf("expected SENT after retries, got %+v", res)
		}
		if attempts.Load() != 3 {
			t.Errorf("expected 3 HTTP attempts, got %d", attempts.Load())
		}
	})

	t.Run("persistent 503 increments retry count and marks FAILED", func(t *testing.T) {
		notifSvc, _, _, historyRepo, subRepo := setupNotificationServiceFixture(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})
		userID := uuid.NewString()
		_, p256, auth := generateTestSubKeys(t)
		_, _ = subRepo.UpsertSubscription(ctx, &model.PushSubscription{
			UserID:   userID,
			Endpoint: "https://push.example.com/503-always",
			P256DH:   p256,
			Auth:     auth,
		})

		res, err := notifSvc.SendNotification(ctx, dto.SendNotificationInput{
			UserID:  userID,
			Channel: model.ChannelPush,
			Subject: "Persistent Fail",
			Content: "Always 503",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Success || res.Notification.Status != model.DeliveryStatusFailed || res.Notification.RetryCount != 1 {
			t.Fatalf("expected FAILED with RetryCount=1, got %+v", res.Notification)
		}
		saved, _ := historyRepo.GetNotificationByID(ctx, res.Notification.ID)
		if saved.RetryCount != 1 || saved.Status != model.DeliveryStatusFailed {
			t.Fatalf("expected persisted retry_count=1 and FAILED, got %+v", saved)
		}
	})

	t.Run("event_id idempotency prevents duplicate push and user opt-out skips push", func(t *testing.T) {
		var calls atomic.Int32
		notifSvc, chanRepo, prefRepo, _, subRepo := setupNotificationServiceFixture(t, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusCreated)
		})
		userID := uuid.NewString()
		_, p256, auth := generateTestSubKeys(t)
		_, _ = subRepo.UpsertSubscription(ctx, &model.PushSubscription{
			UserID:   userID,
			Endpoint: "https://push.example.com/idem",
			P256DH:   p256,
			Auth:     auth,
		})

		first, err := notifSvc.SendNotification(ctx, dto.SendNotificationInput{
			UserID:    userID,
			Channel:   model.ChannelPush,
			EventType: "PaymentSuccessful",
			EventID:   "evt-dup-push",
			Subject:   "Paid",
			Content:   "Payment received",
		})
		if err != nil || !first.Success {
			t.Fatalf("first send failed: %v", err)
		}
		second, err := notifSvc.SendNotification(ctx, dto.SendNotificationInput{
			UserID:    userID,
			Channel:   model.ChannelPush,
			EventType: "PaymentSuccessful",
			EventID:   "evt-dup-push",
			Subject:   "Paid",
			Content:   "Payment received",
		})
		if err != nil || second.Notification.ID != first.Notification.ID {
			t.Fatalf("expected idempotent same notification ID, got %+v (err=%v)", second, err)
		}
		if calls.Load() != 1 {
			t.Errorf("expected exactly 1 push HTTP call across duplicate EventID, got %d", calls.Load())
		}

		// Opt out user for OrderCancelled on PUSH
		pushChan, _ := chanRepo.GetByCode(ctx, model.ChannelPush)
		_ = prefRepo.UpsertPreference(ctx, &model.Preference{
			UserID:    userID,
			ChannelID: pushChan.ID,
			EventType: "OrderCancelled",
			IsEnabled: false,
		})
		optedOutRes, err := notifSvc.SendNotification(ctx, dto.SendNotificationInput{
			UserID:    userID,
			Channel:   model.ChannelPush,
			EventType: "OrderCancelled",
			EventID:   "evt-optout-push",
			Subject:   "Order Cancelled",
			Content:   "Order #9 cancelled",
		})
		if err != nil || !optedOutRes.Success || optedOutRes.Notification.Status != model.DeliveryStatusPending {
			t.Fatalf("expected opted-out push to save history cleanly without sending, got %+v (err=%v)", optedOutRes, err)
		}
		if calls.Load() != 1 {
			t.Errorf("expected still 1 push HTTP call after opt-out, got %d", calls.Load())
		}
	})
}

func TestNotificationService_HistoryPaginationFilteringAndReadManagement(t *testing.T) {
	ctx := context.Background()
	notifSvc, chanRepo, _, historyRepo, _ := setupNotificationServiceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	userID := uuid.NewString()
	otherID := uuid.NewString()
	pushCh, _ := chanRepo.GetByCode(ctx, model.ChannelPush)
	emailCh, _ := chanRepo.GetByCode(ctx, model.ChannelEmail)

	seed := []*model.Notification{
		{ID: uuid.NewString(), UserID: userID, ChannelID: pushCh.ID, ChannelCode: "PUSH", EventType: "OrderConfirmed", Recipient: userID, Title: "N1", Content: "C1", Status: model.DeliveryStatusSent, IsRead: false},
		{ID: uuid.NewString(), UserID: userID, ChannelID: pushCh.ID, ChannelCode: "PUSH", EventType: "OrderShipped", Recipient: userID, Title: "N2", Content: "C2", Status: model.DeliveryStatusSent, IsRead: true},
		{ID: uuid.NewString(), UserID: userID, ChannelID: pushCh.ID, ChannelCode: "PUSH", EventType: "OrderShipped", Recipient: userID, Title: "N3", Content: "C3", Status: model.DeliveryStatusFailed, IsRead: false},
		{ID: uuid.NewString(), UserID: userID, ChannelID: emailCh.ID, ChannelCode: "EMAIL", EventType: "OrderDelivered", Recipient: userID, Title: "N4", Content: "C4", Status: model.DeliveryStatusPending, IsRead: false},
		{ID: uuid.NewString(), UserID: otherID, ChannelID: pushCh.ID, ChannelCode: "PUSH", EventType: "OrderConfirmed", Recipient: otherID, Title: "Other", Content: "Other", Status: model.DeliveryStatusSent, IsRead: false},
	}
	for _, n := range seed {
		if err := historyRepo.CreateNotification(ctx, n); err != nil {
			t.Fatalf("seed CreateNotification failed: %v", err)
		}
	}

	// Pagination check
	page1, err := notifSvc.GetNotifications(ctx, model.NotificationFilter{UserID: userID, Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("GetNotifications page1 failed: %v", err)
	}
	if page1.Total != 4 || page1.UnreadCount != 3 || len(page1.Notifications) != 2 {
		t.Fatalf("unexpected page1 result: total=%d unread=%d len=%d", page1.Total, page1.UnreadCount, len(page1.Notifications))
	}
	if page1.Notifications[0].Title != "N4" || page1.Notifications[1].Title != "N3" {
		t.Errorf("unexpected DESC ordering on page1: %s, %s", page1.Notifications[0].Title, page1.Notifications[1].Title)
	}

	// Filter by status, channel, is_read, event_type
	isReadFalse := false
	filtered, err := notifSvc.GetNotifications(ctx, model.NotificationFilter{
		UserID:    userID,
		Status:    "failed",
		Channel:   "push",
		EventType: "OrderShipped",
		IsRead:    &isReadFalse,
	})
	if err != nil || filtered.Total != 1 || filtered.Notifications[0].Title != "N3" {
		t.Fatalf("unexpected filtered result: %+v (err=%v)", filtered, err)
	}

	// Invalid status filter returns BadRequest
	if _, err := notifSvc.GetNotifications(ctx, model.NotificationFilter{UserID: userID, Status: "UNKNOWN"}); appErrors.AsAppError(err).Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST for invalid status filter, got %v", err)
	}

	// MarkAsRead & MarkAsUnread
	marked, err := notifSvc.MarkAsRead(ctx, seed[0].ID, userID, true)
	if err != nil || !marked.IsRead || marked.ReadAt == nil {
		t.Fatalf("expected N1 marked read with ReadAt, got %+v (err=%v)", marked, err)
	}
	unreadCnt, err := notifSvc.GetUnreadCount(ctx, userID)
	if err != nil || unreadCnt != 2 {
		t.Fatalf("expected unread count 2, got %d (err=%v)", unreadCnt, err)
	}

	// Cross-user MarkAsRead -> Forbidden
	if _, err := notifSvc.MarkAsRead(ctx, seed[0].ID, otherID, true); appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN on cross-user MarkAsRead, got %v", err)
	}

	// Missing notification -> NotFound
	if _, err := notifSvc.MarkAsRead(ctx, uuid.NewString(), userID, true); appErrors.AsAppError(err).Code != appErrors.CodeNotFound {
		t.Errorf("expected NOT_FOUND on missing MarkAsRead, got %v", err)
	}

	// MarkAllAsRead
	updatedAll, err := notifSvc.MarkAllAsRead(ctx, userID)
	if err != nil || updatedAll != 2 {
		t.Fatalf("expected 2 notifications marked as read, got %d (err=%v)", updatedAll, err)
	}
	unreadAfter, _ := notifSvc.GetUnreadCount(ctx, userID)
	if unreadAfter != 0 {
		t.Errorf("expected 0 unread after MarkAllAsRead, got %d", unreadAfter)
	}
	// Other user's unread count untouched
	otherUnread, _ := notifSvc.GetUnreadCount(ctx, otherID)
	if otherUnread != 1 {
		t.Errorf("expected other user's unread count 1, got %d", otherUnread)
	}
}
