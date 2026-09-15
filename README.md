# GoCart — Enterprise E-Commerce Microservices Platform

GoCart is a distributed, multi-vendor, event-driven e-commerce platform built in Go. Each service is fully independent and follows the database-per-service pattern, transactional outbox pattern for reliable event publishing, and 3NF database normalization.

---

## Table of Contents

- [Project Structure](#project-structure)
- [Architecture & Core Principles](#architecture--core-principles)
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

The root [Makefile](file:///d:/projects/GoCart-Server/Makefile) simplifies common development and database tasks:

| Command | Description | Example |
| :--- | :--- | :--- |
| `make build` | Builds all service and utility binaries to `./bin/` | `make build` |
| `make test` | Runs unit and integration tests across packages | `make test` |
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
- **Go**: `1.22+` (or `1.26+`)
- **PostgreSQL**: `15+` (local or hosted e.g. Supabase)
- **Make** (optional, recommended for CLI shortcuts)

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
