FROM golang:alpine AS builder

WORKDIR /app

COPY go.work go.work.sum* go.mod go.sum ./
COPY contracts/ ./contracts/
COPY pkg/ ./pkg/
COPY services/ ./services/

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/server ./services/user-service/cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/server ./server
COPY --from=builder /app/services/user-service/migrations ./migrations

USER appuser

EXPOSE 5050

ENTRYPOINT ["/app/server"]
