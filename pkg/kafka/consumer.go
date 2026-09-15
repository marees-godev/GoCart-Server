package kafka

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	segmentio "github.com/segmentio/kafka-go"
)

// HandlerFunc is the signature for event message handling logic.
type HandlerFunc func(ctx context.Context, envelope *events.EventEnvelope) error

// ConsumerConfig holds options for a specific Consumer instance.
type ConsumerConfig struct {
	GroupID     string
	Topics      []string
	MinBytes    int
	MaxBytes    int
	StartOffset int64
}

// Consumer subscribes to Kafka topics and dispatches events to registered handlers.
type Consumer struct {
	cfg        Config
	consumerCfg ConsumerConfig
	reader     *segmentio.Reader
	producer   *Producer
	retry      *RetryHandler
	mu         sync.Mutex
	running    bool
}

// NewConsumer initializes a Kafka Consumer configured for consumer groups.
func NewConsumer(cfg Config, consumerCfg ConsumerConfig, producer *Producer) *Consumer {
	if consumerCfg.MinBytes == 0 {
		consumerCfg.MinBytes = 10e3 // 10KB
	}
	if consumerCfg.MaxBytes == 0 {
		consumerCfg.MaxBytes = 10e6 // 10MB
	}
	if consumerCfg.StartOffset == 0 {
		consumerCfg.StartOffset = segmentio.FirstOffset
	}

	readerConfig := segmentio.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        consumerCfg.GroupID,
		GroupTopics:    consumerCfg.Topics,
		MinBytes:       consumerCfg.MinBytes,
		MaxBytes:       consumerCfg.MaxBytes,
		StartOffset:    consumerCfg.StartOffset,
		CommitInterval: 1 * time.Second,
	}

	reader := segmentio.NewReader(readerConfig)
	retry := NewRetryHandler(cfg, producer)

	return &Consumer{
		cfg:         cfg,
		consumerCfg: consumerCfg,
		reader:      reader,
		producer:    producer,
		retry:       retry,
	}
}

// Start begins fetching and processing messages until context is canceled.
func (c *Consumer) Start(ctx context.Context, handler HandlerFunc) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("consumer is already running")
	}
	c.running = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return ctx.Err()
			}
			// Connection / transient error fetching message, backoff before retry
			time.Sleep(c.cfg.RetryInterval)
			continue
		}

		if err := c.processMessage(ctx, msg, handler); err != nil {
			// Log error; retry handler handles DLQ dispatch if max retries exceeded
		}

		// Commit offset after processing/handling
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			if errors.Is(err, context.Canceled) {
				return ctx.Err()
			}
		}
	}
}

func (c *Consumer) processMessage(ctx context.Context, msg segmentio.Message, handler HandlerFunc) error {
	envelope, err := events.UnmarshalEnvelope(msg.Value)
	if err != nil {
		// Non-recoverable payload format error: route directly to DLQ
		return c.retry.PublishToDLQ(ctx, msg.Topic, string(msg.Key), msg.Value, fmt.Errorf("invalid envelope format: %w", err), msg.Headers)
	}

	// Execute handler with retry support
	return c.retry.ExecuteWithRetry(ctx, msg, envelope, handler)
}

// Close closes the underlying Kafka reader.
func (c *Consumer) Close() error {
	if c.reader != nil {
		return c.reader.Close()
	}
	return nil
}
