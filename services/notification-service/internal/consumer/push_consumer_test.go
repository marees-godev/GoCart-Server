package consumer_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/push"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type pushMemoryRepo struct {
	mu              sync.RWMutex
	channels        map[string]*model.Channel
	preferences     map[string]*model.Preference
	jobs            map[string]*model.NotificationJob
	jobsByEventID   map[string]*model.NotificationJob
	processedEvents map[string]*model.ProcessedEvent
	notifications   map[string]*model.Notification
	byEventChan     map[string]string
	subsByID        map[string]*model.PushSubscription
	subsByEndpoint  map[string]string
}

func newPushMemoryRepo() *pushMemoryRepo {
	r := &pushMemoryRepo{
		channels:        make(map[string]*model.Channel),
		preferences:     make(map[string]*model.Preference),
		jobs:            make(map[string]*model.NotificationJob),
		jobsByEventID:   make(map[string]*model.NotificationJob),
		processedEvents: make(map[string]*model.ProcessedEvent),
		notifications:   make(map[string]*model.Notification),
		byEventChan:     make(map[string]string),
		subsByID:        make(map[string]*model.PushSubscription),
		subsByEndpoint:  make(map[string]string),
	}

	for _, code := range []string{model.ChannelEmail, model.ChannelPush, model.ChannelSMS, model.ChannelInApp} {
		ch := &model.Channel{ID: uuid.NewString(), Code: code, Name: code, IsActive: true, CreatedAt: time.Now()}
		r.channels[code] = ch
		r.channels[ch.ID] = ch
	}
	return r
}

func (r *pushMemoryRepo) GetByCode(ctx context.Context, code string) (*model.Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.channels[code], nil
}

func (r *pushMemoryRepo) GetByID(ctx context.Context, id string) (*model.Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.channels[id], nil
}

func (r *pushMemoryRepo) GetByEventType(ctx context.Context, eventType string) (*model.Template, error) {
	return nil, nil
}

func (r *pushMemoryRepo) GetByEventTypeAndChannel(ctx context.Context, eventType, channelID string) (*model.Template, error) {
	return nil, nil
}

func (r *pushMemoryRepo) GetPreference(ctx context.Context, userID, channelID, eventType string) (*model.Preference, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.preferences[userID+":"+channelID+":"+eventType], nil
}

func (r *pushMemoryRepo) UpsertPreference(ctx context.Context, pref *model.Preference) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.preferences[pref.UserID+":"+pref.ChannelID+":"+pref.EventType] = pref
	return nil
}

func (r *pushMemoryRepo) CreateJob(ctx context.Context, job *model.NotificationJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.jobsByEventID[job.EventID]; exists {
		return appErrors.Conflict("job already exists")
	}
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	cp := *job
	r.jobs[cp.ID] = &cp
	r.jobsByEventID[cp.EventID] = &cp
	return nil
}

func (r *pushMemoryRepo) GetJobByEventID(ctx context.Context, eventID string) (*model.NotificationJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if j, ok := r.jobsByEventID[eventID]; ok {
		cp := *j
		return &cp, nil
	}
	return nil, nil
}

func (r *pushMemoryRepo) GetJobByID(ctx context.Context, id string) (*model.NotificationJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if j, ok := r.jobs[id]; ok {
		cp := *j
		return &cp, nil
	}
	return nil, nil
}

func (r *pushMemoryRepo) UpdateJobStatus(ctx context.Context, id string, status model.JobStatus, errMsg *string) error {
	return nil
}
func (r *pushMemoryRepo) IncrementRetry(ctx context.Context, id string, errMsg string) error {
	return nil
}
func (r *pushMemoryRepo) ListPendingJobs(ctx context.Context, limit int) ([]*model.NotificationJob, error) {
	return nil, nil
}
func (r *pushMemoryRepo) ClaimPendingJobs(ctx context.Context, limit int) ([]*model.NotificationJob, error) {
	return nil, nil
}

func (r *pushMemoryRepo) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.processedEvents[eventID]
	return ok, nil
}

func (r *pushMemoryRepo) RecordProcessedEvent(ctx context.Context, event *model.ProcessedEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processedEvents[event.EventID] = event
	return nil
}

func (r *pushMemoryRepo) CreateNotification(ctx context.Context, n *model.Notification) error {
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
	n.CreatedAt = time.Now().UTC()
	n.UpdatedAt = n.CreatedAt
	cp := *n
	r.notifications[cp.ID] = &cp
	return nil
}

func (r *pushMemoryRepo) GetNotificationByID(ctx context.Context, id string) (*model.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if n, ok := r.notifications[id]; ok {
		cp := *n
		return &cp, nil
	}
	return nil, nil
}

