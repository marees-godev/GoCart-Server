package consumer_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
)

type memoryNotificationRepo struct {
	mu              sync.RWMutex
	channels        map[string]*model.Channel
	templates       map[string]*model.Template
	preferences     map[string]*model.Preference
	jobs            map[string]*model.NotificationJob
	jobsByEventID   map[string]*model.NotificationJob
	processedEvents map[string]*model.ProcessedEvent
	failCreateJob   bool
}

func newMemoryRepo() *memoryNotificationRepo {
	repo := &memoryNotificationRepo{
		channels:        make(map[string]*model.Channel),
		templates:       make(map[string]*model.Template),
		preferences:     make(map[string]*model.Preference),
		jobs:            make(map[string]*model.NotificationJob),
		jobsByEventID:   make(map[string]*model.NotificationJob),
		processedEvents: make(map[string]*model.ProcessedEvent),
	}

	// Seed channels
	emailChannel := &model.Channel{
		ID:        uuid.NewString(),
		Code:      "EMAIL",
		Name:      "Email Notifications",
		IsActive:  true,
		CreatedAt: time.Now(),
	}
	repo.channels["EMAIL"] = emailChannel
	repo.channels[emailChannel.ID] = emailChannel

	// Seed templates for 11 domain events
	seedTemplates := []struct {
		eventType string
		subject   string
		body      string
	}{
		{"UserRegistered", "Welcome to GoCart, {{.Email}}!", "Hello {{.Email}}, welcome to GoCart!"},
		{"StoreApproved", "Store Approved: Store #{{.StoreID}}", "Your store {{.StoreID}} has been approved."},
		{"StoreRejected", "Store Rejected: Store #{{.StoreID}}", "Store {{.StoreID}} rejected. Reason: {{.Reason}}"},
		{"OrderCreated", "Order Confirmation - Order #{{.OrderID}}", "Thank you for your order #{{.OrderID}} of {{.TotalAmount}} {{.Currency}}."},
		{"PaymentSuccessful", "Payment Receipt - Order #{{.OrderID}}", "Payment of {{.Amount}} {{.Currency}} successful for order #{{.OrderID}}."},
		{"PaymentFailed", "Payment Failed - Order #{{.OrderID}}", "Payment of {{.Amount}} failed for order #{{.OrderID}}. Reason: {{.Reason}}"},
		{"OrderShipped", "Your Order #{{.OrderID}} has Shipped!", "Order #{{.OrderID}} shipped via {{.DeliveryPartner}} (Tracking: {{.TrackingNumber}})."},
		{"OrderDelivered", "Order #{{.OrderID}} Delivered", "Order #{{.OrderID}} has been delivered."},
		{"ReturnApproved", "Return Approved - Return #{{.ReturnID}}", "Return #{{.ReturnID}} for order #{{.OrderID}} approved. Refund: {{.RefundAmount}}."},
		{"ReturnRejected", "Return Rejected - Return #{{.ReturnID}}", "Return #{{.ReturnID}} rejected. Reason: {{.RejectionReason}}."},
		{"RefundCompleted", "Refund Completed - Order #{{.OrderID}}", "Refund of {{.Amount}} {{.Currency}} completed for order #{{.OrderID}}."},
	}

	for _, st := range seedTemplates {
		tmpl := &model.Template{
			ID:              uuid.NewString(),
			EventType:       st.eventType,
			ChannelID:       emailChannel.ID,
			SubjectTemplate: st.subject,
			BodyTemplate:    st.body,
			IsActive:        true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		repo.templates[st.eventType] = tmpl
	}

	return repo
}

func (r *memoryNotificationRepo) GetByCode(ctx context.Context, code string) (*model.Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.channels[code], nil
}

func (r *memoryNotificationRepo) GetByID(ctx context.Context, id string) (*model.Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.channels[id], nil
}

func (r *memoryNotificationRepo) GetByEventType(ctx context.Context, eventType string) (*model.Template, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.templates[eventType], nil
}

func (r *memoryNotificationRepo) GetByEventTypeAndChannel(ctx context.Context, eventType string, channelID string) (*model.Template, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tmpl, ok := r.templates[eventType]
	if ok && tmpl.ChannelID == channelID {
		return tmpl, nil
	}
	return tmpl, nil
}

func (r *memoryNotificationRepo) GetPreference(ctx context.Context, userID, channelID, eventType string) (*model.Preference, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := fmt.Sprintf("%s:%s:%s", userID, channelID, eventType)
	return r.preferences[key], nil
}

func (r *memoryNotificationRepo) UpsertPreference(ctx context.Context, pref *model.Preference) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := fmt.Sprintf("%s:%s:%s", pref.UserID, pref.ChannelID, pref.EventType)
	r.preferences[key] = pref
	return nil
}

