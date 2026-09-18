package kafka_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/kafka"
	segmentio "github.com/segmentio/kafka-go"
)

func TestDefaultConfig(t *testing.T) {
	cfg := kafka.DefaultConfig()
	if len(cfg.Brokers) != 1 || cfg.Brokers[0] != "localhost:9092" {
		t.Errorf("expected broker localhost:9092, got %v", cfg.Brokers)
	}
	if cfg.ClientID != "gocart-service" {
		t.Errorf("expected client ID gocart-service, got %s", cfg.ClientID)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected 3 retries, got %d", cfg.MaxRetries)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	os.Setenv("KAFKA_BROKERS", "broker1:9092,broker2:9092")
	os.Setenv("ORDER_KAFKA_CLIENT_ID", "order-service-client")
	defer func() {
		os.Unsetenv("KAFKA_BROKERS")
		os.Unsetenv("ORDER_KAFKA_CLIENT_ID")
	}()

	cfg, err := kafka.LoadConfigFromEnv("ORDER")
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if len(cfg.Brokers) != 2 || cfg.Brokers[0] != "broker1:9092" || cfg.Brokers[1] != "broker2:9092" {
		t.Errorf("expected brokers [broker1:9092 broker2:9092], got %v", cfg.Brokers)
	}
	if cfg.ClientID != "order-service-client" {
		t.Errorf("expected client ID order-service-client, got %s", cfg.ClientID)
	}
}

func TestRetryHandlerExecution(t *testing.T) {
	cfg := kafka.DefaultConfig()
	cfg.MaxRetries = 3
	cfg.RetryInterval = 5 * time.Millisecond

	retryHandler := kafka.NewRetryHandler(cfg, nil)

	attempts := 0
	handler := func(ctx context.Context, envelope *events.EventEnvelope) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary processing error")
		}
		return nil
	}

	env, err := events.NewEventEnvelope("TestEvent", "test-source", map[string]string{"foo": "bar"})
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}

	msg := segmentio.Message{
		Topic: "test.topic",
		Key:   []byte("test-key"),
	}

	err = retryHandler.ExecuteWithRetry(context.Background(), msg, env, handler)
	if err != nil {
		t.Fatalf("expected handler to succeed after 3 attempts, got error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryHandlerExhaustion(t *testing.T) {
	cfg := kafka.DefaultConfig()
	cfg.MaxRetries = 2
	cfg.RetryInterval = 5 * time.Millisecond

	retryHandler := kafka.NewRetryHandler(cfg, nil)

	attempts := 0
	handler := func(ctx context.Context, envelope *events.EventEnvelope) error {
		attempts++
		return errors.New("persistent error")
	}

	env, _ := events.NewEventEnvelope("TestEvent", "test-source", map[string]string{"key": "value"})
	msg := segmentio.Message{Topic: "test.topic"}

	err := retryHandler.ExecuteWithRetry(context.Background(), msg, env, handler)
	if err == nil {
		t.Fatal("expected error on retry exhaustion, got nil")
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestHeaderHelpers(t *testing.T) {
	headers := []segmentio.Header{
		{Key: "event_type", Value: []byte("OrderCreated")},
		{Key: "x-attempt-count", Value: []byte("3")},
	}

	if val := kafka.HeaderValue(headers, "event_type"); val != "OrderCreated" {
		t.Errorf("expected OrderCreated, got %s", val)
	}
	if val := kafka.IntHeaderValue(headers, "x-attempt-count", 0); val != 3 {
		t.Errorf("expected 3, got %d", val)
	}
	if val := kafka.IntHeaderValue(headers, "non-existent", 10); val != 10 {
		t.Errorf("expected default 10, got %d", val)
	}
}
