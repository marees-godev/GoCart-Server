package tests_test

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	gofrsuuid "github.com/gofrs/uuid/v5"
	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
)

// MockEventBus simulates the Kafka event bus with controllable failures
type MockEventBus struct {
	mu            sync.Mutex
	published     []publishedEvent
	isDown        bool
	failCountDown int
}

type publishedEvent struct {
	Topic    string
	Key      string
	Envelope *events.EventEnvelope
}

func NewMockEventBus() *MockEventBus {
	return &MockEventBus{
		published: make([]publishedEvent, 0),
	}
}

func (b *MockEventBus) Publish(ctx context.Context, topic, key string, envelope *events.EventEnvelope) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.isDown {
		return errors.New("kafka: connection refused (broker offline)")
	}
	if b.failCountDown > 0 {
		b.failCountDown--
		return errors.New("kafka: temporary network timeout")
	}

	b.published = append(b.published, publishedEvent{
		Topic:    topic,
		Key:      key,
		Envelope: envelope,
	})
	return nil
}

func (b *MockEventBus) SetDown(down bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.isDown = down
}

func (b *MockEventBus) GetPublished() []publishedEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	res := make([]publishedEvent, len(b.published))
	copy(res, b.published)
	return res
}

func (b *MockEventBus) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = make([]publishedEvent, 0)
}

// In-memory Outbox Store simulating PostgreSQL transactional ACID boundary
type MockOutboxDB struct {
	mu           sync.Mutex
	merchants    map[uuid.UUID]*model.Merchant
	outboxEvents map[gofrsuuid.UUID]*outbox.Event
	failCommit   bool
	failInsert   bool
}

func NewMockOutboxDB() *MockOutboxDB {
	return &MockOutboxDB{
		merchants:    make(map[uuid.UUID]*model.Merchant),
		outboxEvents: make(map[gofrsuuid.UUID]*outbox.Event),
	}
}

func (db *MockOutboxDB) RegisterMerchantTx(merchant *model.Merchant, forceError bool) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if forceError || db.failInsert {
		return errors.New("db: unique constraint violation or connection aborted")
	}

	// Persist merchant
	merchant.CreatedAt = time.Now().UTC()
	merchant.UpdatedAt = merchant.CreatedAt
	db.merchants[merchant.ID] = merchant

	// Construct event
	payload := events.MerchantRegisteredEvent{
		MerchantID:   merchant.ID.String(),
		Email:        merchant.BusinessEmail,
		BusinessName: merchant.BusinessName,
		FirstName:    merchant.FirstName,
		LastName:     merchant.LastName,
		Phone:        merchant.BusinessPhone,
		CreatedAt:    merchant.CreatedAt,
		RegisteredAt: merchant.CreatedAt,
	}
	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeMerchantRegistered, "merchant-service", merchant.ID.String(), payload)
	if err != nil {
		return err
	}
	bytes, _ := env.Marshal()

	evtID := gofrsuuid.Must(gofrsuuid.NewV7())
	db.outboxEvents[evtID] = &outbox.Event{
		ID:            evtID,
		AggregateType: "merchant",
		AggregateID:   merchant.ID.String(),
		EventType:     events.EventTypeMerchantRegistered,
		Payload:       bytes,
		Topic:         events.TopicMerchantRegistered,
		Status:        outbox.StatusPending,
		RetryCount:    0,
		CreatedAt:     merchant.CreatedAt,
	}

	if db.failCommit {
		delete(db.merchants, merchant.ID)
		delete(db.outboxEvents, evtID)
		return errors.New("db: transaction commit failed, rolled back")
	}

	return nil
}