func (r *memoryNotificationRepo) CreateJob(ctx context.Context, job *model.NotificationJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.failCreateJob {
		return fmt.Errorf("simulated database failure")
	}

	if _, exists := r.jobsByEventID[job.EventID]; exists {
		return fmt.Errorf("job already exists for event_id %s", job.EventID)
	}

	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	job.CreatedAt = time.Now().UTC()
	job.UpdatedAt = time.Now().UTC()

	copied := *job
	r.jobs[job.ID] = &copied
	r.jobsByEventID[job.EventID] = &copied
	return nil
}

func (r *memoryNotificationRepo) GetJobByEventID(ctx context.Context, eventID string) (*model.NotificationJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if j, ok := r.jobsByEventID[eventID]; ok {
		c := *j
		return &c, nil
	}
	return nil, nil
}

func (r *memoryNotificationRepo) GetJobByID(ctx context.Context, id string) (*model.NotificationJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if j, ok := r.jobs[id]; ok {
		c := *j
		return &c, nil
	}
	return nil, nil
}

func (r *memoryNotificationRepo) UpdateJobStatus(ctx context.Context, id string, status model.JobStatus, errMsg *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j, ok := r.jobs[id]; ok {
		j.Status = status
		j.ErrorMessage = errMsg
		now := time.Now().UTC()
		j.UpdatedAt = now
		if status == model.JobStatusCompleted || status == model.JobStatusFailed || status == model.JobStatusOptedOut {
			j.ProcessedAt = &now
		}
	}
	return nil
}

func (r *memoryNotificationRepo) IncrementRetry(ctx context.Context, id string, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j, ok := r.jobs[id]; ok {
		j.RetryCount++
		j.ErrorMessage = &errMsg
		if j.RetryCount >= j.MaxRetries {
			j.Status = model.JobStatusFailed
		}
		j.UpdatedAt = time.Now().UTC()
	}
	return nil
}

func (r *memoryNotificationRepo) ListPendingJobs(ctx context.Context, limit int) ([]*model.NotificationJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []*model.NotificationJob
	for _, j := range r.jobs {
		if j.Status == model.JobStatusPending && j.RetryCount < j.MaxRetries {
			c := *j
			list = append(list, &c)
			if len(list) >= limit {
				break
			}
		}
	}
	return list, nil
}

func (r *memoryNotificationRepo) ClaimPendingJobs(ctx context.Context, limit int) ([]*model.NotificationJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []*model.NotificationJob
	for _, j := range r.jobs {
		if j.Status == model.JobStatusPending && j.RetryCount < j.MaxRetries {
			j.Status = model.JobStatusProcessing
			now := time.Now().UTC()
			j.UpdatedAt = now
			c := *j
			list = append(list, &c)
			if len(list) >= limit {
				break
			}
		}
	}
	return list, nil
}

func (r *memoryNotificationRepo) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.processedEvents[eventID]
	return ok, nil
}

func (r *memoryNotificationRepo) RecordProcessedEvent(ctx context.Context, event *model.ProcessedEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processedEvents[event.EventID] = event
	return nil
}

