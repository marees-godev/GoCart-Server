package tests

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
)

func TestMigrationSQLStructure(t *testing.T) {
	initPath := filepath.Join("..", "migrations", "000001_init.sql")
	initContent, err := os.ReadFile(initPath)
	if err != nil {
		t.Fatalf("failed to read 000001_init.sql: %v", err)
	}

	initSQL := string(initContent)
	requiredInit := []string{
		"CREATE TYPE merchant_status AS ENUM ('PENDING', 'APPROVED', 'REJECTED')",
		"CREATE TABLE IF NOT EXISTS merchants",
		"id UUID PRIMARY KEY",
		"business_name VARCHAR(255)",
		"status merchant_status NOT NULL DEFAULT 'PENDING'",
		"created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"idx_merchants_status",
	}

	for _, req := range requiredInit {
		if !strings.Contains(initSQL, req) {
			t.Errorf("000001_init.sql missing expected statement: %s", req)
		}
	}

	// Verify user_id is completely eliminated from 000001_init.sql
	if strings.Contains(strings.ToLower(initSQL), "user_id") {
		t.Errorf("000001_init.sql should not contain user_id")
	}

	// Verify 000002_add_name_fields.sql exists and drops user_id and adds deleted_at
	nameFieldsPath := filepath.Join("..", "migrations", "000002_add_name_fields.sql")
	nameFieldsContent, err := os.ReadFile(nameFieldsPath)
	if err != nil {
		t.Fatalf("failed to read 000002_add_name_fields.sql: %v", err)
	}
	if !strings.Contains(string(nameFieldsContent), "DROP COLUMN IF EXISTS user_id") {
		t.Errorf("000002_add_name_fields.sql must contain DROP COLUMN IF EXISTS user_id")
	}
	if !strings.Contains(string(nameFieldsContent), "deleted_at") {
		t.Errorf("000002_add_name_fields.sql must contain deleted_at")
	}
}

func TestMerchantStatusConstants(t *testing.T) {
	if model.MerchantStatusPending != "PENDING" {
		t.Errorf("expected MerchantStatusPending to be PENDING, got %s", model.MerchantStatusPending)
	}
	if model.MerchantStatusApproved != "APPROVED" {
		t.Errorf("expected MerchantStatusApproved to be APPROVED, got %s", model.MerchantStatusApproved)
	}
	if model.MerchantStatusRejected != "REJECTED" {
		t.Errorf("expected MerchantStatusRejected to be REJECTED, got %s", model.MerchantStatusRejected)
	}
	if model.MerchantStatusSuspended != "SUSPENDED" {
		t.Errorf("expected MerchantStatusSuspended to be SUSPENDED, got %s", model.MerchantStatusSuspended)
	}
	if model.MerchantStatusActive != "ACTIVE" {
		t.Errorf("expected MerchantStatusActive to be ACTIVE, got %s", model.MerchantStatusActive)
	}
	if model.MerchantStatusInactive != "INACTIVE" {
		t.Errorf("expected MerchantStatusInactive to be INACTIVE, got %s", model.MerchantStatusInactive)
	}
	if model.MerchantStatusPendingReview != "PENDING_REVIEW" {
		t.Errorf("expected MerchantStatusPendingReview to be PENDING_REVIEW, got %s", model.MerchantStatusPendingReview)
	}
	if model.MerchantStatusTerminated != "TERMINATED" {
		t.Errorf("expected MerchantStatusTerminated to be TERMINATED, got %s", model.MerchantStatusTerminated)
	}
}

