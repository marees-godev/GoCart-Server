package service_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
)

type mockTemplateRepo struct {
	mu        sync.RWMutex
	templates map[string]*model.Template
}

func (m *mockTemplateRepo) GetByEventType(ctx context.Context, eventType string) (*model.Template, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.templates[eventType], nil
}

func (m *mockTemplateRepo) GetByEventTypeAndChannel(ctx context.Context, eventType string, channelID string) (*model.Template, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.templates[eventType], nil
}

type mockChannelRepo struct {
	mu       sync.RWMutex
	channels map[string]*model.Channel
}

func (m *mockChannelRepo) GetByCode(ctx context.Context, code string) (*model.Channel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.channels[code], nil
}

func (m *mockChannelRepo) GetByID(ctx context.Context, id string) (*model.Channel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.channels[id], nil
}

type mockPreferenceRepo struct {
	mu          sync.RWMutex
	preferences map[string]*model.Preference
}

func (m *mockPreferenceRepo) GetPreference(ctx context.Context, userID, channelID, eventType string) (*model.Preference, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.preferences[userID+":"+channelID+":"+eventType], nil
}

func (m *mockPreferenceRepo) UpsertPreference(ctx context.Context, pref *model.Preference) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.preferences[pref.UserID+":"+pref.ChannelID+":"+pref.EventType] = pref
	return nil
}

type mockJobRepo struct {
	mu            sync.RWMutex
	jobs          map[string]*model.NotificationJob
	jobsByEventID map[string]*model.NotificationJob
}

func (m *mockJobRepo) CreateJob(ctx context.Context, job *model.NotificationJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	copied := *job
	m.jobs[job.ID] = &copied
	m.jobsByEventID[job.EventID] = &copied
	return nil
}

func (m *mockJobRepo) GetJobByEventID(ctx context.Context, eventID string) (*model.NotificationJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if j, ok := m.jobsByEventID[eventID]; ok {
		copied := *j
		return &copied, nil
	}
	return nil, nil
}

func (m *mockJobRepo) GetJobByID(ctx context.Context, id string) (*model.NotificationJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if j, ok := m.jobs[id]; ok {
		copied := *j
		return &copied, nil
	}
	return nil, nil
}

func (m *mockJobRepo) UpdateJobStatus(ctx context.Context, id string, status model.JobStatus, errMsg *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobs[id]; ok {
		j.Status = status
		j.ErrorMessage = errMsg
	}
	return nil
}

func (m *mockJobRepo) IncrementRetry(ctx context.Context, id string, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobs[id]; ok {
		j.RetryCount++
	}
	return nil
}

func (m *mockJobRepo) ListPendingJobs(ctx context.Context, limit int) ([]*model.NotificationJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*model.NotificationJob
	for _, j := range m.jobs {
		if j.Status == model.JobStatusPending {
			copied := *j
			list = append(list, &copied)
		}
	}
	return list, nil
}

func (m *mockJobRepo) ClaimPendingJobs(ctx context.Context, limit int) ([]*model.NotificationJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []*model.NotificationJob
	for _, j := range m.jobs {
		if j.Status == model.JobStatusPending {
			j.Status = model.JobStatusProcessing
			copied := *j
			list = append(list, &copied)
			if len(list) >= limit {
				break
			}
		}
	}
	return list, nil
}

type mockIdempotencyRepo struct {
	mu        sync.RWMutex
	processed map[string]*model.ProcessedEvent
}

func (m *mockIdempotencyRepo) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.processed[eventID]
	return ok, nil
}

func (m *mockIdempotencyRepo) RecordProcessedEvent(ctx context.Context, event *model.ProcessedEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processed[event.EventID] = event
	return nil
}

