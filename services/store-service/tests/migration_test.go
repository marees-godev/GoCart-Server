package tests_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreMigrationsSQLStructure(t *testing.T) {
	migrationsDir := filepath.Join("..", "migrations")

	// 1. Check 000001_init.sql
	initSQL, err := os.ReadFile(filepath.Join(migrationsDir, "000001_init.sql"))
	if err != nil {
		t.Fatalf("failed to read 000001_init.sql: %v", err)
	}
	initContent := string(initSQL)

	requiredInitDeclarations := []string{
		"CREATE TYPE store_approval_status AS ENUM ('DRAFT', 'PENDING_APPROVAL', 'APPROVED', 'REJECTED', 'SUSPENDED', 'CLOSED')",
		"CREATE TYPE outbox_status AS ENUM ('PENDING', 'PROCESSING', 'PUBLISHED', 'FAILED')",
		"CREATE TABLE IF NOT EXISTS stores",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"merchant_id UUID NOT NULL",
		"name VARCHAR(255) NOT NULL",
		"slug VARCHAR(255) NOT NULL UNIQUE",
		"approval_status store_approval_status NOT NULL DEFAULT 'DRAFT'",
		"CREATE TABLE IF NOT EXISTS store_bank_accounts",
		"store_id UUID NOT NULL REFERENCES stores(id) ON DELETE CASCADE",
		"CREATE TABLE IF NOT EXISTS outbox_events",
		"idx_stores_merchant_id",
		"idx_stores_slug",
		"idx_stores_approval_status",
		"idx_store_bank_accounts_store_id",
	}

	for _, decl := range requiredInitDeclarations {
		if !strings.Contains(initContent, decl) {
			t.Errorf("000001_init.sql missing required statement: %s", decl)
		}
	}

	// 2. Check 000002_kyc_publishing.sql
	kycSQL, err := os.ReadFile(filepath.Join(migrationsDir, "000002_kyc_publishing.sql"))
	if err != nil {
		t.Fatalf("failed to read 000002_kyc_publishing.sql: %v", err)
	}
	kycContent := string(kycSQL)

	requiredKYCDeclarations := []string{
		"CREATE TYPE store_kyc_status AS ENUM ('NOT_SUBMITTED', 'PENDING', 'VERIFIED', 'REJECTED')",
		"ALTER TABLE stores ADD COLUMN IF NOT EXISTS is_published BOOLEAN NOT NULL DEFAULT FALSE",
		"ALTER TABLE stores ADD COLUMN IF NOT EXISTS kyc_status store_kyc_status NOT NULL DEFAULT 'NOT_SUBMITTED'",
		"ALTER TABLE stores ADD COLUMN IF NOT EXISTS gstin VARCHAR(15)",
	}

	for _, decl := range requiredKYCDeclarations {
		if !strings.Contains(kycContent, decl) {
			t.Errorf("000002_kyc_publishing.sql missing required statement: %s", decl)
		}
	}

	// 3. Check 000003_store_appeals.sql
	appealsSQL, err := os.ReadFile(filepath.Join(migrationsDir, "000003_store_appeals.sql"))
	if err != nil {
		t.Fatalf("failed to read 000003_store_appeals.sql: %v", err)
	}
	appealsContent := string(appealsSQL)

	requiredAppealsDeclarations := []string{
		"CREATE TYPE store_appeal_status AS ENUM ('PENDING', 'APPROVED', 'REJECTED')",
		"CREATE TABLE IF NOT EXISTS store_appeals",
		"store_id UUID NOT NULL REFERENCES stores(id) ON DELETE CASCADE",
		"idx_store_appeals_store_id",
		"idx_store_appeals_merchant_id",
		"idx_store_appeals_status",
	}

	for _, decl := range requiredAppealsDeclarations {
		if !strings.Contains(appealsContent, decl) {
			t.Errorf("000003_store_appeals.sql missing required statement: %s", decl)
		}
	}

	// 4. Check 000004_outbox_topic.sql
	outboxSQL, err := os.ReadFile(filepath.Join(migrationsDir, "000004_outbox_topic.sql"))
	if err != nil {
		t.Fatalf("failed to read 000004_outbox_topic.sql: %v", err)
	}
	outboxContent := string(outboxSQL)

	requiredOutboxDeclarations := []string{
		"ALTER TABLE outbox_events",
		"topic VARCHAR(255) NOT NULL DEFAULT ''",
		"idx_outbox_pending_created",
	}

	for _, decl := range requiredOutboxDeclarations {
		if !strings.Contains(outboxContent, decl) {
			t.Errorf("000004_outbox_topic.sql missing required statement: %s", decl)
		}
	}
}
