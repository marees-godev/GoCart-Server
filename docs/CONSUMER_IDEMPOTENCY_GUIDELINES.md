# Downstream Consumer Idempotency Guidelines

## Overview
GoCart uses the **Transactional Outbox Pattern** to achieve reliable, at-least-once event delivery from producing microservices (such as `merchant-service`, `order-service`, `store-service`) to Apache Kafka.

Because of network retries, broker rebalances, and recovery from publisher restarts, **downstream consumers may receive duplicate events**. All consuming microservices (e.g., `store-service`, `notification-service`, `auth-service`, `order-service`) **must implement idempotent message processing**.

---

## 1. Message Headers & CloudEvent Envelope

Each event emitted to Kafka contains standardized metadata headers and a CloudEvents-compliant payload envelope:

### Kafka Message Headers
| Header Key | Type | Description |
| :--- | :--- | :--- |
| `event_id` | `string` (UUID v7) | **Primary deduplication key**. Globally unique per event emission. |
| `idempotency_key` | `string` (UUID v7) | Explicit deduplication alias for downstream idempotency middleware. |
| `event_type` | `string` | Domain event type (e.g., `MerchantRegistered`, `MerchantActivated`, `MerchantSuspended`). |
| `aggregate_id` | `string` (UUID) | Partition key and Aggregate ID (`merchant_id`). Guarantees FIFO in-order delivery per merchant. |
| `source` | `string` | Originating microservice (e.g., `merchant-service`). |
| `timestamp` | `string` (RFC 3339) | Wall-clock time when event was recorded in outbox. |

### Envelope Payload Specification
```json
{
  "event_id": "018dc3f4-5678-7123-8abc-def012345678",
  "event_type": "MerchantActivated",
  "aggregate_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
  "event_version": "1.0",
  "source": "merchant-service",
  "timestamp": "2026-10-05T16:00:00Z",
  "occurred_at": "2026-10-05T16:00:00Z",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "data": {
    "merchant_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
    "previous_status": "PENDING",
    "new_status": "ACTIVE",
    "activated_by": "adm-12345",
    "reason": "KYC documentation approved",
    "activated_at": "2026-10-05T16:00:00Z"
  }
}
```

---

## 2. Deduplication Strategies for Consumers

### Pattern A: Relational Database Deduplication Log (Strongest Guarantee)
When consumer operations modify a PostgreSQL database, record the `event_id` in a dedicated `processed_events` table **inside the same database transaction** that applies the business update.

#### 1. Database Schema
```sql
CREATE TABLE IF NOT EXISTS processed_events (
    consumer_name VARCHAR(100) NOT NULL,
    event_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (consumer_name, event_id)
);
```

#### 2. Go Implementation
```go
func (h *ConsumerHandler) HandleMerchantActivated(ctx context.Context, msg kafka.Message, env *events.EventEnvelope) error {
    var payload events.MerchantActivatedEvent
    if err := env.UnmarshalData(&payload); err != nil {
        return err // Route to poison-pill DLQ if schema is corrupted
    }

    eventUUID, err := uuid.Parse(env.EventID)
    if err != nil {
        return fmt.Errorf("invalid event_id UUID: %w", err)
    }

    return h.db.WithTransaction(ctx, func(tx pgx.Tx) error {
        // Step 1: Idempotency check with ON CONFLICT DO NOTHING
        res, err := tx.Exec(ctx, `
            INSERT INTO processed_events (consumer_name, event_id, event_type, processed_at)
            VALUES ($1, $2, $3, NOW())
            ON CONFLICT (consumer_name, event_id) DO NOTHING`,
            h.consumerName, eventUUID, env.EventType,
        )
        if err != nil {
            return fmt.Errorf("failed to record processed event: %w", err)
        }

        if res.RowsAffected() == 0 {
            // Event was already processed — safely ACK without re-executing side effects
            h.logger.Info("Duplicate event skipped", "event_id", env.EventID)
            return nil
        }

        // Step 2: Apply consumer business updates (e.g., auto-enable merchant stores)
        return h.storeRepo.EnableStoresForMerchant(ctx, tx, payload.MerchantID)
    })
}
```

---

### Pattern B: Redis Atomic Idempotency Check (High Throughput / Non-Transactional Sinks)
When consumers call external APIs (e.g., third-party payment gateways, email/SMS notifications) or update caches, use Redis atomic keys with expiration.

#### Go Implementation
```go
func (h *NotificationConsumer) HandleMerchantActivated(ctx context.Context, env *events.EventEnvelope) error {
    dedupKey := fmt.Sprintf("idempotency:%s:%s", h.consumerName, env.EventID)
    
    // 24-hour TTL protects against duplicated redelivery windows while preventing memory leaks
    isNew, err := h.redisClient.SetNX(ctx, dedupKey, "PROCESSED", 24*time.Hour).Result()
    if err != nil {
        return fmt.Errorf("redis idempotency lookup failed: %w", err)
    }

    if !isNew {
        h.logger.Info("Duplicate event detected via Redis idempotency store, skipping", "event_id", env.EventID)
        return nil
    }

    // Deliver notification safely
    var payload events.MerchantActivatedEvent
    _ = env.UnmarshalData(&payload)
    return h.mailer.SendAccountActivatedEmail(ctx, payload.MerchantID)
}
```

---

### Pattern C: Domain State Machine Idempotency
If the domain model is an immutable state machine, repeated application of the same state transition should result in a harmless no-op rather than an error:

```go
func (s *StoreService) OnMerchantSuspended(ctx context.Context, merchantID string) error {
    // If merchant stores are already in suspended mode, simply return nil
    return s.repo.SetStoresSuspendedIfActive(ctx, merchantID)
}
```

---

## 3. Checklist for Consumer Implementers
- [x] Read `event_id` or `idempotency_key` from message headers or `envelope.EventID`.
- [x] Perform atomic check-and-set using either PostgreSQL `processed_events` or Redis `SetNX`.
- [x] Acknowledge message to Kafka (commit offset) only **after** the business transaction succeeds.
- [x] Do not fail or retry if duplicate event is detected — log at `INFO` level and commit offset immediately.
