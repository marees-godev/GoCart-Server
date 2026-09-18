# GoCart — Enterprise E-Commerce Microservices Platform

GoCart is a distributed, multi-vendor, event-driven e-commerce platform built in Go. Each service is fully independent and follows the database-per-service pattern, transactional outbox pattern for reliable event publishing, and 3NF database normalization.

---

## Table of Contents

- [Project Structure](#project-structure)
- [Architecture & Core Principles](#architecture--core-principles)
- [Transactional Outbox](#transactional-outbox)
  - [How It Works](#how-it-works)
  - [Outbox Schema](#outbox-schema)
  - [Adopting the Pattern in a Service](#adopting-the-pattern-in-a-service)
  - [Admin Endpoints](#admin-endpoints)
  - [Environment Variables](#environment-variables-outbox)
- [Microservices Overview](#microservices-overview)
- [Database Migrations](#database-migrations)
  - [Migration Strategy](#migration-strategy)
  - [Migration CLI Commands](#migration-cli-commands)
  - [Makefile Reference](#makefile-reference)
- [Environment Configuration](#environment-configuration)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Running Migrations](#running-migrations)
  - [Running Tests](#running-tests)
  - [Running a Service](#running-a-service)

---

## Project Structure

```text
GoCart-Server/
├── cmd/
│   └── migrate/                    # Centralized multi-database migration runner
│       └── main.go
├── pkg/                            # Shared internal libraries (logging, database, helpers)
│   └── database/                   # PostgreSQL connection pool & migrator engine (pgx/v5)
│       ├── config.go
│       ├── migrator.go
│       ├── postgres.go
│       └── postgres_test.go
├── services/                       # Autonomous microservices
│   ├── auth-service/               # Authentication & token management
│   ├── cart-service/               # Shopping cart & session items
│   ├── category-service/           # Hierarchical product categories
│   ├── delivery-service/           # Shipments, couriers & tracking updates
│   ├── inventory-service/          # Stock reservations & inventory ledger
│   ├── merchant-service/           # Merchant onboarding & approval workflows
│   ├── notification-service/       # Multi-channel notifications & templates
│   ├── order-service/              # Orders, snapshots & checkout saga state
│   ├── payment-service/            # Payment gateway, refunds & webhooks
│   ├── product-service/            # Products, SKUs & image gallery
│   ├── rating-service/             # Customer reviews & verified ratings
│   ├── return-service/             # Order returns & refund triggers
│   ├── store-service/              # Vendor storefronts & moderation
│   └── user-service/               # User profiles, addresses & countries
├── Makefile                        # Central developer automation tasks
├── docker-compose.yml              # Local infrastructure orchestration
└── README.md
```

---

## Architecture & Core Principles

1. **Database per Service**: Every service owns its database exclusively. Direct cross-database access is prohibited.
2. **Transactional Outbox**: Every state-changing service maintains an `outbox_events` table within the same transaction to guarantee reliable event dispatch to message brokers (e.g., Kafka).
3. **Normalized Schemas & Lookups**:
   - Reference entities (`countries`, `payment_methods`, `delivery_partners`, `notification_channels`) use normalized lookup tables so frontends consume standard UUIDs.
   - High data integrity with `CHECK` constraints, `TIMESTAMPTZ` audit timestamps, and `gen_random_uuid()` primary keys.
4. **Historical Immutability**: `order_items` stores historical snapshots of prices, product names, taxes, and discounts at the moment of checkout, ensuring product catalog changes never alter historic order receipts.
5. **No Redundant Indexes**: Primary keys and unique constraints automatically generate unique b-tree indexes in PostgreSQL; duplicate explicit index declarations are avoided to maximize write throughput.

---

## Transactional Outbox

GoCart uses the **Transactional Outbox** pattern as the standard mechanism for reliably publishing domain events. It eliminates the "DB updated but event lost" failure scenario by making state changes and event records atomic.

The shared implementation lives in [`pkg/outbox/`](file:///d:/projects/GoCart-Server/pkg/outbox/) and can be adopted by any service.

### How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│  Service (e.g. order-service)                                   │
│                                                                 │
│  BEGIN TRANSACTION                                              │
│    INSERT INTO orders (...)          ← business state change    │
│    INSERT INTO outbox_events (...)   ← event record, same tx    │
│  COMMIT                                                         │
│                                                                 │
│  Either both succeed or both roll back. Never one without       │
│  the other.                                                     │
└─────────────────────────────────────────────────────────────────┘
                          │
                          │  (every 2 seconds)
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│  Outbox Publisher (background goroutine)                        │
│                                                                 │
│  SELECT ... FROM outbox_events                                  │
│    WHERE status = 'PENDING'                                     │
│    FOR UPDATE SKIP LOCKED   ← safe for multiple instances       │
│                                                                 │
│  For each event:                                                │
│    producer.Publish(topic, payload)  → Kafka                    │
│      ✓ success  →  status = 'PUBLISHED', published_at = NOW()   │
│      ✗ failure  →  retry_count++                                │
│                      if retry_count >= MaxRetries               │
│                        status = 'FAILED'                        │
│                      else                                       │
│                        status = 'PENDING'  (retry next poll)    │
└─────────────────────────────────────────────────────────────────┘
```

**Key guarantees:**

| Guarantee | Mechanism |
| :--- | :--- |
| Event never lost if DB write succeeds | Business row + outbox row committed in the **same transaction** |
| Event never published without broker ACK | `PUBLISHED` status set only after `WriteMessages` returns `nil` |
| No duplicate publishing across instances | `FOR UPDATE SKIP LOCKED` — locked rows are invisible to other publisher goroutines |
| Survives service restarts | `PENDING` rows persist in Postgres; publisher resumes on next start |
| Failed events are retried | Event stays/resets to `PENDING` until `retry_count >= MaxRetries` |

---

### Outbox Schema

Every service that publishes events must have this table (included in each service's `000001_init.sql`):

```sql
CREATE TABLE outbox_events (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(100)  NOT NULL,   -- e.g. 'order'
    aggregate_id   VARCHAR(255)  NOT NULL,   -- e.g. the order UUID
    event_type     VARCHAR(100)  NOT NULL,   -- e.g. 'OrderCreated'
    payload        JSONB         NOT NULL,   -- full EventEnvelope JSON
    topic          VARCHAR(255)  NOT NULL DEFAULT '', -- Kafka topic
    status         VARCHAR(50)   NOT NULL DEFAULT 'PENDING',
    retry_count    INT           NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    published_at   TIMESTAMPTZ
);

CREATE INDEX idx_outbox_pending_created
    ON outbox_events (created_at ASC)
    WHERE status = 'PENDING'; -- partial index, only scans relevant rows
```

Status lifecycle: `PENDING` → `PUBLISHED` (happy path) or `PENDING` → `FAILED` (max retries exceeded).

---

### Adopting the Pattern in a Service

**1. Create the outbox store and publisher in `main.go`:**

```go
outboxStore     := outbox.NewStore()
outboxPublisher := outbox.NewPublisher(db.Pool, kafkaProducer, outboxStore, outbox.Config{
    PollInterval: cfg.Outbox.PollInterval, // default 2s
    BatchSize:    cfg.Outbox.BatchSize,    // default 50
    MaxRetries:   cfg.Outbox.MaxRetries,   // default 5
})
go outboxPublisher.Start(ctx) // stops cleanly on SIGTERM/SIGINT
```

**2. In your repository, write both records in the same transaction:**

```go
func (r *OrderRepo) CreateOrderWithOutbox(ctx context.Context, ...) error {
    return r.db.WithTransaction(ctx, func(tx pgx.Tx) error {
        // Step 1: persist the business record
        if err := insertOrder(ctx, tx, order); err != nil {
            return err
        }

        // Step 2: persist the outbox event — same tx, guaranteed atomic
        payload, _ := envelope.Marshal()
        return r.outboxStore.Insert(ctx, tx, &outbox.Event{
            AggregateType: "order",
            AggregateID:   order.ID.String(),
            EventType:     envelope.EventType,
            Payload:       payload,
            Topic:         "orders.created",
        })
    })
}
```

If the transaction rolls back for any reason (validation error, DB constraint, etc.), no outbox row is created. The publisher never sees the event. This is structurally impossible to get wrong.

---

### Admin Endpoints

Each service exposes two operational endpoints for managing failed events:

#### `GET /admin/outbox/stats`

Returns event counts grouped by status. Useful for monitoring.

```bash
curl http://localhost:8500/admin/outbox/stats
```
```json
{ "PENDING": 0, "PUBLISHED": 1402, "FAILED": 3 }
```

#### `POST /admin/outbox/requeue`

Resets `FAILED` events back to `PENDING` so the publisher retries them. All filters are optional — omit both to requeue every failed event.

```bash
# Requeue all failed events
curl -X POST http://localhost:8500/admin/outbox/requeue \
  -H 'Content-Type: application/json' \
  -d '{}'

# Requeue only failed OrderCreated events
curl -X POST http://localhost:8500/admin/outbox/requeue \
  -H 'Content-Type: application/json' \
  -d '{"aggregate_type": "order", "event_type": "OrderCreated"}'
```
```json
{ "requeued": 3, "filters": { "aggregate_type": "order", "event_type": "OrderCreated" } }
```

> **Note:** These endpoints have no auth middleware by default. Gate them behind an API key or JWT middleware before exposing in production.

---

### Environment Variables (Outbox)

Add these to your service's `.env`:

```env
# Kafka broker
KAFKA_BROKERS=localhost:9092      # comma-separated for multiple brokers
KAFKA_CLIENT_ID=order-service

# Outbox publisher tuning
OUTBOX_POLL_INTERVAL=2s           # how often to check for PENDING events
OUTBOX_BATCH_SIZE=50              # max events processed per cycle
OUTBOX_MAX_RETRIES=5             # attempts before an event is marked FAILED
```

---

## Microservices Overview

| Microservice | Port | Key Entities | Outbox Support |
| :--- | :---: | :--- | :---: |
| **Auth Service** | `9451` | `auth_credentials`, `refresh_tokens` | Yes |
| **User Service** | `5050` | `users`, `user_addresses`, `countries` | Yes |
| **Product Service** | `7070` | `products`, `product_images` | Yes |
| **Category Service** | `7080` | `categories` (hierarchical) | Yes |
| **Store Service** | `7500` | `stores` | Yes |
| **Merchant Service** | `7600` | `merchants` | Yes |
| **Cart Service** | `8181` | `carts`, `cart_items` | Yes |
| **Inventory Service** | `8300` | `inventories`, `inventory_reservations`, `inventory_transactions` | Yes |
| **Order Service** | `8500` | `orders`, `order_items` | Yes |
| **Payment Service** | `4242` | `payments`, `payment_methods`, `payment_webhooks`, `refunds` | Yes |
| **Delivery Service** | `9050` | `deliveries`, `delivery_partners`, `delivery_tracking_updates` | Yes |
| **Return Service** | `9150` | `returns` | Yes |
| **Rating Service** | `6066` | `ratings` | Yes |
| **Notification Service** | `9099` | `notifications`, `notification_channels`, `notification_templates` | Yes |

---

## Database Migrations

### Migration Strategy
- Each microservice has its own migration directory at `services/<service-name>/migrations/`.
- Migrations use a single `.sql` file per version (e.g., `000001_init.sql`).
- The migration engine tracks applied versions in a dedicated `schema_migrations` table per database.
- Supports **forward migrations**, **full drops**, and **clean database resets**.

### Migration CLI Commands

Run the migration tool using `go run cmd/migrate/main.go`:

```pwsh
# Run pending migrations for a specific service
go run cmd/migrate/main.go -service=auth
go run cmd/migrate/main.go -service=user

# Run pending migrations for ALL services
go run cmd/migrate/main.go -service=all

# Drop all tables in a service's database
go run cmd/migrate/main.go -service=auth -action=drop

# Reset (Drop all tables + Apply fresh migrations) for a specific service
go run cmd/migrate/main.go -service=user -action=reset

# Reset (Drop all tables + Apply fresh migrations) for ALL services
go run cmd/migrate/main.go -service=all -action=reset
```

#### Supported Service Names & Aliases
The `-service` flag accepts singular, plural, or full service names:
`auth`, `user` (or `users`, `user-service`), `merchant`, `store`, `category` (or `categories`), `product` (or `products`), `inventory` (or `inventories`), `cart`, `order` (or `orders`), `payment` (or `payments`), `delivery` (or `deliveries`), `return` (or `returns`), `rating` (or `ratings`), `notification` (or `notifications`), or `all`.

---

### Makefile Reference

The root [Makefile](file:///d:/projects/GoCart-Server/Makefile) simplifies common development, docker, and database tasks:

| Command | Description | Example |
| :--- | :--- | :--- |
| `make build` | Builds all service and utility binaries to `./bin/` | `make build` |
| `make test` | Runs unit and integration tests across packages | `make test` |
| `make docker-up` | Launches full stack (infrastructure + microservices) via Docker Compose | `make docker-up` |
| `make docker-down` | Stops all Docker containers | `make docker-down` |
| `make docker-build` | Rebuilds all service Docker container images | `make docker-build` |
| `make infra-up` | Launches local infrastructure only (PostgreSQL, Redis, Kafka) | `make infra-up` |
| `make infra-down` | Stops local infrastructure containers | `make infra-down` |
| `make migrate` | Applies pending migrations for a specific service | `make migrate SERVICE=auth` |
| `make migrate-up` | Alias for `make migrate` | `make migrate-up SERVICE=order` |
| `make migrate-drop` | Drops all tables and resets the public schema for a service | `make migrate-drop SERVICE=auth` |
| `make migrate-reset` | Drops all tables and reapplies migrations from scratch for a service | `make migrate-reset SERVICE=product` |
| `make migrate-all` | Applies pending migrations across **all 14 services** | `make migrate-all` |
| `make migrate-drop-all` | Drops all tables across **all 14 services** | `make migrate-drop-all` |
| `make migrate-reset-all` | Drops and reapplies fresh migrations across **all 14 services** | `make migrate-reset-all` |
| `make run-auth` | Starts the Auth Service server | `make run-auth` |

---

## Environment Configuration

Each service manages its own `.env` configuration file in its respective directory:

```env
# services/auth-service/.env
PORT=9451
DATABASE_URL=postgresql://postgres:password@localhost:5432/gocart_auth?sslmode=disable
DB_MAX_CONNS=25
DB_MIN_CONNS=2
DB_AUTO_MIGRATE=true
DB_MIGRATIONS_PATH=./migrations
```

Alternatively, root `.env` can define service database URLs using uppercase prefixes:
```env
AUTH_DATABASE_URL=postgresql://...
USERS_DATABASE_URL=postgresql://...
PRODUCTS_DATABASE_URL=postgresql://...
...
```

---

## Getting Started

### Prerequisites
- **Docker & Docker Compose**: `20.10+` / `v2+` (Required for local infrastructure and containerized services)
- **Go**: `1.24+` (Optional, for native local Go development)
- **Make** (optional, recommended for CLI shortcuts)

### Docker Local Development (Recommended)

To start PostgreSQL, Redis, and Kafka in containers for local development:
```bash
make infra-up
```

To start all infrastructure dependencies and all 14 microservices in containers:
```bash
make docker-up
```

For comprehensive details on local setup, ports, and troubleshooting, refer to [Local Development Guide](file:///d:/Projects/GoCart-Server/docs/local-development.md).

### Running Migrations
To initialize all microservice databases from scratch:
```bash
make migrate-reset-all
```

### Running Tests
Execute test suites across the repository:
```bash
make test
```

### Running a Service
Start any microservice directly:
```pwsh
go run services/auth-service/cmd/server/main.go
# or
make run-auth
```