func (r *pushMemoryRepo) GetNotificationByEventIDAndChannel(ctx context.Context, eventID, channelID string) (*model.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if id, ok := r.byEventChan[eventID+":"+channelID]; ok {
		cp := *r.notifications[id]
		return &cp, nil
	}
	return nil, nil
}

func (r *pushMemoryRepo) ListNotifications(ctx context.Context, filter model.NotificationFilter) ([]*model.Notification, int, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []*model.Notification
	for _, n := range r.notifications {
		if filter.UserID == "" || n.UserID == filter.UserID {
			cp := *n
			res = append(res, &cp)
		}
	}
	return res, len(res), len(res), nil
}

func (r *pushMemoryRepo) UpdateDeliveryStatus(ctx context.Context, id string, status model.DeliveryStatus, errMsg *string, incrementRetry bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n, ok := r.notifications[id]; ok {
		n.Status = status
		n.ErrorMessage = errMsg
		if incrementRetry {
			n.RetryCount++
		}
		if status == model.DeliveryStatusSent {
			now := time.Now().UTC()
			n.SentAt = &now
		}
	}
	return nil
}

func (r *pushMemoryRepo) MarkAsRead(ctx context.Context, id, userID string, isRead bool) (*model.Notification, error) {
	return nil, nil
}
func (r *pushMemoryRepo) MarkAllAsRead(ctx context.Context, userID string) (int, error) {
	return 0, nil
}
func (r *pushMemoryRepo) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	return 0, nil
}

func (r *pushMemoryRepo) UpsertSubscription(ctx context.Context, sub *model.PushSubscription) (*model.PushSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sub.ID == "" {
		sub.ID = uuid.NewString()
	}
	sub.IsActive = true
	cp := *sub
	r.subsByID[cp.ID] = &cp
	r.subsByEndpoint[cp.Endpoint] = cp.ID
	return &cp, nil
}
func (r *pushMemoryRepo) GetSubscriptionByID(ctx context.Context, id string) (*model.PushSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.subsByID[id], nil
}
func (r *pushMemoryRepo) GetSubscriptionByEndpoint(ctx context.Context, endpoint string) (*model.PushSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if id, ok := r.subsByEndpoint[endpoint]; ok {
		return r.subsByID[id], nil
	}
	return nil, nil
}
func (r *pushMemoryRepo) ListSubscriptionsByUserID(ctx context.Context, userID string, activeOnly bool) ([]*model.PushSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []*model.PushSubscription
	for _, s := range r.subsByID {
		if s.UserID == userID && (!activeOnly || s.IsActive) {
			cp := *s
			res = append(res, &cp)
		}
	}
	return res, nil
}
func (r *pushMemoryRepo) UpdateSubscription(ctx context.Context, sub *model.PushSubscription) (*model.PushSubscription, error) {
	return sub, nil
}
func (r *pushMemoryRepo) DeactivateSubscription(ctx context.Context, id, userID, endpoint string) (bool, error) {
	return true, nil
}
func (r *pushMemoryRepo) MarkSubscriptionGone(ctx context.Context, id string, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.subsByID[id]; ok {
		s.IsActive = false
	}
	return nil
}
func (r *pushMemoryRepo) RecordSubscriptionFailure(ctx context.Context, id string, errMsg string) error {
	return nil
}
func (r *pushMemoryRepo) MarkSubscriptionUsed(ctx context.Context, id string) error {
	return nil
}

type mockOrderLookup struct {
	orderUsers map[string]string
	calls      atomic.Int32
}

func (m *mockOrderLookup) LookupOrderUserID(ctx context.Context, orderID string) (string, error) {
	m.calls.Add(1)
	return m.orderUsers[orderID], nil
}