func TestTemplateService_Render(t *testing.T) {
	channelID := uuid.NewString()
	templateRepo := &mockTemplateRepo{
		templates: map[string]*model.Template{
			"OrderCreated": {
				ID:              uuid.NewString(),
				EventType:       "OrderCreated",
				ChannelID:       channelID,
				SubjectTemplate: "Order Confirmation #{{.OrderID}}",
				BodyTemplate:    "Thank you {{.UserID}}! Your order #{{.OrderID}} of {{.TotalAmount}} is placed.",
				IsActive:        true,
			},
		},
	}

	tmplSvc := service.NewTemplateService(templateRepo)
	ctx := context.Background()

	tmpl, err := tmplSvc.ResolveTemplate(ctx, "OrderCreated", channelID)
	if err != nil {
		t.Fatalf("failed to resolve template: %v", err)
	}

	data := map[string]interface{}{
		"OrderID":     "ORD-999",
		"UserID":      "USR-111",
		"TotalAmount": 49.99,
	}

	subj, body, err := tmplSvc.Render(tmpl, data)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	if subj != "Order Confirmation #ORD-999" {
		t.Errorf("unexpected subject: %s", subj)
	}
	if body != "Thank you USR-111! Your order #ORD-999 of 49.99 is placed." {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestPreferenceService_OptOut(t *testing.T) {
	channelID := uuid.NewString()
	channelRepo := &mockChannelRepo{
		channels: map[string]*model.Channel{
			"EMAIL": {
				ID:       channelID,
				Code:     "EMAIL",
				IsActive: true,
			},
		},
	}
	prefRepo := &mockPreferenceRepo{
		preferences: map[string]*model.Preference{
			"USR-OPTOUT:" + channelID + ":OrderCreated": {
				UserID:    "USR-OPTOUT",
				ChannelID: channelID,
				EventType: "OrderCreated",
				IsEnabled: false,
			},
		},
	}

	prefSvc := service.NewPreferenceService(channelRepo, prefRepo)
	ctx := context.Background()

	// Opted out user
	ch, allowed, err := prefSvc.CheckPreferenceAndSelectChannel(ctx, "USR-OPTOUT", "OrderCreated", "EMAIL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("expected allowed to be false for opted-out user")
	}
	if ch.Code != "EMAIL" {
		t.Errorf("expected EMAIL channel, got %s", ch.Code)
	}

	// Normal user (no opt-out preference stored)
	ch, allowed, err = prefSvc.CheckPreferenceAndSelectChannel(ctx, "USR-NORMAL", "OrderCreated", "EMAIL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("expected allowed to be true by default")
	}
	if ch == nil || ch.Code != "EMAIL" {
		t.Errorf("expected EMAIL channel, got %v", ch)
	}
}

func TestNotificationPipeline_CorrelationIDAndJobStructure(t *testing.T) {
	channelID := uuid.NewString()
	templateID := uuid.NewString()

	templateRepo := &mockTemplateRepo{
		templates: map[string]*model.Template{
			"OrderCreated": {
				ID:              templateID,
				EventType:       "OrderCreated",
				ChannelID:       channelID,
				SubjectTemplate: "Order #{{.OrderID}} Confirmed",
				BodyTemplate:    "Total: {{.TotalAmount}}",
				IsActive:        true,
			},
		},
	}
	channelRepo := &mockChannelRepo{
		channels: map[string]*model.Channel{
			"EMAIL": {
				ID:       channelID,
				Code:     "EMAIL",
				IsActive: true,
			},
		},
	}
	prefRepo := &mockPreferenceRepo{preferences: make(map[string]*model.Preference)}
	jobRepo := &mockJobRepo{
		jobs:          make(map[string]*model.NotificationJob),
		jobsByEventID: make(map[string]*model.NotificationJob),
	}
	idempotencyRepo := &mockIdempotencyRepo{processed: make(map[string]*model.ProcessedEvent)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tmplSvc := service.NewTemplateService(templateRepo)
	prefSvc := service.NewPreferenceService(channelRepo, prefRepo)
	pipeline := service.NewNotificationPipelineService(tmplSvc, prefSvc, jobRepo, idempotencyRepo, logger)

	ctx := context.Background()
	payload := events.OrderCreatedEvent{
		OrderID:     "ORD-CORR-1",
		UserID:      "USR-CORR-1",
		TotalAmount: 199.00,
	}
	env, _ := events.NewEventEnvelope(events.EventTypeOrderCreated, "order-service", payload)
	env.CorrelationID = "req-correlation-999"

	job, err := pipeline.ProcessEvent(ctx, env)
	if err != nil {
		t.Fatalf("failed to process event: %v", err)
	}

	if job == nil {
		t.Fatalf("expected non-nil job")
		return
	}
	if job.CorrelationID != "req-correlation-999" {
		t.Errorf("expected correlation ID 'req-correlation-999', got: %s", job.CorrelationID)
	}
	if job.Recipient != "USR-CORR-1" {
		t.Errorf("expected recipient 'USR-CORR-1', got: %s", job.Recipient)
	}
	if job.ChannelID != channelID {
		t.Errorf("expected channel ID %s, got: %s", channelID, job.ChannelID)
	}
	if job.TemplateID != templateID {
		t.Errorf("expected template ID %s, got: %s", templateID, job.TemplateID)
	}
}

func TestIsPushSupportedEvent(t *testing.T) {
	tests := []struct {
		eventType string
		expected  bool
	}{
		{"OrderCreated", true},
		{"order.created", true},
		{"order_created", true},
		{"OrderPlaced", true},
		{"OrderCancelled", true},
		{"order.cancelled", true},
		{"order_cancelled", true},
		{"OrderConfirmed", true},
		{"order.confirmed", true},
		{"OrderPaymentConfirmed", true},
		{"order.payment_confirmed", true},
		{"order_payment_confirmed", true},
		{"OrderShipped", true},
		{"order.shipped", true},
		{"OrderDelivered", true},
		{"order.delivered", true},
		{"DeliveryCreated", true},
		{"delivery.created", true},
		{"UserRegistered", false},
		{"StoreApproved", false},
		{"MerchantApproved", false},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			if got := service.IsPushSupportedEvent(tt.eventType); got != tt.expected {
				t.Errorf("IsPushSupportedEvent(%q) = %v, want %v", tt.eventType, got, tt.expected)
			}
		})
	}
}

type mockUserEmailLookup struct {
	email string
	err   error
}

func (m *mockUserEmailLookup) LookupUserEmail(ctx context.Context, userID string) (string, error) {
	return m.email, m.err
}

func TestNotificationPipeline_UserEmailLookup(t *testing.T) {
	channelID := uuid.New().String()
	templateID := uuid.New().String()

	tmplRepo := &mockTemplateRepo{
		templates: map[string]*model.Template{
			events.EventTypeOrderCreated: {
				ID:              templateID,
				EventType:       events.EventTypeOrderCreated,
				ChannelID:       channelID,
				SubjectTemplate: "Order #{{.OrderID}} Confirmed",
				BodyTemplate:    "Hello {{.Email}}, your order {{.OrderID}} is confirmed.",
				IsActive:        true,
			},
		},
	}
	channelRepo := &mockChannelRepo{
		channels: map[string]*model.Channel{
			"EMAIL": {ID: channelID, Code: "EMAIL", Name: "Email", IsActive: true},
		},
	}
	prefRepo := &mockPreferenceRepo{preferences: make(map[string]*model.Preference)}
	jobRepo := &mockJobRepo{
		jobs:          make(map[string]*model.NotificationJob),
		jobsByEventID: make(map[string]*model.NotificationJob),
	}
	idempotencyRepo := &mockIdempotencyRepo{processed: make(map[string]*model.ProcessedEvent)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tmplSvc := service.NewTemplateService(tmplRepo)
	prefSvc := service.NewPreferenceService(channelRepo, prefRepo)
	userLookup := &mockUserEmailLookup{email: "customer@example.com"}

	pipeline := service.NewPushNotificationPipelineService(tmplSvc, prefSvc, jobRepo, idempotencyRepo, nil, nil, logger, userLookup)

	ctx := context.Background()
	payload := events.OrderCreatedEvent{
		OrderID:     "ORD-12345",
		UserID:      uuid.New().String(),
		TotalAmount: 250.00,
	}
	env, err := events.NewEventEnvelope(events.EventTypeOrderCreated, "order-service", payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}

	job, err := pipeline.ProcessEvent(ctx, env)
	if err != nil {
		t.Fatalf("failed to process event: %v", err)
	}

	if job == nil {
		t.Fatalf("expected non-nil job")
	}
	if job.Recipient != "customer@example.com" {
		t.Errorf("expected recipient 'customer@example.com', got: %s", job.Recipient)
	}
	if job.Status != model.JobStatusPending {
		t.Errorf("expected status PENDING, got: %s", job.Status)
	}
}

func TestNotificationPipeline_OrderPaymentConfirmedEvent(t *testing.T) {
	channelID := uuid.New().String()
	templateID := uuid.New().String()

	tmplRepo := &mockTemplateRepo{
		templates: map[string]*model.Template{
			events.EventTypeOrderPaymentConfirmed: {
				ID:              templateID,
				EventType:       events.EventTypeOrderPaymentConfirmed,
				ChannelID:       channelID,
				SubjectTemplate: "Action Required: Payment Received for Order #{{.OrderNumber}}",
				BodyTemplate:    "Payment of {{.TotalAmount}} for Order #{{.OrderNumber}} has been received.",
				IsActive:        true,
			},
			events.EventTypeOrderConfirmed: {
				ID:              uuid.New().String(),
				EventType:       events.EventTypeOrderConfirmed,
				ChannelID:       channelID,
				SubjectTemplate: "Generic Confirmed #{{.OrderID}}",
				BodyTemplate:    "Order {{.OrderID}} is confirmed.",
				IsActive:        true,
			},
		},
	}
	channelRepo := &mockChannelRepo{
		channels: map[string]*model.Channel{
			"EMAIL": {ID: channelID, Code: "EMAIL", Name: "Email", IsActive: true},
		},
	}
	prefRepo := &mockPreferenceRepo{preferences: make(map[string]*model.Preference)}
	jobRepo := &mockJobRepo{
		jobs:          make(map[string]*model.NotificationJob),
		jobsByEventID: make(map[string]*model.NotificationJob),
	}
	idempotencyRepo := &mockIdempotencyRepo{processed: make(map[string]*model.ProcessedEvent)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tmplSvc := service.NewTemplateService(tmplRepo)
	prefSvc := service.NewPreferenceService(channelRepo, prefRepo)

	merchantEmailLookup := &mockUserEmailLookup{email: "merchant@example.com"}
	pipeline := service.NewPushNotificationPipelineService(tmplSvc, prefSvc, jobRepo, idempotencyRepo, nil, nil, logger, merchantEmailLookup)

	ctx := context.Background()

	testEventTypes := []string{
		events.EventTypeOrderPaymentConfirmed,
		"order.payment_confirmed",
		"order_payment_confirmed",
	}

	for _, evtType := range testEventTypes {
		t.Run(evtType, func(t *testing.T) {
			now := time.Now().UTC()
			payload := events.OrderPaymentConfirmedEvent{
				OrderID:       "ORD-PAY-100",
				OrderNumber:   "ORD-20261125-99999",
				UserID:        "USR-CUST-1",
				StoreID:       "STORE-MERCH-1",
				TotalAmount:   1500.00,
				Currency:      "INR",
				PaymentStatus: "PAID",
				PaidAt:        now,
			}
			env, err := events.NewEventEnvelope(evtType, "order-service", payload)
			if err != nil {
				t.Fatalf("failed to create envelope: %v", err)
			}
			env.EventID = uuid.New().String()

			job, err := pipeline.ProcessEvent(ctx, env)
			if err != nil {
				t.Fatalf("failed to process event %s: %v", evtType, err)
			}
			if job == nil {
				t.Fatalf("expected job for %s, got nil", evtType)
			}
			if job.TemplateID != templateID {
				t.Errorf("expected dedicated OrderPaymentConfirmed template %s, got %s", templateID, job.TemplateID)
			}
			if job.Recipient != "merchant@example.com" {
				t.Errorf("expected recipient 'merchant@example.com', got: %s", job.Recipient)
			}
		})
	}
}