func (db *MockOutboxDB) ActivateMerchantTx(merchantID uuid.UUID, adminID, reason string, forceError bool) (*model.Merchant, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	m, exists := db.merchants[merchantID]
	if !exists {
		return nil, errors.New("merchant not found")
	}

	currStatus := model.MerchantStatus(m.Status)
	targetStatus, err := model.ValidateLifecycleTransition(model.LifecycleActionActivate, currStatus)
	if err != nil {
		return nil, err
	}

	if forceError || db.failInsert {
		return nil, errors.New("db: disk failure or deadlock during activation")
	}

	// Tentative update
	prevStatus := m.Status
	m.Status = string(targetStatus)
	m.UpdatedAt = m.CreatedAt.Add(10 * time.Millisecond)

	payload := events.MerchantActivatedEvent{
		MerchantID:     merchantID.String(),
		PreviousStatus: prevStatus,
		NewStatus:      m.Status,
		ActivatedBy:    adminID,
		Reason:         reason,
		ActivatedAt:    m.UpdatedAt,
	}
	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeMerchantActivated, "merchant-service", merchantID.String(), payload)
	if err != nil {
		return nil, err
	}
	bytes, _ := env.Marshal()

	evtID := gofrsuuid.Must(gofrsuuid.NewV7())
	db.outboxEvents[evtID] = &outbox.Event{
		ID:            evtID,
		AggregateType: "merchant",
		AggregateID:   merchantID.String(),
		EventType:     events.EventTypeMerchantActivated,
		Payload:       bytes,
		Topic:         events.TopicMerchantActivated,
		Status:        outbox.StatusPending,
		RetryCount:    0,
		CreatedAt:     m.UpdatedAt,
	}

	if db.failCommit {
		m.Status = prevStatus
		delete(db.outboxEvents, evtID)
		return nil, errors.New("db: transaction commit failed, rolled back")
	}

	return m, nil
}

func (db *MockOutboxDB) SuspendMerchantTx(merchantID uuid.UUID, adminID, reason string, forceError bool) (*model.Merchant, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	m, exists := db.merchants[merchantID]
	if !exists {
		return nil, errors.New("merchant not found")
	}

	currStatus := model.MerchantStatus(m.Status)
	targetStatus, err := model.ValidateLifecycleTransition(model.LifecycleActionSuspend, currStatus)
	if err != nil {
		return nil, err
	}

	if forceError || db.failInsert {
		return nil, errors.New("db: deadlock during suspension")
	}

	prevStatus := m.Status
	m.Status = string(targetStatus)
	m.UpdatedAt = m.UpdatedAt.Add(10 * time.Millisecond)

	payload := events.MerchantSuspendedEvent{
		MerchantID:     merchantID.String(),
		PreviousStatus: prevStatus,
		NewStatus:      m.Status,
		SuspendedBy:    adminID,
		Reason:         reason,
		SuspendedAt:    m.UpdatedAt,
	}
	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeMerchantSuspended, "merchant-service", merchantID.String(), payload)
	if err != nil {
		return nil, err
	}
	bytes, _ := env.Marshal()

	evtID := gofrsuuid.Must(gofrsuuid.NewV7())
	db.outboxEvents[evtID] = &outbox.Event{
		ID:            evtID,
		AggregateType: "merchant",
		AggregateID:   merchantID.String(),
		EventType:     events.EventTypeMerchantSuspended,
		Payload:       bytes,
		Topic:         events.TopicMerchantSuspended,
		Status:        outbox.StatusPending,
		RetryCount:    0,
		CreatedAt:     m.UpdatedAt,
	}

	if db.failCommit {
		m.Status = prevStatus
		delete(db.outboxEvents, evtID)
		return nil, errors.New("db: transaction commit failed, rolled back")
	}

	return m, nil
}

// Simulate Relay / Publisher cycle (emulating PostgreSQL ORDER BY created_at ASC FOR UPDATE SKIP LOCKED)
func (db *MockOutboxDB) ProcessRelay(ctx context.Context, bus *MockEventBus, cfg outbox.Config) (int, int) {
	db.mu.Lock()
	defer db.mu.Unlock()

	publishedCount := 0
	failedCount := 0

	pendingList := make([]*outbox.Event, 0)
	for _, evt := range db.outboxEvents {
		if evt.Status == outbox.StatusPending {
			pendingList = append(pendingList, evt)
		}
	}
	sort.Slice(pendingList, func(i, j int) bool {
		return pendingList[i].CreatedAt.Before(pendingList[j].CreatedAt)
	})

	for _, evt := range pendingList {
		env, err := events.UnmarshalEnvelope(evt.Payload)
		if err != nil {
			evt.Status = outbox.StatusFailed
			failedCount++
			continue
		}

		err = bus.Publish(ctx, evt.Topic, evt.AggregateID, env)
		if err != nil {
			evt.RetryCount++
			failedCount++
			if evt.RetryCount >= cfg.MaxRetries {
				evt.Status = outbox.StatusFailed
				// Forward to DLQ
				dlqTopic := cfg.DLQTopic
				if dlqTopic == "" {
					dlqTopic = evt.Topic + ".dlq"
				}
				_ = bus.Publish(ctx, dlqTopic, evt.AggregateID, env)
			}
			continue
		}

		evt.Status = outbox.StatusPublished
		now := time.Now().UTC()
		evt.PublishedAt = &now
		publishedCount++
	}

	return publishedCount, failedCount
}

