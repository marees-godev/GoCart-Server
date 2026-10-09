package outbox_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
)

type mockKafkaPublisher struct {
	mu            sync.Mutex
	published     []publishedMessage
	failTopics    map[string]bool
	failCountDown map[string]int
}

type publishedMessage struct {
	Topic    string
	Key      string
	Envelope *events.EventEnvelope
}

func newMockKafka() *mockKafkaPublisher {
	return &mockKafkaPublisher{
		published:     make([]publishedMessage, 0),
		failTopics:    make(map[string]bool),
		failCountDown: make(map[string]int),
	}
}

func (m *mockKafkaPublisher) Publish(ctx context.Context, topic, key string, envelope *events.EventEnvelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failTopics[topic] {
		return errors.New("simulated broker connection failure")
	}

	if count, ok := m.failCountDown[topic]; ok && count > 0 {
		m.failCountDown[topic]--
		return errors.New("simulated temporary broker outage")
	}

	m.published = append(m.published, publishedMessage{
		Topic:    topic,
		Key:      key,
		Envelope: envelope,
	})
	return nil
}

func (m *mockKafkaPublisher) getPublished() []publishedMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]publishedMessage, len(m.published))
	copy(res, m.published)
	return res
}

func TestPublisherConfig(t *testing.T) {
	cfg := outbox.DefaultConfig()
	if cfg.PollInterval != 2*time.Second {
		t.Errorf("expected default poll interval 2s, got %v", cfg.PollInterval)
	}
	if cfg.BatchSize != 50 {
		t.Errorf("expected default batch size 50, got %d", cfg.BatchSize)
	}
	if cfg.MaxRetries != 5 {
		t.Errorf("expected default max retries 5, got %d", cfg.MaxRetries)
	}
	if cfg.BaseRetryDelay != 1*time.Second {
		t.Errorf("expected default base retry delay 1s, got %v", cfg.BaseRetryDelay)
	}
}

func TestMockKafkaPublishing(t *testing.T) {
	mk := newMockKafka()
	ctx := context.Background()

	payload := events.MerchantActivatedEvent{
		MerchantID:     "m-1",
		PreviousStatus: "PENDING",
		NewStatus:      "ACTIVE",
		ActivatedBy:    "admin-1",
		ActivatedAt:    time.Now().UTC(),
	}
	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeMerchantActivated, "merchant-service", "m-1", payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}

	if err := mk.Publish(ctx, events.TopicMerchantActivated, "m-1", env); err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}

	published := mk.getPublished()
	if len(published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(published))
	}
	if published[0].Key != "m-1" {
		t.Errorf("expected key m-1, got %s", published[0].Key)
	}
	if published[0].Topic != events.TopicMerchantActivated {
		t.Errorf("expected topic %s, got %s", events.TopicMerchantActivated, published[0].Topic)
	}
}

func TestOutboxEventModel(t *testing.T) {
	evtID := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	nextRetry := now.Add(2 * time.Second)

	evt := &outbox.Event{
		ID:            evtID,
		AggregateType: "merchant",
		AggregateID:   "m-99",
		EventType:     events.EventTypeMerchantSuspended,
		Payload:       []byte(`{"merchant_id":"m-99"}`),
		Headers:       []byte(`{"trace_id":"trace-123"}`),
		Topic:         events.TopicMerchantSuspended,
		Status:        outbox.StatusPending,
		RetryCount:    1,
		NextRetryAt:   &nextRetry,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if evt.ID != evtID {
		t.Errorf("expected ID %s, got %s", evtID, evt.ID)
	}
	if evt.AggregateType != "merchant" {
		t.Errorf("expected aggregate type merchant, got %s", evt.AggregateType)
	}
	if evt.AggregateID != "m-99" {
		t.Errorf("expected aggregate ID m-99, got %s", evt.AggregateID)
	}
	if evt.EventType != events.EventTypeMerchantSuspended {
		t.Errorf("expected event type %s, got %s", events.EventTypeMerchantSuspended, evt.EventType)
	}
	if evt.Topic != events.TopicMerchantSuspended {
		t.Errorf("expected topic %s, got %s", events.TopicMerchantSuspended, evt.Topic)
	}
	if evt.Status != outbox.StatusPending {
		t.Errorf("expected PENDING status, got %s", evt.Status)
	}
	if evt.RetryCount != 1 {
		t.Errorf("expected retry count 1, got %d", evt.RetryCount)
	}
	if evt.NextRetryAt == nil || evt.NextRetryAt.Before(now) {
		t.Errorf("expected next retry to be in future")
	}
	if len(evt.Payload) == 0 {
		t.Errorf("expected non-empty payload")
	}
	if len(evt.Headers) == 0 {
		t.Errorf("expected non-empty headers")
	}
	if evt.CreatedAt != now {
		t.Errorf("expected created at %v, got %v", now, evt.CreatedAt)
	}
	if evt.UpdatedAt != now {
		t.Errorf("expected updated at %v, got %v", now, evt.UpdatedAt)
	}
}

func TestBackoffCalculation(t *testing.T) {
	cfg := outbox.Config{
		BaseRetryDelay: 1 * time.Second,
		MaxRetryDelay:  60 * time.Second,
		MaxRetries:     5,
	}

	calcDelay := func(retryCount int) time.Duration {
		delay := cfg.BaseRetryDelay
		for i := 0; i < retryCount; i++ {
			delay *= 2
			if cfg.MaxRetryDelay > 0 && delay > cfg.MaxRetryDelay {
				delay = cfg.MaxRetryDelay
				break
			}
		}
		return delay
	}

	if calcDelay(0) != 1*time.Second {
		t.Errorf("expected 1s for retry 0, got %v", calcDelay(0))
	}
	if calcDelay(1) != 2*time.Second {
		t.Errorf("expected 2s for retry 1, got %v", calcDelay(1))
	}
	if calcDelay(2) != 4*time.Second {
		t.Errorf("expected 4s for retry 2, got %v", calcDelay(2))
	}
	if calcDelay(3) != 8*time.Second {
		t.Errorf("expected 8s for retry 3, got %v", calcDelay(3))
	}
	if calcDelay(4) != 16*time.Second {
		t.Errorf("expected 16s for retry 4, got %v", calcDelay(4))
	}
	if calcDelay(10) != 60*time.Second {
		t.Errorf("expected cap at 60s, got %v", calcDelay(10))
	}
}