func TestPushNotificationConsumer_AllOrderPaymentDeliveryMerchantEvents(t *testing.T) {
	ctx := context.Background()
	pubVAPID, privVAPID, err := push.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys failed: %v", err)
	}

	uaPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ECDH keygen failed: %v", err)
	}
	uaAuthBytes := make([]byte, 16)
	_, _ = rand.Read(uaAuthBytes)
	uaPubB64 := base64.RawURLEncoding.EncodeToString(uaPriv.PublicKey().Bytes())
	uaAuthB64 := base64.RawURLEncoding.EncodeToString(uaAuthBytes)

	var (
		mu             sync.Mutex
		receivedPushes []dto.PushPayload
	)
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			rawBody, _ := io.ReadAll(r.Body)
			plain, decErr := push.DecryptPayload(rawBody, uaPriv, uaAuthB64)
			if decErr != nil {
				rec.WriteHeader(http.StatusBadRequest)
				return rec.Result(), nil
			}
			var p dto.PushPayload
			_ = json.Unmarshal(plain, &p)
			mu.Lock()
			receivedPushes = append(receivedPushes, p)
			mu.Unlock()
			rec.WriteHeader(http.StatusCreated)
			return rec.Result(), nil
		}),
	}

	repo := newPushMemoryRepo()
	logger := slog.New(slog.DiscardHandler)
	tmplSvc := service.NewTemplateService()
	prefSvc := service.NewPreferenceService(repo, repo)
	pushClient := push.NewWebPushClient(
		config.VAPIDConfig{PublicKey: pubVAPID, PrivateKey: privVAPID, Subject: "mailto:push@gocart.local"},
		config.PushConfig{RequestTimeout: time.Second, MaxRetries: 1, RetryInterval: 5 * time.Millisecond, TTLSeconds: 3600},
		httpClient,
		logger,
	)
	notifSvc := service.NewNotificationService(repo, repo, repo, repo, tmplSvc, pushClient, logger)

	userWithSub := uuid.NewString()
	userWithoutSub := uuid.NewString()
	_, _ = repo.UpsertSubscription(ctx, &model.PushSubscription{
		UserID:   userWithSub,
		Endpoint: "https://fcm.googleapis.com/fcm/send/consumer-sub",
		P256DH:   uaPubB64,
		Auth:     uaAuthB64,
	})

	orderLookup := &mockOrderLookup{
		orderUsers: map[string]string{
			"ord-lookup-1": userWithSub,
		},
	}

	pipeline := service.NewPushNotificationPipelineService(tmplSvc, prefSvc, repo, repo, notifSvc, orderLookup, logger)
	evtConsumer := consumer.NewNotificationEventConsumer(pipeline, logger)

	testEvents := []struct {
		eventType      string
		payload        any
		expectPushSent bool
	}{
		{
			// OrderCreated (Order Placed) triggers push notification
			eventType: events.EventTypeOrderCreated,
			payload: events.OrderCreatedEvent{
				OrderID:     "ord-10",
				UserID:      userWithSub,
				TotalAmount: 99.99,
				Currency:    "USD",
				Status:      "PENDING",
			},
			expectPushSent: true,
		},
		{
			// OrderConfirmed triggers push notification
			eventType: events.EventTypeOrderConfirmed,
			payload: events.OrderConfirmedEvent{
				OrderID:       "ord-10",
				UserID:        userWithSub,
				PaymentStatus: "PAID",
				ConfirmedAt:   time.Now(),
			},
			expectPushSent: true,
		},
		{
			// OrderShipped triggers push notification
			eventType: events.EventTypeOrderShipped,
			payload: events.OrderShippedEvent{
				OrderID:         "ord-10",
				UserID:          userWithSub,
				DeliveryPartner: "FedEx",
				TrackingNumber:  "FDX-10",
			},
			expectPushSent: true,
		},
		{
			// OrderDelivered triggers push notification
			eventType: events.EventTypeOrderDelivered,
			payload: events.OrderDeliveredEvent{
				OrderID: "ord-10",
				UserID:  userWithSub,
			},
			expectPushSent: true,
		},
		{
			// OrderCancelled triggers push notification
			eventType: events.EventTypeOrderCancelled,
			payload: events.OrderCancelledEvent{
				OrderID:     "ord-10",
				UserID:      userWithSub,
				Reason:      "Customer requested",
				CancelledAt: time.Now(),
			},
			expectPushSent: true,
		},
		{
			// Order placed for user without push subscription
			eventType: events.EventTypeOrderCreated,
			payload: events.OrderCreatedEvent{
				OrderID:     "ord-11",
				UserID:      userWithoutSub,
				TotalAmount: 49.99,
				Currency:    "USD",
			},
			expectPushSent: false,
		},
		{
			eventType: events.EventTypePaymentSuccessful,
			payload: events.PaymentSuccessfulEvent{
				PaymentID: "pay-1",
				OrderID:   "ord-10",
				UserID:    userWithSub,
				Amount:    149.99,
				Currency:  "USD",
			},
			expectPushSent: false,
		},
		{
			eventType: events.EventTypePaymentFailed,
			payload: events.PaymentFailedEvent{
				PaymentID: "pay-2",
				OrderID:   "ord-10",
				UserID:    userWithSub,
				Amount:    149.99,
				Reason:    "Card declined",
			},
			expectPushSent: false,
		},
		{
			// DeliveryCreated with empty UserID exercises OrderUserLookup fallback and triggers push notification
			eventType: events.EventTypeDeliveryCreated,
			payload: events.DeliveryCreatedEvent{
				DeliveryID:      "del-1",
				OrderID:         "ord-lookup-1",
				DeliveryPartner: "Delhivery",
				TrackingNumber:  "DLV-999",
				Status:          "CREATED",
			},
			expectPushSent: true,
		},
		{
			// DeliveryStatusUpdated triggers push notification
			eventType: events.EventTypeDeliveryStatusUpdated,
			payload: events.DeliveryStatusUpdatedEvent{
				DeliveryID:     "del-1",
				OrderID:        "ord-lookup-1",
				PreviousStatus: "CREATED",
				NewStatus:      "OUT_FOR_DELIVERY",
				TrackingNumber: "DLV-999",
			},
			expectPushSent: true,
		},
		{
			eventType: events.EventTypeMerchantApproved,
			payload: events.MerchantApprovedEvent{
				MerchantID: userWithSub,
				UserID:     userWithSub,
				NewStatus:  "APPROVED",
			},
			expectPushSent: false,
		},
		{
			eventType: events.EventTypeMerchantRejected,
			payload: events.MerchantRejectedEvent{
				MerchantID: userWithSub,
				UserID:     userWithSub,
				NewStatus:  "REJECTED",
				Reason:     "Missing GST certificate",
			},
			expectPushSent: false,
		},
	}

	expectedPushCount := 0
	for _, tc := range testEvents {
		env, envErr := events.NewEventEnvelope(tc.eventType, "test-producer", tc.payload)
		if envErr != nil {
			t.Fatalf("NewEventEnvelope(%s) failed: %v", tc.eventType, envErr)
		}
		env.CorrelationID = "corr-" + tc.eventType
		if err := evtConsumer.HandleEvent(ctx, env); err != nil {
			t.Fatalf("HandleEvent(%s) failed: %v", tc.eventType, err)
		}
		if tc.expectPushSent {
			expectedPushCount++
		}
	}

	if len(receivedPushes) != expectedPushCount {
		t.Fatalf("expected %d delivered Web Push messages, got %d", expectedPushCount, len(receivedPushes))
	}
	if orderLookup.calls.Load() != 2 {
		t.Errorf("expected OrderUserLookup called 2 times for delivery events without UserID, got %d", orderLookup.calls.Load())
	}

	// Notification history for userWithoutSub was persisted cleanly with PENDING status despite no push subscription
	noSubHistory, _, _, _ := repo.ListNotifications(ctx, model.NotificationFilter{UserID: userWithoutSub})
	if len(noSubHistory) != 1 || noSubHistory[0].Status != model.DeliveryStatusPending || noSubHistory[0].EventType != events.EventTypeOrderCreated {
		t.Fatalf("expected 1 PENDING history record for user without push subscription, got %+v", noSubHistory)
	}

	// Duplicate OrderCreated event is idempotent and does not send another push
	dupEnv, _ := events.NewEventEnvelope(events.EventTypeOrderCreated, "order-service", events.OrderCreatedEvent{
		OrderID: "ord-dup", UserID: userWithSub, TotalAmount: 99.99, Currency: "USD",
	})
	_ = evtConsumer.HandleEvent(ctx, dupEnv)
	countAfterFirstDup := len(receivedPushes)
	_ = evtConsumer.HandleEvent(ctx, dupEnv)
	if len(receivedPushes) != countAfterFirstDup {
		t.Errorf("expected duplicate event NOT to send second Web Push (before=%d after=%d)", countAfterFirstDup, len(receivedPushes))
	}
}