func (db *MockOutboxDB) RequeueFailed() int {
	db.mu.Lock()
	defer db.mu.Unlock()

	count := 0
	for _, evt := range db.outboxEvents {
		if evt.Status == outbox.StatusFailed {
			evt.Status = outbox.StatusPending
			evt.RetryCount = 0
			count++
		}
	}
	return count
}

// -------------------------------------------------------------
// Test 1: State Transition Tests (Register, Activate, Suspend)
// -------------------------------------------------------------
func TestStateTransitionEvents(t *testing.T) {
	db := NewMockOutboxDB()
	bus := NewMockEventBus()
	ctx := context.Background()
	cfg := outbox.DefaultConfig()

	// 1. Register Merchant
	merchantID := uuid.New()
	merch := &model.Merchant{
		ID:            merchantID,
		BusinessName:  "Aurora Retail",
		BusinessEmail: "aurora@example.com",
		FirstName:     "Jane",
		LastName:      "Doe",
		BusinessPhone: "+15551234567",
		Status:        string(model.MerchantStatusPending),
	}

	if err := db.RegisterMerchantTx(merch, false); err != nil {
		t.Fatalf("failed to register merchant: %v", err)
	}

	if len(db.outboxEvents) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(db.outboxEvents))
	}

	// 2. Activate Merchant
	_, err := db.ActivateMerchantTx(merchantID, "admin-101", "Approved via KYC", false)
	if err != nil {
		t.Fatalf("failed to activate merchant: %v", err)
	}

	if len(db.outboxEvents) != 2 {
		t.Fatalf("expected 2 outbox events, got %d", len(db.outboxEvents))
	}

	// 3. Suspend Merchant
	_, err = db.SuspendMerchantTx(merchantID, "admin-101", "Suspicious chargebacks", false)
	if err != nil {
		t.Fatalf("failed to suspend merchant: %v", err)
	}

	if len(db.outboxEvents) != 3 {
		t.Fatalf("expected 3 outbox events, got %d", len(db.outboxEvents))
	}

	// 4. Relay publishes all 3 events
	published, failed := db.ProcessRelay(ctx, bus, cfg)
	if failed != 0 {
		t.Errorf("expected 0 relay failures, got %d", failed)
	}
	if published != 3 {
		t.Errorf("expected 3 published events, got %d", published)
	}

	// Verify published messages on bus
	messages := bus.GetPublished()
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages on bus, got %d", len(messages))
	}

	expectedTypes := []string{
		events.EventTypeMerchantRegistered,
		events.EventTypeMerchantActivated,
		events.EventTypeMerchantSuspended,
	}
	expectedTopics := []string{
		events.TopicMerchantRegistered,
		events.TopicMerchantActivated,
		events.TopicMerchantSuspended,
	}

	for i, msg := range messages {
		if msg.Envelope.EventType != expectedTypes[i] {
			t.Errorf("event %d: expected type %s, got %s", i, expectedTypes[i], msg.Envelope.EventType)
		}
		if msg.Topic != expectedTopics[i] {
			t.Errorf("event %d: expected topic %s, got %s", i, expectedTopics[i], msg.Topic)
		}
		if msg.Key != merchantID.String() {
			t.Errorf("event %d: expected partition key %s, got %s", i, merchantID.String(), msg.Key)
		}
	}
}