func setupConsumerTest() (*consumer.NotificationEventConsumer, *memoryNotificationRepo) {
	repo := newMemoryRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tmplSvc := service.NewTemplateService(repo)
	prefSvc := service.NewPreferenceService(repo, repo)
	pipelineSvc := service.NewNotificationPipelineService(tmplSvc, prefSvc, repo, repo, logger)
	evtConsumer := consumer.NewNotificationEventConsumer(pipelineSvc, logger)
	return evtConsumer, repo
}

// 1. Acceptance Criteria: Notification Service consumes all required domain events and maps appropriately
func TestNotificationConsumer_AllRequiredDomainEvents(t *testing.T) {
	c, repo := setupConsumerTest()
	ctx := context.Background()

	testCases := []struct {
		name              string
		eventType         string
		payload           interface{}
		expectedRecipient string
		expectedSubject   string
		expectedInBody    string
	}{
		{
			name:      "UserRegistered",
			eventType: events.EventTypeUserRegistered,
			payload: events.UserRegisteredEvent{
				UserID: uuid.NewString(),
				Email:  "customer@example.com",
				Role:   "CUSTOMER",
			},
			expectedRecipient: "customer@example.com",
			expectedSubject:   "Welcome to GoCart, customer@example.com!",
			expectedInBody:    "welcome to GoCart!",
		},
		{
			name:      "StoreApproved",
			eventType: events.EventTypeStoreApproved,
			payload: events.StoreApprovedEvent{
				StoreID:    "store-100",
				MerchantID: "merchant-200",
				AdminID:    "admin-1",
				ApprovedAt: time.Now(),
			},
			expectedRecipient: "merchant-200",
			expectedSubject:   "Store Approved: Store #store-100",
			expectedInBody:    "approved",
		},
		{
			name:      "StoreRejected",
			eventType: events.EventTypeStoreRejected,
			payload: events.StoreRejectedEvent{
				StoreID:    "store-101",
				MerchantID: "merchant-201",
				AdminID:    "admin-1",
				Reason:     "Invalid tax identification",
				RejectedAt: time.Now(),
			},
			expectedRecipient: "merchant-201",
			expectedSubject:   "Store Rejected: Store #store-101",
			expectedInBody:    "Invalid tax identification",
		},
		{
			name:      "OrderCreated",
			eventType: events.EventTypeOrderCreated,
			payload: events.OrderCreatedEvent{
				OrderID:     "ord-1000",
				UserID:      "usr-555",
				TotalAmount: 99.95,
				Currency:    "USD",
				Status:      "CREATED",
			},
			expectedRecipient: "usr-555",
			expectedSubject:   "Order Confirmation - Order #ord-1000",
			expectedInBody:    "99.95 USD",
		},
		{
			name:      "PaymentSuccessful",
			eventType: events.EventTypePaymentSuccessful,
			payload: events.PaymentSuccessfulEvent{
				PaymentID:     "pay-3000",
				OrderID:       "ord-1000",
				UserID:        "usr-555",
				Amount:        99.95,
				Currency:      "USD",
				PaymentMethod: "CREDIT_CARD",
				PaidAt:        time.Now(),
			},
			expectedRecipient: "usr-555",
			expectedSubject:   "Payment Receipt - Order #ord-1000",
			expectedInBody:    "99.95 USD successful",
		},
		{
			name:      "PaymentFailed",
			eventType: events.EventTypePaymentFailed,
			payload: events.PaymentFailedEvent{
				PaymentID: "pay-3001",
				OrderID:   "ord-1001",
				UserID:    "usr-556",
				Amount:    45.50,
				Reason:    "Insufficient funds",
				FailedAt:  time.Now(),
			},
			expectedRecipient: "usr-556",
			expectedSubject:   "Payment Failed - Order #ord-1001",
			expectedInBody:    "Insufficient funds",
		},
		{
			name:      "OrderShipped",
			eventType: events.EventTypeOrderShipped,
			payload: events.OrderShippedEvent{
				OrderID:         "ord-1000",
				UserID:          "usr-555",
				TrackingNumber:  "TRACK-8899",
				DeliveryPartner: "DHL Express",
				ShippedAt:       time.Now(),
			},
			expectedRecipient: "usr-555",
			expectedSubject:   "Your Order #ord-1000 has Shipped!",
			expectedInBody:    "DHL Express",
		},
		{
			name:      "OrderDelivered",
			eventType: events.EventTypeOrderDelivered,
			payload: events.OrderDeliveredEvent{
				OrderID:     "ord-1000",
				UserID:      "usr-555",
				DeliveredAt: time.Now(),
			},
			expectedRecipient: "usr-555",
			expectedSubject:   "Order #ord-1000 Delivered",
			expectedInBody:    "has been delivered",
		},
		{
			name:      "ReturnApproved",
			eventType: events.EventTypeReturnApproved,
			payload: events.ReturnApprovedEvent{
				ReturnID:     "ret-401",
				OrderID:      "ord-1000",
				OrderItemID:  "item-1",
				UserID:       "usr-555",
				RefundAmount: 50.00,
				ApprovedAt:   time.Now(),
			},
			expectedRecipient: "usr-555",
			expectedSubject:   "Return Approved - Return #ret-401",
			expectedInBody:    "50",
		},
		{
			name:      "ReturnRejected",
			eventType: events.EventTypeReturnRejected,
			payload: events.ReturnRejectedEvent{
				ReturnID:        "ret-402",
				OrderID:         "ord-1000",
				OrderItemID:     "item-2",
				UserID:          "usr-555",
				RejectionReason: "Item altered by buyer",
				RejectedAt:      time.Now(),
			},
			expectedRecipient: "usr-555",
			expectedSubject:   "Return Rejected - Return #ret-402",
			expectedInBody:    "Item altered by buyer",
		},
		{
			name:      "RefundCompleted",
			eventType: events.EventTypeRefundCompleted,
			payload: events.RefundCompletedEvent{
				RefundID:        "ref-501",
				PaymentID:       "pay-3000",
				OrderID:         "ord-1000",
				UserID:          "usr-555",
				Amount:          50.00,
				Currency:        "USD",
				GatewayRefundID: "gw-ref-999",
				CompletedAt:     time.Now(),
			},
			expectedRecipient: "usr-555",
			expectedSubject:   "Refund Completed - Order #ord-1000",
			expectedInBody:    "50 USD",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env, err := events.NewEventEnvelope(tc.eventType, "test-service", tc.payload)
			if err != nil {
				t.Fatalf("failed to create envelope: %v", err)
			}
			env.CorrelationID = "corr-" + uuid.NewString()

			err = c.HandleEvent(ctx, env)
			if err != nil {
				t.Fatalf("unexpected error handling event %s: %v", tc.eventType, err)
			}

			// Verify job was created in repository with recipient, channel, and template information
			job, err := repo.GetJobByEventID(ctx, env.EventID)
			if err != nil {
				t.Fatalf("failed to get job by event_id: %v", err)
			}
			if job == nil {
				t.Fatalf("expected job to be created for event %s, but got nil", tc.eventType)
			}

			if job.Recipient != tc.expectedRecipient {
				t.Errorf("expected recipient %s, got %s", tc.expectedRecipient, job.Recipient)
			}
			if job.ChannelCode != "EMAIL" {
				t.Errorf("expected channel EMAIL, got %s", job.ChannelCode)
			}
			if job.ChannelID == "" {
				t.Errorf("expected channel_id to be populated")
			}
			if job.TemplateID == "" {
				t.Errorf("expected template_id to be populated")
			}
			if job.Subject != tc.expectedSubject {
				t.Errorf("expected subject %q, got %q", tc.expectedSubject, job.Subject)
			}
			if job.Status != model.JobStatusPending {
				t.Errorf("expected status PENDING, got %s", job.Status)
			}
			if job.CorrelationID != env.CorrelationID {
				t.Errorf("expected correlation ID %s, got %s", env.CorrelationID, job.CorrelationID)
			}
		})
	}
}

