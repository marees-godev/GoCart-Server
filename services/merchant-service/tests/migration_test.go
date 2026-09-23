package tests

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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
}

func TestMerchantModelFields(t *testing.T) {
	m := model.Merchant{}
	v := reflect.TypeOf(m)

	if _, ok := v.FieldByName("Role"); ok {
		t.Errorf("Merchant model should not contain Role field")
	}

	expectedFields := map[string]reflect.Type{
		"ID":              reflect.TypeOf(uuid.UUID{}),
		"UserID":          reflect.TypeOf(uuid.UUID{}),
		"BusinessName":    reflect.TypeOf(""),
		"FirstName":       reflect.TypeOf(""),
		"LastName":        reflect.TypeOf(""),
		"BusinessEmail":   reflect.TypeOf(""),
		"BusinessPhone":   reflect.TypeOf(""),
		"TaxID":           reflect.TypeOf(""),
		"Status":          reflect.TypeOf(""),
		"RejectionReason": reflect.TypeOf(""),
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
