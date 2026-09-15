package kafka

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	segmentio "github.com/segmentio/kafka-go"
)

// RetryHandler manages backoff retries and Dead-Letter Queue (DLQ) routing.
type RetryHandler struct {
	cfg      Config
	producer *Producer
}

// NewRetryHandler creates a new RetryHandler instance.
func NewRetryHandler(cfg Config, producer *Producer) *RetryHandler {
	return &RetryHandler{
		cfg:      cfg,
		producer: producer,
	}
}

// ExecuteWithRetry executes a handler function, retrying up to cfg.MaxRetries times.
// If retries fail, it routes the message to the DLQ topic.
func (r *RetryHandler) ExecuteWithRetry(ctx context.Context, msg segmentio.Message, envelope *events.EventEnvelope, handler HandlerFunc) error {
	maxRetries := r.cfg.MaxRetries
	if maxRetries < 1 {
		maxRetries = 1
	}

	var lastErr error
	backoff := r.cfg.RetryInterval

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := handler(ctx, envelope)
		if err == nil {
			return nil
		}
		lastErr = err

		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				backoff *= 2 // Exponential backoff
			}
		}
	}

	// All retries exhausted: Publish to DLQ if producer is available
	if r.producer != nil {
		dlqErr := r.PublishToDLQ(ctx, msg.Topic, string(msg.Key), msg.Value, lastErr, msg.Headers)
		if dlqErr != nil {
			return fmt.Errorf("handler failed (%v) and DLQ publish failed: %w", lastErr, dlqErr)
		}
	}

	return fmt.Errorf("message processing failed after %d attempts: %w", maxRetries, lastErr)
}

// PublishToDLQ publishes a failed message to `<topic>.dlq` with failure headers.
func (r *RetryHandler) PublishToDLQ(ctx context.Context, originalTopic, key string, value []byte, processErr error, originalHeaders []segmentio.Header) error {
	if r.producer == nil {
		return fmt.Errorf("producer unavailable for DLQ publishing")
	}

	dlqTopic := fmt.Sprintf("%s%s", originalTopic, r.cfg.DLQSuffix)
	attemptCount := IntHeaderValue(originalHeaders, "x-attempt-count", r.cfg.MaxRetries) + 1

	headers := []segmentio.Header{
		{Key: "x-original-topic", Value: []byte(originalTopic)},
		{Key: "x-error-message", Value: []byte(processErr.Error())},
		{Key: "x-failure-timestamp", Value: []byte(time.Now().UTC().Format(time.RFC3339))},
		{Key: "x-attempt-count", Value: []byte(strconv.Itoa(attemptCount))},
	}

	// Preserve non-failure headers
	for _, h := range originalHeaders {
		if h.Key != "x-error-message" && h.Key != "x-failure-timestamp" && h.Key != "x-attempt-count" {
			headers = append(headers, h)
		}
	}

	dlqMsg := segmentio.Message{
		Topic:   dlqTopic,
		Key:     []byte(key),
		Value:   value,
		Headers: headers,
		Time:    time.Now().UTC(),
	}

	return r.producer.writer.WriteMessages(ctx, dlqMsg)
}
