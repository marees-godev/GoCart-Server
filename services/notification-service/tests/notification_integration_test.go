package tests_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/kafka"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/push"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func genTestSubKeys(t *testing.T) (string, string) {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating ECDH key: %v", err)
	}
	authBytes := make([]byte, 16)
	_, _ = rand.Read(authBytes)
	return base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()),
		base64.RawURLEncoding.EncodeToString(authBytes)
}

func TestPostgresNotificationAndPushIntegration(t *testing.T) {
	dbURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dbURL == "" {
		dbURL = strings.TrimSpace(os.Getenv("NOTIFICATION_TEST_DATABASE_URL"))
	}
	if dbURL == "" {
		t.Skip("Skipping PostgreSQL integration test: TEST_DATABASE_URL / NOTIFICATION_TEST_DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("Skipping PostgreSQL integration test: unable to connect: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Skipping PostgreSQL integration test: ping failed: %v", err)
	}

	migrator, mErr := database.NewMigrator(pool, "../migrations")
	if mErr != nil {
		t.Fatalf("NewMigrator failed: %v", mErr)
	}
	if runErr := migrator.Run(ctx); runErr != nil {
		t.Fatalf("migration execution failed: %v", runErr)
	}

	repo := repository.NewNotificationRepository(pool)
	userID := uuid.NewString()
	endpoint1 := "https://push.example.com/int/" + uuid.NewString()
	endpoint2 := "https://push.example.com/int/" + uuid.NewString()
	eventID := "int-evt-" + uuid.NewString()

	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanCancel()
		_, _ = pool.Exec(cleanCtx, "DELETE FROM notifications WHERE user_id = $1::uuid", userID)
		_, _ = pool.Exec(cleanCtx, "DELETE FROM push_subscriptions WHERE user_id = $1::uuid", userID)
		_, _ = pool.Exec(cleanCtx, "DELETE FROM processed_events WHERE event_id = $1", eventID)
	})

	// 1. Push subscriptions CRUD in PostgreSQL
	p1, a1 := genTestSubKeys(t)
	p2, a2 := genTestSubKeys(t)
	sub1, err := repo.UpsertSubscription(ctx, &model.PushSubscription{
		UserID:     userID,
		Endpoint:   endpoint1,
		P256DH:     p1,
		Auth:       a1,
		DeviceName: "Chrome Int",
	})
	if err != nil {
		t.Fatalf("UpsertSubscription 1 failed: %v", err)
	}
	sub2, err := repo.UpsertSubscription(ctx, &model.PushSubscription{
		UserID:     userID,
		Endpoint:   endpoint2,
		P256DH:     p2,
		Auth:       a2,
		DeviceName: "Firefox Int",
	})
	if err != nil {
		t.Fatalf("UpsertSubscription 2 failed: %v", err)
	}

	listSubs, err := repo.ListSubscriptionsByUserID(ctx, userID, true)
	if err != nil || len(listSubs) != 2 {
		t.Fatalf("expected 2 active subscriptions, got %d (err=%v)", len(listSubs), err)
	}

	if err := repo.MarkSubscriptionGone(ctx, sub2.ID, "410 Gone"); err != nil {
		t.Fatalf("MarkSubscriptionGone failed: %v", err)
	}
	activeAfterGone, err := repo.ListSubscriptionsByUserID(ctx, userID, true)
	if err != nil || len(activeAfterGone) != 1 || activeAfterGone[0].ID != sub1.ID {
		t.Fatalf("expected only sub1 active after 410 Gone, got %+v (err=%v)", activeAfterGone, err)
	}

	// 2. Notification history persistence, ENUM status transitions, pagination, read/unread & idempotency index
	pushCh, err := repo.GetByCode(ctx, model.ChannelPush)
	if err != nil || pushCh == nil {
		t.Fatalf("expected seeded PUSH channel, got %+v (err=%v)", pushCh, err)
	}

	n1 := &model.Notification{
		ID:            uuid.NewString(),
		UserID:        userID,
		ChannelID:     pushCh.ID,
		ChannelCode:   model.ChannelPush,
		EventType:     events.EventTypeOrderConfirmed,
		EventID:       &eventID,
		Recipient:     userID,
		Title:         "Order Confirmed",
		Content:       "Order #ORD-INT-1 confirmed",
		Status:        model.DeliveryStatusPending,
		CorrelationID: "corr-int-1",
	}
	if err := repo.CreateNotification(ctx, n1); err != nil {
		t.Fatalf("CreateNotification failed: %v", err)
	}

	// Duplicate (event_id, channel_id) must fail unique index
	dupN := &model.Notification{
		ID:          uuid.NewString(),
		UserID:      userID,
		ChannelID:   pushCh.ID,
		EventType:   events.EventTypeOrderConfirmed,
		EventID:     &eventID,
		Recipient:   userID,
		Title:       "Dup",
		Content:     "Dup",
		Status:      model.DeliveryStatusPending,
	}
	if dupErr := repo.CreateNotification(ctx, dupN); dupErr == nil {
		t.Errorf("expected unique constraint error on duplicate (event_id, channel_id)")
	}

	if err := repo.UpdateDeliveryStatus(ctx, n1.ID, model.DeliveryStatusSent, nil, false); err != nil {
		t.Fatalf("UpdateDeliveryStatus failed: %v", err)
	}

	items, total, unread, err := repo.ListNotifications(ctx, model.NotificationFilter{
		UserID:  userID,
		Status:  "SENT",
		Channel: "PUSH",
		Limit:   10,
		Offset:  0,
	})
	if err != nil || total != 1 || unread != 1 || len(items) != 1 || items[0].Status != model.DeliveryStatusSent {
		t.Fatalf("ListNotifications failed: total=%d unread=%d len=%d err=%v", total, unread, len(items), err)
	}

	marked, err := repo.MarkAsRead(ctx, n1.ID, userID, true)
	if err != nil || !marked.IsRead || marked.ReadAt == nil {
		t.Fatalf("MarkAsRead failed: %+v (err=%v)", marked, err)
	}
	if cnt, err := repo.GetUnreadCount(ctx, userID); err != nil || cnt != 0 {
		t.Errorf("expected unread count 0 after MarkAsRead, got %d (err=%v)", cnt, err)
	}
}

