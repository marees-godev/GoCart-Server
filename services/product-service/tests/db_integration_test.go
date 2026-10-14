package tests_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/repository"
)

func TestDBOutboxIntegration(t *testing.T) {
	_ = godotenv.Load("../.env")
	dbURL := os.Getenv("PRODUCT_SERVICE_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("Skipping DB outbox integration test: database URL not provided")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to postgres database: %v", err)
	}
	defer pool.Close()

	if migrator, mErr := database.NewMigrator(pool, "../migrations"); mErr == nil {
		if runErr := migrator.Run(ctx); runErr != nil {
			t.Logf("migrator run warning: %v", runErr)
		}
	}

	repo := repository.NewProductRepository(pool)

	uniqueSKU := "SKU-INT-" + time.Now().Format("20060102150405")
	prod := &model.Product{
		StoreID:     "11111111-1111-1111-1111-111111111111",
		CategoryID:  "22222222-2222-2222-2222-222222222222",
		SKU:         uniqueSKU,
		Name:        "DB Integration Product",
		Description: "Testing database outbox persistence",
		Price:       99.99,
		MRP:         119.99,
		Tax:         5.00,
		Currency:    "USD",
		Status:      model.StatusDraft,
	}

	// 1. Persist product and outbox event in PostgreSQL
	err = repo.CreateProduct(ctx, prod)
	if err != nil {
		t.Fatalf("failed to create product in database: %v", err)
	}

	if prod.ID == "" {
		t.Fatalf("expected non-empty product ID after insert")
	}

	// 2. Query outbox_events table directly to verify event persistence
	var eventType, topic, status string
	var payloadBytes []byte

	query := `
		SELECT event_type, topic, status, payload
		FROM outbox_events
		WHERE aggregate_type = 'product' AND aggregate_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	err = pool.QueryRow(ctx, query, prod.ID).Scan(&eventType, &topic, &status, &payloadBytes)
	if err != nil {
		t.Fatalf("failed to query outbox_events table: %v", err)
	}

	if eventType != events.EventTypeProductCreated {
		t.Errorf("expected event_type %s, got %s", events.EventTypeProductCreated, eventType)
	}

	if topic != events.TopicProductCreated {
		t.Errorf("expected topic %s, got %s", events.TopicProductCreated, topic)
	}

	if status != "PENDING" && status != "PUBLISHED" {
		t.Errorf("unexpected outbox status: %s", status)
	}

	// 3. Unmarshal envelope from payload
	env, err := events.UnmarshalEnvelope(payloadBytes)
	if err != nil {
		t.Fatalf("failed to unmarshal event envelope: %v", err)
	}

	var payload events.ProductCreatedEvent
	if err := env.UnmarshalData(&payload); err != nil {
		t.Fatalf("failed to unmarshal ProductCreatedEvent payload: %v", err)
	}

	if payload.ProductID != prod.ID || payload.SKU != uniqueSKU {
		t.Errorf("unmarshaled payload mismatch: %+v", payload)
	}
}