func TestPushNotificationConsumer_MalformedPayloadRoutesToDLQ(t *testing.T) {
	repo := newPushMemoryRepo()
	logger := slog.New(slog.DiscardHandler)
	tmplSvc := service.NewTemplateService()
	prefSvc := service.NewPreferenceService(repo, repo)
	notifSvc := service.NewNotificationService(repo, repo, repo, repo, tmplSvc, nil, logger)
	pipeline := service.NewPushNotificationPipelineService(tmplSvc, prefSvc, repo, repo, notifSvc, nil, logger)
	evtConsumer := consumer.NewNotificationEventConsumer(pipeline, logger)

	var dlqCalls int
	var dlqEnvelope *events.EventEnvelope
	var dlqReason error
	evtConsumer.SetDLQPublisher(func(ctx context.Context, envelope *events.EventEnvelope, reason error) error {
		dlqCalls++
		dlqEnvelope = envelope
		dlqReason = reason
		return nil
	})

	badEnv := &events.EventEnvelope{
		EventID:   uuid.NewString(),
		EventType: events.EventTypeMerchantApproved,
		Data:      []byte(`{invalid-json`),
	}
	err := evtConsumer.HandleEvent(context.Background(), badEnv)
	if err != nil {
		t.Fatalf("expected malformed event routed to DLQ without retry error, got: %v", err)
	}
	if dlqCalls != 1 || dlqEnvelope == nil || dlqEnvelope.EventID != badEnv.EventID {
		t.Fatalf("expected DLQPublisher called once with bad envelope, got calls=%d", dlqCalls)
	}
	if dlqReason == nil || !strings.Contains(dlqReason.Error(), "malformed") {
		t.Errorf("expected malformed reason passed to DLQPublisher, got: %v", dlqReason)
	}
}