func TestKafkaNotificationConsumerAndDLQIntegration(t *testing.T) {
	rawBrokers := strings.TrimSpace(os.Getenv("TEST_KAFKA_BROKERS"))
	if rawBrokers == "" {
		t.Skip("Skipping Kafka integration test: TEST_KAFKA_BROKERS not set")
	}
	brokers := strings.Split(rawBrokers, ",")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cfg := kafka.Config{
		Brokers:        brokers,
		ClientID:       "notification-int-test",
		ConnectTimeout: 3 * time.Second,
		MaxRetries:     2,
		RetryInterval:  200 * time.Millisecond,
		DLQSuffix:      ".dlq",
	}
	producer := kafka.NewProducer(cfg)
	defer producer.Close()

	if err := producer.Ping(ctx); err != nil {
		t.Skipf("Skipping Kafka integration test: broker unavailable: %v", err)
	}

	testTopic := "gocart.notification.inttest." + uuid.NewString()
	groupID := "notification-inttest-group-" + uuid.NewString()

	pubVAPID, privVAPID, _ := push.GenerateVAPIDKeys()
	var delivered atomic.Int32
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			delivered.Add(1)
			rec := httptest.NewRecorder()
			rec.WriteHeader(http.StatusCreated)
			return rec.Result(), nil
		}),
	}
	logger := slog.New(slog.DiscardHandler)
	tmplSvc := service.NewTemplateService()
	pushClient := push.NewWebPushClient(
		config.VAPIDConfig{PublicKey: pubVAPID, PrivateKey: privVAPID, Subject: "mailto:int@gocart.local"},
		config.PushConfig{RequestTimeout: time.Second, MaxRetries: 1, RetryInterval: 5 * time.Millisecond},
		httpClient,
		logger,
	)

	memRepo := newIntMemRepo()
	userID := uuid.NewString()
	p256, auth := genTestSubKeys(t)
	_, _ = memRepo.UpsertSubscription(ctx, &model.PushSubscription{
		UserID:   userID,
		Endpoint: "https://push.example.com/kafka-int",
		P256DH:   p256,
		Auth:     auth,
	})

	prefSvc := service.NewPreferenceService(memRepo, memRepo)
	notifSvc := service.NewNotificationService(memRepo, memRepo, memRepo, memRepo, tmplSvc, pushClient, logger)
	pipeline := service.NewPushNotificationPipelineService(tmplSvc, prefSvc, memRepo, memRepo, notifSvc, nil, logger)
	evtConsumer := consumer.NewNotificationEventConsumer(pipeline, logger)

	dlqCh := make(chan string, 1)
	evtConsumer.SetDLQPublisher(func(pCtx context.Context, env *events.EventEnvelope, reason error) error {
		dlqTopic := testTopic + ".dlq"
		if err := producer.Publish(pCtx, dlqTopic, env.EventID, env); err != nil {
			return err
		}
		dlqCh <- env.EventID
		return nil
	})

	kafkaConsumer := kafka.NewConsumer(cfg, kafka.ConsumerConfig{
		GroupID: groupID,
		Topics:  []string{testTopic},
	}, producer)
	defer kafkaConsumer.Close()

	consCtx, consCancel := context.WithCancel(ctx)
	defer consCancel()
	go func() {
		err := kafkaConsumer.Start(consCtx, evtConsumer.HandleEvent)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("consumer stopped: %v", err)
		}
	}()

	// Publish valid event and poison-pill event
	validEnv, _ := events.NewEventEnvelope(events.EventTypeOrderConfirmed, "order-service", events.OrderConfirmedEvent{
		OrderID: "ord-kafka-1", UserID: userID, PaymentStatus: "PAID", ConfirmedAt: time.Now(),
	})
	if err := producer.Publish(ctx, testTopic, validEnv.EventID, validEnv); err != nil {
		t.Fatalf("failed publishing valid test event: %v", err)
	}

	poisonEnv := &events.EventEnvelope{
		EventID:   uuid.NewString(),
		EventType: events.EventTypeOrderConfirmed,
		Source:    "order-service",
		Timestamp: time.Now().UTC(),
		Data:      []byte(`{bad-json`),
	}
	if err := producer.Publish(ctx, testTopic, poisonEnv.EventID, poisonEnv); err != nil {
		t.Fatalf("failed publishing poison event: %v", err)
	}

	deadline := time.After(15 * time.Second)
	gotDLQ := false
	for !gotDLQ || delivered.Load() < 1 {
		select {
		case dlqID := <-dlqCh:
			if dlqID == poisonEnv.EventID {
				gotDLQ = true
			}
		case <-deadline:
			t.Fatalf("timed out waiting for Kafka processing (delivered=%d, gotDLQ=%v)", delivered.Load(), gotDLQ)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

type intMemRepo struct {
	chans  map[string]*model.Channel
	prefs  map[string]*model.Preference
	jobs   map[string]*model.NotificationJob
	proc   map[string]*model.ProcessedEvent
	notifs map[string]*model.Notification
	subs   map[string]*model.PushSubscription
}

func newIntMemRepo() *intMemRepo {
	r := &intMemRepo{
		chans:  make(map[string]*model.Channel),
		prefs:  make(map[string]*model.Preference),
		jobs:   make(map[string]*model.NotificationJob),
		proc:   make(map[string]*model.ProcessedEvent),
		notifs: make(map[string]*model.Notification),
		subs:   make(map[string]*model.PushSubscription),
	}
	for _, c := range []string{model.ChannelPush, model.ChannelEmail} {
		ch := &model.Channel{ID: uuid.NewString(), Code: c, Name: c, IsActive: true}
		r.chans[c] = ch
		r.chans[ch.ID] = ch
	}
	return r
}

func (r *intMemRepo) GetByCode(ctx context.Context, code string) (*model.Channel, error) {
	return r.chans[code], nil
}
func (r *intMemRepo) GetByID(ctx context.Context, id string) (*model.Channel, error) {
	return r.chans[id], nil
}
func (r *intMemRepo) GetByEventType(ctx context.Context, eventType string) (*model.Template, error) {
	return nil, nil
}
func (r *intMemRepo) GetByEventTypeAndChannel(ctx context.Context, eventType, channelID string) (*model.Template, error) {
	return nil, nil
}
func (r *intMemRepo) GetPreference(ctx context.Context, userID, channelID, category string) (*model.Preference, error) {
	return nil, nil
}
func (r *intMemRepo) GetPreferencesByUserID(ctx context.Context, userID string) ([]*model.Preference, error) {
	return nil, nil
}
func (r *intMemRepo) UpsertPreference(ctx context.Context, pref *model.Preference) error {
	return nil
}
func (r *intMemRepo) CreateJob(ctx context.Context, job *model.NotificationJob) error {
	job.ID = uuid.NewString()
	r.jobs[job.EventID] = job
	return nil
}
func (r *intMemRepo) GetJobByEventID(ctx context.Context, eventID string) (*model.NotificationJob, error) {
	return r.jobs[eventID], nil
}
func (r *intMemRepo) GetJobByID(ctx context.Context, id string) (*model.NotificationJob, error) {
	return nil, nil
}
func (r *intMemRepo) UpdateJobStatus(ctx context.Context, id string, status model.JobStatus, errMsg *string) error {
	return nil
}
func (r *intMemRepo) IncrementRetry(ctx context.Context, id string, errMsg string) error {
	return nil
}
func (r *intMemRepo) ListPendingJobs(ctx context.Context, limit int) ([]*model.NotificationJob, error) {
	return nil, nil
}
func (r *intMemRepo) ClaimPendingJobs(ctx context.Context, limit int) ([]*model.NotificationJob, error) {
	return nil, nil
}
func (r *intMemRepo) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	_, ok := r.proc[eventID]
	return ok, nil
}
func (r *intMemRepo) RecordProcessedEvent(ctx context.Context, event *model.ProcessedEvent) error {
	r.proc[event.EventID] = event
	return nil
}
func (r *intMemRepo) UpsertSubscription(ctx context.Context, sub *model.PushSubscription) (*model.PushSubscription, error) {
	sub.ID = uuid.NewString()
	sub.IsActive = true
	r.subs[sub.ID] = sub
	return sub, nil
}
func (r *intMemRepo) GetSubscriptionByID(ctx context.Context, id string) (*model.PushSubscription, error) {
	return r.subs[id], nil
}
func (r *intMemRepo) GetSubscriptionByEndpoint(ctx context.Context, endpoint string) (*model.PushSubscription, error) {
	return nil, nil
}
func (r *intMemRepo) ListSubscriptionsByUserID(ctx context.Context, userID string, activeOnly bool) ([]*model.PushSubscription, error) {
	var res []*model.PushSubscription
	for _, s := range r.subs {
		if s.UserID == userID && (!activeOnly || s.IsActive) {
			res = append(res, s)
		}
	}
	return res, nil
}
func (r *intMemRepo) UpdateSubscription(ctx context.Context, sub *model.PushSubscription) (*model.PushSubscription, error) {
	return sub, nil
}
func (r *intMemRepo) DeactivateSubscription(ctx context.Context, id, userID, endpoint string) (bool, error) {
	return true, nil
}
func (r *intMemRepo) MarkSubscriptionGone(ctx context.Context, id string, errMsg string) error {
	return nil
}
func (r *intMemRepo) RecordSubscriptionFailure(ctx context.Context, id string, errMsg string) error {
	return nil
}
func (r *intMemRepo) MarkSubscriptionUsed(ctx context.Context, id string) error {
	return nil
}
func (r *intMemRepo) CreateNotification(ctx context.Context, n *model.Notification) error {
	if n.ID == "" {
		n.ID = uuid.NewString()
	}
	r.notifs[n.ID] = n
	return nil
}
func (r *intMemRepo) GetNotificationByID(ctx context.Context, id string) (*model.Notification, error) {
	return r.notifs[id], nil
}
func (r *intMemRepo) GetNotificationByEventIDAndChannel(ctx context.Context, eventID, channelID string) (*model.Notification, error) {
	return nil, nil
}
func (r *intMemRepo) ListNotifications(ctx context.Context, filter model.NotificationFilter) ([]*model.Notification, int, int, error) {
	return nil, 0, 0, nil
}
func (r *intMemRepo) UpdateDeliveryStatus(ctx context.Context, id string, status model.DeliveryStatus, errMsg *string, inc bool) error {
	return nil
}
func (r *intMemRepo) MarkAsRead(ctx context.Context, id, userID string, isRead bool) (*model.Notification, error) {
	return nil, nil
}
func (r *intMemRepo) MarkAllAsRead(ctx context.Context, userID string) (int, error) {
	return 0, nil
}
func (r *intMemRepo) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	return 0, nil
}

var _ = dto.PushPayload{}