// -------------------------------------------------------------
// Test 2: Rollback Test (Atomicity & Failure Isolation)
// -------------------------------------------------------------
func TestRollbackOnDBError(t *testing.T) {
	db := NewMockOutboxDB()
	bus := NewMockEventBus()
	ctx := context.Background()
	cfg := outbox.DefaultConfig()

	// Case A: DB error during insert
	merch := &model.Merchant{
		ID:            uuid.New(),
		BusinessName:  "Fail Co",
		BusinessEmail: "fail@example.com",
		Status:        string(model.MerchantStatusPending),
	}

	err := db.RegisterMerchantTx(merch, true)
	if err == nil {
		t.Fatal("expected insert error, got nil")
	}

	// Assert no merchant and no outbox event
	if len(db.merchants) != 0 {
		t.Errorf("expected 0 merchants in db, got %d", len(db.merchants))
	}
	if len(db.outboxEvents) != 0 {
		t.Errorf("expected 0 outbox events in db, got %d", len(db.outboxEvents))
	}

	// Case B: DB error during commit
	db.failCommit = true
	merch2 := &model.Merchant{
		ID:            uuid.New(),
		BusinessName:  "Commit Fail Co",
		BusinessEmail: "commitfail@example.com",
		Status:        string(model.MerchantStatusPending),
	}

	err = db.RegisterMerchantTx(merch2, false)
	if err == nil {
		t.Fatal("expected commit error, got nil")
	}

	// Assert rollback restored DB state completely
	if len(db.merchants) != 0 {
		t.Errorf("expected 0 merchants after rollback, got %d", len(db.merchants))
	}
	if len(db.outboxEvents) != 0 {
		t.Errorf("expected 0 outbox events after rollback, got %d", len(db.outboxEvents))
	}

	// Process relay: assert zero messages published
	db.ProcessRelay(ctx, bus, cfg)
	if len(bus.GetPublished()) != 0 {
		t.Errorf("expected zero messages on bus after rollback, got %d", len(bus.GetPublished()))
	}
}

// -------------------------------------------------------------
// Test 3: Broker Outage & Recovery Test
// -------------------------------------------------------------
func TestBrokerOutageAndAutomaticRecovery(t *testing.T) {
	db := NewMockOutboxDB()
	bus := NewMockEventBus()
	ctx := context.Background()
	cfg := outbox.DefaultConfig()

	// Register merchant while broker is healthy
	merchID := uuid.New()
	_ = db.RegisterMerchantTx(&model.Merchant{
		ID:            merchID,
		BusinessName:  "Resilient Mart",
		BusinessEmail: "resilient@example.com",
		Status:        string(model.MerchantStatusPending),
	}, false)

	// Simulate broker outage
	bus.SetDown(true)

	// Publisher runs cycle while broker is down
	published, failed := db.ProcessRelay(ctx, bus, cfg)
	if published != 0 {
		t.Errorf("expected 0 published during outage, got %d", published)
	}
	if failed != 1 {
		t.Errorf("expected 1 failure during outage, got %d", failed)
	}

	// Assert event accumulates in PENDING status with incremented retry count
	for _, evt := range db.outboxEvents {
		if evt.Status != outbox.StatusPending {
			t.Errorf("expected event to remain PENDING during outage, got %s", evt.Status)
		}
		if evt.RetryCount != 1 {
			t.Errorf("expected retry_count 1, got %d", evt.RetryCount)
		}
	}

	// Add another event (Activation) during outage
	_, _ = db.ActivateMerchantTx(merchID, "admin-1", "Kyc OK", false)

	// Run publisher again during outage
	published, failed = db.ProcessRelay(ctx, bus, cfg)
	if published != 0 {
		t.Errorf("expected 0 published during continued outage, got %d", published)
	}
	if failed != 2 {
		t.Errorf("expected 2 failures during continued outage, got %d", failed)
	}

	// Broker recovers!
	bus.SetDown(false)

	// Publisher runs next cycle: both accumulated events publish successfully
	published, failed = db.ProcessRelay(ctx, bus, cfg)
	if failed != 0 {
		t.Errorf("expected 0 failures after recovery, got %d", failed)
	}
	if published != 2 {
		t.Errorf("expected 2 published events after recovery, got %d", published)
	}

	// Assert both events are now marked PUBLISHED
	for _, evt := range db.outboxEvents {
		if evt.Status != outbox.StatusPublished {
			t.Errorf("expected event status PUBLISHED, got %s", evt.Status)
		}
		if evt.PublishedAt == nil {
			t.Errorf("expected non-nil published_at timestamp")
		}
	}
}

