FROM golang:alpine AS builder

ENV GOTOOLCHAIN=auto

WORKDIR /app

COPY go.work go.work.sum* go.mod go.sum ./
COPY contracts/ ./contracts/
COPY pkg/ ./pkg/
COPY gateway/ ./gateway/
COPY services/ ./services/

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/server ./gateway/api-gateway/cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/server ./server

USER appuser

EXPOSE 8080

ENTRYPOINT ["/app/server"]
