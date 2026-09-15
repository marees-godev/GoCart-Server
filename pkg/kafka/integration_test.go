package kafka_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/kafka"
)

func TestLiveKafkaPublishAndConsume(t *testing.T) {
	cfg := kafka.DefaultConfig()
	cfg.Brokers = []string{"localhost:9092"}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Verify Kafka broker is UP
	health := kafka.CheckHealth(ctx, cfg)
	if health.Status != "UP" {
		t.Skipf("Skipping live integration test: Kafka broker not reachable at %v (%s)", cfg.Brokers, health.Error)
	}

	producer := kafka.NewProducer(cfg)
	defer producer.Close()

	topic := "gocart.test.order-created"
	orderID := "live-test-order-999"

	payload := events.OrderCreatedEvent{
		OrderID:     orderID,
		UserID:      "usr-live-100",
		TotalAmount: 250.00,
		Currency:    "USD",
		Status:      "CREATED",
		CreatedAt:   time.Now().UTC(),
	}

	// 1. Publish Event
	_, err := producer.PublishEvent(ctx, topic, orderID, events.EventTypeOrderCreated, "test-suite", payload)
	if err != nil {
		t.Fatalf("failed to publish live event: %v", err)
	}

	// 2. Consume Event
	consumerCfg := kafka.ConsumerConfig{
		GroupID:     "gocart.test-suite.order-created-group",
		Topics:      []string{topic},
		StartOffset: -2, // FirstOffset
	}

	consumer := kafka.NewConsumer(cfg, consumerCfg, producer)
	defer consumer.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	var receivedOrderID string

	go func() {
		_ = consumer.Start(ctx, func(handlerCtx context.Context, env *events.EventEnvelope) error {
			if env.EventType == events.EventTypeOrderCreated {
				var evt events.OrderCreatedEvent
				if err := env.UnmarshalData(&evt); err == nil && evt.OrderID == orderID {
					receivedOrderID = evt.OrderID
					wg.Done()
					cancel() // Stop consumer loop
				}
			}
			return nil
		})
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if receivedOrderID != orderID {
			t.Errorf("expected received OrderID %s, got %s", orderID, receivedOrderID)
		} else {
			t.Logf("Successfully produced and consumed live Kafka event for Order ID: %s", receivedOrderID)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("timeout waiting to consume event from live Kafka broker")
	}
}