// 2. Acceptance Criteria: Invalid/unsupported events do not crash the consumer
func TestNotificationConsumer_InvalidAndUnsupportedEvents(t *testing.T) {
	c, _ := setupConsumerTest()
	ctx := context.Background()

	t.Run("nil envelope does not crash", func(t *testing.T) {
		err := c.HandleEvent(ctx, nil)
		if err != nil {
			t.Errorf("expected nil envelope to be ignored cleanly, got error: %v", err)
		}
	})

	t.Run("unsupported event type does not crash and is ignored safely", func(t *testing.T) {
		env, err := events.NewEventEnvelope("UnknownDomainEvent", "some-service", map[string]string{"foo": "bar"})
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		err = c.HandleEvent(ctx, env)
		if err != nil {
			t.Errorf("expected unknown event to be ignored cleanly, got error: %v", err)
		}
	})

	t.Run("malformed event payload data does not crash", func(t *testing.T) {
		env := &events.EventEnvelope{
			EventID:   uuid.NewString(),
			EventType: events.EventTypeOrderCreated,
			Data:      []byte("invalid-non-json-data"),
		}
		err := c.HandleEvent(ctx, env)
		if err != nil {
			t.Errorf("expected malformed data payload to not crash consumer, got: %v", err)
		}
	})
}

