package tests_test

import (
	"context"
	"os"

	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/repository"
)

func TestDBOutboxIntegration(t *testing.T) {
	_ = godotenv.Load("../.env")
	dbURL := os.Getenv("STORE_SERVICE_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("Skipping DB outbox integration test: database URL not provided")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to postgres database: %v", err)
	}
	defer pool.Close()

	repo := repository.NewStoreRepository(pool)

	uniqueSlug := "integration-store-" + time.Now().Format("20060102150405")
	store := &model.Store{
		MerchantID:     "11111111-1111-1111-1111-111111111111",
		Name:           "DB Integration Store",
		Slug:           uniqueSlug,
		BusinessEmail:  "integration@example.com",
		BusinessPhone:  "+1555123456",
		Description:    "Testing database outbox persistence",
		Address:        "123 Main St",
		ApprovalStatus: model.StoreStatusDraft,
	}

	// 1. Persist store and outbox event in PostgreSQL
	err = repo.Create(ctx, store)
	if err != nil {
		t.Fatalf("failed to create store in database: %v", err)
	}

	if store.ID == "" {
		t.Fatalf("expected non-empty store ID after insert")
	}

	// 2. Query outbox_events table directly to verify event persistence
	var eventType, topic, status string
	var payloadBytes []byte

	query := `
		SELECT event_type, topic, status, payload
		FROM outbox_events
		WHERE aggregate_type = 'store' AND aggregate_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	err = pool.QueryRow(ctx, query, store.ID).Scan(&eventType, &topic, &status, &payloadBytes)
	if err != nil {
		t.Fatalf("failed to query outbox_events table: %v", err)
	}

	if eventType != events.EventTypeStoreCreated {
		t.Errorf("expected event_type %s, got %s", events.EventTypeStoreCreated, eventType)
	}

	if topic != events.TopicStoreCreated {
		t.Errorf("expected topic %s, got %s", events.TopicStoreCreated, topic)
	}

	if status != "PENDING" && status != "PUBLISHED" {
		t.Errorf("unexpected outbox status: %s", status)
	}

	// 3. Unmarshal envelope from payload
	env, err := events.UnmarshalEnvelope(payloadBytes)
	if err != nil {
		t.Fatalf("failed to unmarshal event envelope: %v", err)
	}

	var payload events.StoreCreatedEvent
	if err := env.UnmarshalData(&payload); err != nil {
		t.Fatalf("failed to unmarshal StoreCreatedEvent payload: %v", err)
	}

	if payload.StoreID != store.ID || payload.Slug != uniqueSlug {
		t.Errorf("unmarshaled payload mismatch: %+v", payload)
	}

	// Cleanup test row
	_, _ = pool.Exec(ctx, "DELETE FROM stores WHERE id = $1", store.ID)
	_, _ = pool.Exec(ctx, "DELETE FROM outbox_events WHERE aggregate_id = $1", store.ID)
}
