# Local Development Environment Guide

This document describes how to start and manage the local development environment for GoCart using Docker and Docker Compose.

## Overview

The local environment includes:
- **PostgreSQL 16**: Multi-database instance automatically initialized with separate databases for all 14 microservices via `scripts/init-local-dbs.sql`.
- **Redis 7**: In-memory data store for caching and session management.
- **Apache Kafka**: Message broker operating in KRaft mode for event-driven architecture.
- **Go Microservices**: 14 independent Go services (`auth-service`, `user-service`, `cart-service`, etc.).

---

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop) or Docker Engine with Docker Compose v2+
- [Go 1.24+](https://go.dev/dl/) (optional, for running/testing services natively outside containerization)

---

## Quickstart Commands

### 1. Start Infrastructure Only (Recommended for local Go dev)
If you are developing Go code locally and only need PostgreSQL, Redis, and Kafka running in containers:

```bash
make infra-up
# or
docker compose up -d postgres redis kafka
```

Verify infrastructure container status:
```bash
docker compose ps
```

### 2. Start Full Stack (Infrastructure + All 14 Microservices)
To launch all infrastructure services and all microservices simultaneously:

```bash
make docker-up
# or
docker compose up -d
```

### 3. Rebuild Service Docker Images
If you modified service code and want to rebuild container images:

```bash
make docker-build
# or
docker compose build
```

### 4. Stop the Environment
Stop all running containers:

```bash
make docker-down
# or
docker compose down
```

To stop containers and wipe volume data (resets databases & message queues):
```bash
docker compose down -v
```

---

## Infrastructure Port & Connection Mapping

| Component | Host Port | Container Port | Service URL / DSN |
| :--- | :--- | :--- | :--- |
| **PostgreSQL** | `5432` | `5432` | `postgresql://postgres:postgres@localhost:5432/<db_name>?sslmode=disable` |
| **Redis** | `6379` | `6379` | `localhost:6379` |
| **Kafka** | `9092` | `9092` | `localhost:9092` / `kafka:9092` (inside docker network) |

---

## Database Management

Databases are auto-provisioned on initial `postgres` container creation. Service-specific database names:

- `gocart_auth`
- `gocart_users`
- `gocart_products`
- `gocart_category`
- `gocart_store`
- `gocart_merchant`
- `gocart_cart`
- `gocart_inventory`
- `gocart_orders`
- `gocart_payments`
- `gocart_delivery`
- `gocart_returns`
- `gocart_ratings`
- `gocart_notifications`

To inspect databases via `psql`:
```bash
docker compose exec postgres psql -U postgres -l
```

---

## Microservice Ports

| Service | Port | Health Check |
| :--- | :--- | :--- |
| `auth-service` | `9451` | `http://localhost:9451/health` |
| `cart-service` | `8181` | `http://localhost:8181/health` |
| `category-service` | `7080` | `http://localhost:7080/health` |
| `delivery-service` | `9050` | `http://localhost:9050/health` |
| `inventory-service` | `8300` | `http://localhost:8300/health` |
| `merchant-service` | `7600` | `http://localhost:7600/health` |
| `notification-service` | `9099` | `http://localhost:9099/health` |
| `order-service` | `8500` | `http://localhost:8500/health` |
| `payment-service` | `8888` | `http://localhost:8888/health` |
| `product-service` | `7070` | `http://localhost:7070/health` |
| `rating-service` | `5555` | `http://localhost:5555/health` |
| `return-service` | `9150` | `http://localhost:9150/health` |
| `store-service` | `7500` | `http://localhost:7500/health` |
| `user-service` | `5050` | `http://localhost:5050/health` |