// 3. Acceptance Criteria: Duplicate events are handled idempotently
func TestNotificationConsumer_Idempotency(t *testing.T) {
	c, repo := setupConsumerTest()
	ctx := context.Background()

	payload := events.OrderCreatedEvent{
		OrderID:     "ord-idem-1",
		UserID:      "usr-idem-1",
		TotalAmount: 150.00,
		Currency:    "USD",
		Status:      "CREATED",
	}

	env, err := events.NewEventEnvelope(events.EventTypeOrderCreated, "order-service", payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}

	// First consumption
	err = c.HandleEvent(ctx, env)
	if err != nil {
		t.Fatalf("first HandleEvent failed: %v", err)
	}

	job1, err := repo.GetJobByEventID(ctx, env.EventID)
	if err != nil || job1 == nil {
		t.Fatalf("expected job to exist after first consumption: %v", err)
	}

	initialJobCount := len(repo.jobs)

	// Duplicate consumption (same event ID)
	err = c.HandleEvent(ctx, env)
	if err != nil {
		t.Fatalf("duplicate HandleEvent failed: %v", err)
	}

	// Verify no new job was created
	if len(repo.jobs) != initialJobCount {
		t.Fatalf("expected job count to remain %d, got %d", initialJobCount, len(repo.jobs))
	}
}

