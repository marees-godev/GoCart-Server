# syntax=docker/dockerfile:1.4
FROM golang:alpine AS builder

ENV GOTOOLCHAIN=auto \
    GOPROXY=https://proxy.golang.org,direct

WORKDIR /app

COPY go.work go.work.sum* go.mod go.sum ./
COPY contracts/ ./contracts/
COPY pkg/ ./pkg/
COPY gateway/ ./gateway/
COPY services/ ./services/

RUN --mount=type=cache,target=/go/pkg/mod \
    for i in 1 2 3; do go mod download && break || (echo "Retrying go mod download ($i/3)..." && sleep 2); done

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/server ./gateway/api-gateway/cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/server ./server

USER appuser

EXPOSE 8080

ENTRYPOINT ["/app/server"]
