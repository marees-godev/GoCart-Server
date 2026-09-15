# GoCart Asynchronous Event-Bus Conventions & Architecture

This document defines the architectural standards, naming conventions, event envelope specification, producer/consumer responsibilities, and error handling policies for asynchronous messaging across GoCart services using Apache Kafka.

---

## 1. Architectural Core Principles

1. **Database Decoupling**: No service ever directly accesses another service's database. State synchronization across bounded contexts MUST be achieved via domain events.
2. **Transactional Outbox**: Services publishing events write state changes and outbox records within the same local database transaction. An outbox processor periodically dispatches pending records to Kafka brokers.
3. **At-Least-Once Delivery**: Producers publish with retry logic, and consumers MUST implement idempotent message processing.
4. **Backward Compatibility**: Event payload additions must be additive. Structural field removals or non-backward-compatible changes require a new major `event_version`.

---

## 2. Topic & Queue Naming Conventions

All Kafka topics follow a structured dot-delimited hierarchy:

`gocart.<domain>.<event-type>`

### Examples:
- `gocart.orders.order-created`
- `gocart.payments.payment-successful`
- `gocart.payments.payment-failed`
- `gocart.inventory.inventory-reserved`
- `gocart.inventory.inventory-reservation-failed`

### Retry and Dead-Letter Topics:
- **Retry Topic**: `gocart.<domain>.<event-type>.retry`
- **Dead-Letter Queue (DLQ)**: `gocart.<domain>.<event-type>.dlq`

---

## 3. Consumer Group Naming Conventions

Consumer groups allow multiple instances of a service to form a coordinated consumption cluster for a given topic:

`gocart.<consumer-service>.<topic-short-name>-group`

### Examples:
- For `inventory-service` listening to `gocart.orders.order-created`:
  `gocart.inventory-service.order-created-group`
- For `notification-service` listening to `gocart.payments.payment-successful`:
  `gocart.notification-service.payment-successful-group`

---

## 4. Standardized Event Envelope Schema

All messages published to Kafka brokers MUST be wrapped in the standard `EventEnvelope` defined in `contracts/events/envelope.go`.

```json
{
  "event_id": "c1f7b8e2-4b2a-4638-92f5-1b777a837c41",
  "event_type": "OrderCreated",
  "event_version": "1.0",
  "source": "order-service",
  "timestamp": "2026-09-15T10:15:30Z",
  "correlation_id": "order-req-84920",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "data": {
    "order_id": "78201a3b-285d-4fbc-b40b-4a572a118e6a",
    "user_id": "3194a82c-1234-4567-89ab-cdef01234567",
    "total_amount": 149.99,
    "currency": "USD",
    "items": [
      {
        "product_id": "11111111-2222-3333-4444-555555555555",
        "quantity": 2,
        "unit_price": 74.995
      }
    ]
  },
  "metadata": {
    "environment": "production",
    "schema_ref": "v1.0"
  }
}
```

---

## 5. Producer Responsibilities

1. **Envelope Formatting**: Always wrap domain payloads in `contracts/events.EventEnvelope`.
2. **Partition Keys**: Use entity identifiers (e.g., `order_id`, `user_id`) as Kafka message keys to preserve event order per entity across partitions.
3. **Correlation & Tracing**: Propagate `correlation_id` and OpenTelemetry `trace_id` headers.
4. **Outbox Guarantee**: Use the transactional outbox pattern to guarantee that business state persistence and event publishing are atomic.

---

## 6. Consumer Responsibilities & Error Handling

1. **Idempotency**: Consumers MUST check local processing records (e.g. `processed_events` table) using `event_id` before processing to prevent duplicate side-effects.
2. **Retry Mechanism**: When a transient error occurs during message handling (e.g., network timeout, database lock), the consumer retries up to `MaxRetries` using exponential backoff.
3. **Dead-Letter Queue (DLQ)**: If retries are exhausted without success, the consumer publishes the message along with failure metadata (`error_message`, `failure_timestamp`, `attempt_count`) to the corresponding `.dlq` topic and commits the offset to avoid blocking partition execution.
4. **Broker Connection Handling**: Readers perform background reconnection automatically with exponential backoff if the broker becomes temporarily unavailable.