func TestMerchantModelFields(t *testing.T) {
	m := model.Merchant{}
	v := reflect.TypeOf(m)

	if _, ok := v.FieldByName("Role"); ok {
		t.Errorf("Merchant model should not contain Role field")
	}
	if _, ok := v.FieldByName("UserID"); ok {
		t.Errorf("Merchant model should NOT contain UserID field")
	}

	expectedFields := map[string]reflect.Type{
		"ID":              reflect.TypeOf(uuid.UUID{}),
		"BusinessName":    reflect.TypeOf(""),
		"FirstName":       reflect.TypeOf(""),
		"LastName":        reflect.TypeOf(""),
		"BusinessEmail":   reflect.TypeOf(""),
		"BusinessPhone":   reflect.TypeOf(""),
		"PanCardNumber":   reflect.TypeOf(""),
		"Status":          reflect.TypeOf(""),
		"RejectionReason": reflect.TypeOf(""),
		"Version":         reflect.TypeOf(0),
		"CreatedAt":       reflect.TypeOf(time.Time{}),
		"UpdatedAt":       reflect.TypeOf(time.Time{}),
		"DeletedAt":       reflect.TypeOf((*time.Time)(nil)),
	}

	for fieldName, expectedType := range expectedFields {
		f, ok := v.FieldByName(fieldName)
		if !ok {
			t.Errorf("Merchant model missing field %s", fieldName)
			continue
		}
		if f.Type != expectedType {
			t.Errorf("Merchant field %s expected type %v, got %v", fieldName, expectedType, f.Type)
		}
	}
}

func TestMerchantLifecycleMigration(t *testing.T) {
	migrationPath := filepath.Join("..", "migrations", "000003_merchant_lifecycle.sql")
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("failed to read 000003_merchant_lifecycle.sql: %v", err)
	}

	sqlText := string(content)
	expectedTokens := []string{
		"ALTER TYPE merchant_status ADD VALUE IF NOT EXISTS 'ACTIVE'",
		"ALTER TYPE merchant_status ADD VALUE IF NOT EXISTS 'INACTIVE'",
		"ALTER TYPE merchant_status ADD VALUE IF NOT EXISTS 'PENDING_REVIEW'",
		"ALTER TYPE merchant_status ADD VALUE IF NOT EXISTS 'TERMINATED'",
		"ALTER TABLE merchants ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1",
		"CREATE TYPE merchant_lifecycle_action AS ENUM",
		"CREATE TYPE merchant_audit_status AS ENUM ('SUCCESS', 'FAILED')",
		"DROP TABLE IF EXISTS merchant_status_audit",
		"CREATE TABLE IF NOT EXISTS merchant_lifecycle_audit",
		"merchant_id UUID NOT NULL REFERENCES merchants(id) ON DELETE CASCADE",
		"admin_id VARCHAR(100) NOT NULL",
		"action merchant_lifecycle_action NOT NULL",
		"previous_status VARCHAR(50) NOT NULL",
		"new_status VARCHAR(50) NOT NULL",
		"status merchant_audit_status NOT NULL DEFAULT 'SUCCESS'",
		"request_id VARCHAR(100) NOT NULL DEFAULT ''",
		"idx_merchant_lifecycle_audit_merchant_id",
		"idx_merchant_lifecycle_audit_created_at",
		"idx_merchant_lifecycle_audit_admin_id",
	}

	for _, token := range expectedTokens {
		if !strings.Contains(sqlText, token) {
			t.Errorf("000003_merchant_lifecycle.sql missing token: %s", token)
		}
	}
}

func TestMerchantLifecycleAuditModelFields(t *testing.T) {
	a := model.MerchantLifecycleAudit{}
	v := reflect.TypeOf(a)

	expectedFields := map[string]reflect.Type{
		"ID":             reflect.TypeOf(uuid.UUID{}),
		"MerchantID":     reflect.TypeOf(uuid.UUID{}),
		"AdminID":        reflect.TypeOf(""),
		"Action":         reflect.TypeOf(model.LifecycleAction("")),
		"PreviousStatus": reflect.TypeOf(""),
		"NewStatus":      reflect.TypeOf(""),
		"Reason":         reflect.TypeOf(""),
		"Status":         reflect.TypeOf(model.AuditStatus("")),
		"ErrorMessage":   reflect.TypeOf(""),
		"RequestID":      reflect.TypeOf(""),
		"CreatedAt":      reflect.TypeOf(time.Time{}),
	}

	for fieldName, expectedType := range expectedFields {
		f, ok := v.FieldByName(fieldName)
		if !ok {
			t.Errorf("MerchantLifecycleAudit model missing field %s", fieldName)
			continue
		}
		if f.Type != expectedType {
			t.Errorf("MerchantLifecycleAudit field %s expected type %v, got %v", fieldName, expectedType, f.Type)
		}
	}
}
