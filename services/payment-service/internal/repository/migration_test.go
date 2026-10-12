package repository_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPaymentOutboxMigrationsSchema(t *testing.T) {
	migrationsDir := filepath.Join("..", "..", "migrations")

	// 1. Verify 000001_init.sql
	initSQL, err := os.ReadFile(filepath.Join(migrationsDir, "000001_init.sql"))
	if err != nil {
		t.Fatalf("failed to read 000001_init.sql: %v", err)
	}
	initContent := string(initSQL)

	requiredInitElements := []string{
		"CREATE TYPE outbox_status AS ENUM ('PENDING', 'PUBLISHED', 'FAILED')",
		"CREATE TABLE IF NOT EXISTS outbox_events",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"aggregate_type VARCHAR(100) NOT NULL",
		"aggregate_id VARCHAR(255) NOT NULL",
		"event_type VARCHAR(100) NOT NULL",
		"payload JSONB NOT NULL",
		"topic VARCHAR(255) NOT NULL DEFAULT ''",
		"status outbox_status NOT NULL DEFAULT 'PENDING'",
		"retry_count INT NOT NULL DEFAULT 0",
		"created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"published_at TIMESTAMPTZ",
		"idx_payment_outbox_status_created",
		"idx_payment_outbox_aggregate",
	}

	for _, elem := range requiredInitElements {
		if !strings.Contains(initContent, elem) {
			t.Errorf("000001_init.sql missing required schema element: %s", elem)
		}
	}

	// 2. Verify 000002_outbox_topic.sql
	topicSQL, err := os.ReadFile(filepath.Join(migrationsDir, "000002_outbox_topic.sql"))
	if err != nil {
		t.Fatalf("failed to read 000002_outbox_topic.sql: %v", err)
	}
	topicContent := string(topicSQL)

	requiredTopicElements := []string{
		"ALTER TABLE outbox_events",
		"ADD COLUMN IF NOT EXISTS topic VARCHAR(255) NOT NULL DEFAULT ''",
		"idx_outbox_pending_created",
		"WHERE status = 'PENDING'",
		"idx_payment_outbox_aggregate",
	}

	for _, elem := range requiredTopicElements {
		if !strings.Contains(topicContent, elem) {
			t.Errorf("000002_outbox_topic.sql missing required statement: %s", elem)
		}
	}
}
