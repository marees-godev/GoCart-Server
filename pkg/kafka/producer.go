package kafka

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	segmentio "github.com/segmentio/kafka-go"
)

// Producer handles publishing domain events to Kafka topics.
type Producer struct {
	cfg    Config
	writer *segmentio.Writer
}

// NewProducer initializes a new Kafka Producer with the provided configuration.
func NewProducer(cfg Config) *Producer {
	writer := &segmentio.Writer{
		Addr:                   segmentio.TCP(cfg.Brokers...),
		Balancer:               &segmentio.LeastBytes{},
		MaxAttempts:            cfg.MaxRetries,
		BatchTimeout:           10 * time.Millisecond,
		Async:                  false,
		AllowAutoTopicCreation: true,
	}

	return &Producer{
		cfg:    cfg,
		writer: writer,
	}
}

// Publish sends an EventEnvelope to a specified topic.
func (p *Producer) Publish(ctx context.Context, topic, key string, envelope *events.EventEnvelope) error {
	if envelope == nil {
		return fmt.Errorf("cannot publish nil envelope")
	}

	payloadBytes, err := envelope.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal envelope: %w", err)
	}

	msg := segmentio.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: payloadBytes,
		Headers: []segmentio.Header{
			{Key: "event_id", Value: []byte(envelope.EventID)},
			{Key: "event_type", Value: []byte(envelope.EventType)},
			{Key: "source", Value: []byte(envelope.Source)},
			{Key: "timestamp", Value: []byte(envelope.Timestamp.Format(time.RFC3339))},
		},
		Time: envelope.Timestamp,
	}

	var lastErr error
	maxRetries := p.cfg.MaxRetries
	if maxRetries < 1 {
		maxRetries = 1
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = p.writer.WriteMessages(ctx, msg)
		if err == nil {
			return nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(p.cfg.RetryInterval):
		}
	}

	return fmt.Errorf("failed to publish message to topic %s after %d attempts: %w", topic, maxRetries, lastErr)
}

// PublishEvent constructs an EventEnvelope from a payload and publishes it to the specified topic.
func (p *Producer) PublishEvent(ctx context.Context, topic, key, eventType, source string, payload interface{}) (*events.EventEnvelope, error) {
	envelope, err := events.NewEventEnvelope(eventType, source, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create event envelope: %w", err)
	}

	if err := p.Publish(ctx, topic, key, envelope); err != nil {
		return nil, err
	}

	return envelope, nil
}

// Ping checks broker connectivity by dialing the first configured broker address.
func (p *Producer) Ping(ctx context.Context) error {
	if len(p.cfg.Brokers) == 0 {
		return fmt.Errorf("no kafka brokers configured")
	}

	dialer := &net.Dialer{
		Timeout: p.cfg.ConnectTimeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", p.cfg.Brokers[0])
	if err != nil {
		return fmt.Errorf("failed to connect to kafka broker %s: %w", p.cfg.Brokers[0], err)
	}
	_ = conn.Close()
	return nil
}

// Close gracefully closes the underlying Kafka writer.
func (p *Producer) Close() error {
	if p.writer != nil {
		return p.writer.Close()
	}
	return nil
}

// HeaderValue is a helper to get header values from segmentio Kafka messages.
func HeaderValue(headers []segmentio.Header, key string) string {
	for _, h := range headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

// IntHeaderValue is a helper to parse integer headers (e.g. retry counts).
func IntHeaderValue(headers []segmentio.Header, key string, defaultValue int) int {
	val := HeaderValue(headers, key)
	if val == "" {
		return defaultValue
	}
	if n, err := strconv.Atoi(val); err == nil {
		return n
	}
	return defaultValue
}
