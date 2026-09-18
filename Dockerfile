# syntax=docker/dockerfile:1.4
# ------------------------------------------------------------------------------
# Stage 1: Build all service binaries and the API Gateway
# ------------------------------------------------------------------------------
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy workspace and module definitions
COPY go.work go.work.sum* ./
COPY go.mod go.sum* ./
COPY contracts/ contracts/
COPY pkg/ pkg/
COPY gateway/ gateway/
COPY services/ services/

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GOTOOLCHAIN=auto

# Build all 14 microservices and API Gateway
RUN go build -ldflags="-w -s" -o /app/bin/auth-service ./services/auth-service/cmd/server && \
    go build -ldflags="-w -s" -o /app/bin/user-service ./services/user-service/cmd/server && \
    go build -ldflags="-w -s" -o /app/bin/product-service ./services/product-service/cmd/server && \
    go build -ldflags="-w -s" -o /app/bin/category-service ./services/category-service/cmd/server && \
    go build -ldflags="-w -s" -o /app/bin/store-service ./services/store-service/cmd/server && \
    go build -ldflags="-w -s" -o /app/bin/merchant-service ./services/merchant-service/cmd/server && \
    go build -ldflags="-w -s" -o /app/bin/cart-service ./services/cart-service/cmd/server && \
    go build -ldflags="-w -s" -o /app/bin/inventory-service ./services/inventory-service/cmd/server && \
    go build -ldflags="-w -s" -o /app/bin/order-service ./services/order-service/cmd/server && \
    go build -ldflags="-w -s" -o /app/bin/payment-service ./services/payment-service/cmd/server && \
    go build -ldflags="-w -s" -o /app/bin/delivery-service ./services/delivery-service/cmd/server && \
    go build -ldflags="-w -s" -o /app/bin/notification-service ./services/notification-service/cmd/server && \
    go build -ldflags="-w -s" -o /app/bin/return-service ./services/return-service/cmd/server && \
    go build -ldflags="-w -s" -o /app/bin/rating-service ./services/rating-service/cmd/server && \
    go build -ldflags="-w -s" -o /app/bin/api-gateway ./gateway/api-gateway/cmd/server

# ------------------------------------------------------------------------------
# Stage 2: Minimal runtime container with supervisord
# ------------------------------------------------------------------------------
FROM alpine:3.20

WORKDIR /app

# Install supervisor and basic utilities
RUN apk add --no-cache supervisor ca-certificates tzdata curl

# Copy compiled binaries
COPY --from=builder /app/bin/ /app/bin/

# Copy migrations and static configurations for all services
COPY --from=builder /app/services/ /app/services/
COPY --from=builder /app/gateway/ /app/gateway/

# Copy supervisor configuration
COPY infrastructure/docker/supervisord.conf /etc/supervisor/supervisord.conf

# Render dynamically sets PORT env variable (default to 8080 if not set)
ENV PORT=8080 \
    APP_ENV=production \
    LOG_LEVEL=info \
    LOG_FORMAT=json

EXPOSE 8080

CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]