// 4. Acceptance Criteria: Preferences check and channel selection
func TestNotificationConsumer_UserPreferenceOptOut(t *testing.T) {
	c, repo := setupConsumerTest()
	ctx := context.Background()

	userID := "usr-optout-123"
	channel, _ := repo.GetByCode(ctx, "EMAIL")

	// Set preference: user opted out of OrderCreated notifications
	_ = repo.UpsertPreference(ctx, &model.Preference{
		UserID:    userID,
		ChannelID: channel.ID,
		EventType: events.EventTypeOrderCreated,
		IsEnabled: false,
	})

	payload := events.OrderCreatedEvent{
		OrderID:     "ord-optout-1",
		UserID:      userID,
		TotalAmount: 25.00,
		Currency:    "USD",
	}

	env, err := events.NewEventEnvelope(events.EventTypeOrderCreated, "order-service", payload)
	if err != nil {
		t.Fatalf("failed creating envelope: %v", err)
	}

	err = c.HandleEvent(ctx, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	job, err := repo.GetJobByEventID(ctx, env.EventID)
	if err != nil || job == nil {
		t.Fatalf("expected job record with OPTED_OUT status, got: %v", err)
	}

	if job.Status != model.JobStatusOptedOut {
		t.Errorf("expected job status OPTED_OUT, got: %s", job.Status)
	}
}

// 5. Acceptance Criteria: Failed event processing can be retried
func TestNotificationConsumer_TransientErrorRetry(t *testing.T) {
	c, repo := setupConsumerTest()
	ctx := context.Background()

	// Inject database failure
	repo.failCreateJob = true

	env, err := events.NewEventEnvelope(events.EventTypeUserRegistered, "auth-service", events.UserRegisteredEvent{
		UserID: "usr-err-1",
		Email:  "error@example.com",
	})
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}

	err = c.HandleEvent(ctx, env)
	if err == nil {
		t.Fatalf("expected transient error to be returned to trigger Kafka retry")
	}

	// Recovery: restore DB, retry succeeds
	repo.failCreateJob = false
	err = c.HandleEvent(ctx, env)
	if err != nil {
		t.Fatalf("expected retry to succeed after recovery: %v", err)
	}

	job, _ := repo.GetJobByEventID(ctx, env.EventID)
	if job == nil {
		t.Fatalf("expected job to be created on successful retry")
	}
}

// 6. Asynchronous job worker processing
func TestJobWorker_ProcessBatchAndRetry(t *testing.T) {
	_, repo := setupConsumerTest()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a pending job
	job := &model.NotificationJob{
		ID:          uuid.NewString(),
		EventID:     "evt-worker-1",
		ChannelID:   repo.channels["EMAIL"].ID,
		ChannelCode: "EMAIL",
		TemplateID:  repo.templates["OrderCreated"].ID,
		EventType:   "OrderCreated",
		Recipient:   "worker@example.com",
		Subject:     "Subject",
		Content:     "Content",
		Status:      model.JobStatusPending,
		MaxRetries:  3,
	}
	_ = repo.CreateJob(ctx, job)

	worker := service.NewJobWorker(repo, logger, 50*time.Millisecond)

	// Run single batch processing
	workerCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	go worker.Start(workerCtx)
	time.Sleep(120 * time.Millisecond)

	updatedJob, _ := repo.GetJobByID(ctx, job.ID)
	if updatedJob.Status != model.JobStatusCompleted {
		t.Errorf("expected job status COMPLETED after worker execution, got: %s", updatedJob.Status)
	}
}

// 7. Test Non-UUID user_id safe handling
func TestNotificationConsumer_NonUUIDUserID_SafeHandling(t *testing.T) {
	c, repo := setupConsumerTest()
	ctx := context.Background()

	// RefundCompletedEvent with non-UUID AggregateID and empty UserID
	env, err := events.NewEventEnvelope(events.EventTypeRefundCompleted, "payment-service", events.RefundCompletedEvent{
		RefundID:    "ref-9999",
		PaymentID:   "pay-1111",
		OrderID:     "ord-2222",
		Amount:      50.00,
		Currency:    "USD",
		CompletedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("failed creating envelope: %v", err)
	}
	env.AggregateID = "non-uuid-refund-aggregate-id"

	err = c.HandleEvent(ctx, env)
	if err != nil {
		t.Fatalf("unexpected error with non-uuid fallback: %v", err)
	}

	job, err := repo.GetJobByEventID(ctx, env.EventID)
	if err != nil || job == nil {
		t.Fatalf("expected job to be created, got error: %v", err)
	}

	if job.UserID != nil {
		t.Errorf("expected UserID to be nil for non-UUID input, got: %s", *job.UserID)
	}
	if job.Recipient != "ord-2222" {
		t.Errorf("expected recipient ord-2222, got: %s", job.Recipient)
	}
}