// -------------------------------------------------------------
// Test 4: Max Retries, DLQ Routing, and Admin Replay
// -------------------------------------------------------------
func TestMaxRetriesAndDLQReplay(t *testing.T) {
	db := NewMockOutboxDB()
	bus := NewMockEventBus()
	ctx := context.Background()
	cfg := outbox.Config{
		MaxRetries: 3,
		DLQTopic:   "gocart.merchant.dlq",
	}

	merchID := uuid.New()
	_ = db.RegisterMerchantTx(&model.Merchant{
		ID:            merchID,
		BusinessName:  "DLQ Mart",
		BusinessEmail: "dlq@example.com",
		Status:        string(model.MerchantStatusPending),
	}, false)

	// Broker is permanently failing
	bus.SetDown(true)

	// 3 failed cycles exceed max retries
	for cycle := 1; cycle <= 3; cycle++ {
		db.ProcessRelay(ctx, bus, cfg)
	}

	// Assert event is marked FAILED
	for _, evt := range db.outboxEvents {
		if evt.Status != outbox.StatusFailed {
			t.Fatalf("expected event status FAILED after %d retries, got %s", cfg.MaxRetries, evt.Status)
		}
	}

	// Broker recovers, but event is FAILED so publisher ignores it
	bus.SetDown(false)
	published, _ := db.ProcessRelay(ctx, bus, cfg)
	if published != 0 {
		t.Errorf("FAILED events should not be published without requeue, published: %d", published)
	}

	// Admin executes requeue/replay
	requeued := db.RequeueFailed()
	if requeued != 1 {
		t.Fatalf("expected 1 event requeued, got %d", requeued)
	}

	// Publisher runs: event now succeeds!
	published, failed := db.ProcessRelay(ctx, bus, cfg)
	if failed != 0 || published != 1 {
		t.Errorf("expected 1 published after requeue, got published: %d, failed: %d", published, failed)
	}
}

// -------------------------------------------------------------
// Test 5: CloudEvents Payload and Attribute Validation
// -------------------------------------------------------------
func TestCloudEventsPayloadValidation(t *testing.T) {
	now := time.Now().UTC()
	merchantID := "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d"
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"

	payload := events.MerchantActivatedEvent{
		MerchantID:     merchantID,
		PreviousStatus: "PENDING",
		NewStatus:      "ACTIVE",
		ActivatedBy:    "admin-55",
		Reason:         "Passed all checks",
		ActivatedAt:    now,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeMerchantActivated, "merchant-service", merchantID, payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}
	env.TraceID = traceID

	// Validate envelope attributes
	if env.EventID == "" {
		t.Error("expected non-empty EventID")
	}
	if env.EventType != "MerchantActivated" {
		t.Errorf("expected EventType MerchantActivated, got %s", env.EventType)
	}
	if env.AggregateID != merchantID {
		t.Errorf("expected AggregateID %s, got %s", merchantID, env.AggregateID)
	}
	if env.EventVersion != "1.0" {
		t.Errorf("expected EventVersion 1.0, got %s", env.EventVersion)
	}
	if env.Source != "merchant-service" {
		t.Errorf("expected Source merchant-service, got %s", env.Source)
	}
	if env.TraceID != traceID {
		t.Errorf("expected TraceID %s, got %s", traceID, env.TraceID)
	}

	// Validate JSON marshaling and schema conformance
	bytes, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("failed to marshal JSON envelope: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("failed to parse JSON envelope: %v", err)
	}

	for _, key := range []string{"event_id", "event_type", "aggregate_id", "event_version", "source", "timestamp", "data"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("JSON output missing required CloudEvents key: %s", key)
		}
	}
}
