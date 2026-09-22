package tests

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gofrs/uuid/v5"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
)

func TestMigrationSQLStructure(t *testing.T) {
	migrationPath := filepath.Join("..", "migrations", "000001_init.sql")
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("failed to read migration file: %v", err)
	}

	sql := string(content)

	requiredStrings := []string{
		"CREATE TABLE IF NOT EXISTS auth_credentials",
		"id UUID PRIMARY KEY",
		"user_id UUID NOT NULL UNIQUE",
		"email VARCHAR(255) NOT NULL UNIQUE",
		"phone VARCHAR(20) UNIQUE",
		"password_hash VARCHAR(255) NOT NULL",
		"role VARCHAR(50) NOT NULL DEFAULT 'customer'",
		"email_verified BOOLEAN NOT NULL DEFAULT FALSE",
		"failed_login_count INT NOT NULL DEFAULT 0",
		"locked_until TIMESTAMPTZ",

		"CREATE TABLE IF NOT EXISTS refresh_tokens",
		"token_hash VARCHAR(255) NOT NULL UNIQUE",
		"expires_at TIMESTAMPTZ NOT NULL",
		"revoked BOOLEAN NOT NULL DEFAULT FALSE",

		"CREATE TABLE IF NOT EXISTS password_reset_tokens",
		"used BOOLEAN NOT NULL DEFAULT FALSE",

		"CREATE TABLE IF NOT EXISTS outbox_events",

		"idx_auth_credentials_email",
		"idx_auth_credentials_phone",
		"idx_refresh_tokens_user_id",
		"idx_password_reset_tokens_user_id",
	}

	for _, str := range requiredStrings {
		if !strings.Contains(sql, str) {
			t.Errorf("migration SQL missing required declaration: %s", str)
		}
	}
}

func TestRollbackSQLStructure(t *testing.T) {
	rollbackPath := filepath.Join("..", "migrations", "rollback", "000001_init_rollback.sql")
	content, err := os.ReadFile(rollbackPath)
	if err != nil {
		t.Fatalf("failed to read rollback file: %v", err)
	}

	sql := string(content)

	requiredDrops := []string{
		"DROP TABLE IF EXISTS password_reset_tokens",
		"DROP TABLE IF EXISTS refresh_tokens",
		"DROP TABLE IF EXISTS outbox_events",
		"DROP TABLE IF EXISTS auth_credentials",
	}

	for _, str := range requiredDrops {
		if !strings.Contains(sql, str) {
			t.Errorf("rollback SQL missing statement: %s", str)
		}
	}
}

func TestAuthModelSecurityAndFields(t *testing.T) {
	cred := model.AuthCredential{
		ID:               uuid.Must(uuid.NewV7()),
		UserID:           uuid.Must(uuid.NewV7()),
		Email:            "test@example.com",
		PasswordHash:     "$2a$12$securehash",
		Role:             model.RoleCustomer,
		EmailVerified:    true,
		IsActive:         true,
		FailedLoginCount: 3,
	}

	if cred.PasswordHash == "" {
		t.Error("expected non-empty password hash")
	}

	field, ok := reflect.TypeOf(cred).FieldByName("PasswordHash")
	if !ok {
		t.Fatal("AuthCredential missing PasswordHash field")
	}

	if tag := field.Tag.Get("json"); tag != "-" {
		t.Errorf("PasswordHash field MUST have json:\"-\" tag for security, got: %s", tag)
	}
}

func TestTokenModelsFields(t *testing.T) {
	rtField, ok := reflect.TypeOf(model.RefreshToken{}).FieldByName("TokenHash")
	if !ok || rtField.Tag.Get("json") != "-" {
		t.Errorf("RefreshToken TokenHash must exist and be ignored in json output")
	}

	prField, ok := reflect.TypeOf(model.PasswordResetToken{}).FieldByName("TokenHash")
	if !ok || prField.Tag.Get("json") != "-" {
		t.Errorf("PasswordResetToken TokenHash must exist and be ignored in json output")
	}
}

func TestCompoundUniqueMigrationSQL(t *testing.T) {
	migrationPath := filepath.Join("..", "migrations", "000003_email_role_unique.sql")
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("failed to read migration file: %v", err)
	}

	sql := string(content)

	requiredStrings := []string{
		"ALTER TABLE auth_credentials DROP CONSTRAINT IF EXISTS auth_credentials_email_key",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_credentials_email_role ON auth_credentials (LOWER(email), role)",
	}

	for _, str := range requiredStrings {
		if !strings.Contains(sql, str) {
			t.Errorf("000003 migration missing required SQL: %s", str)
		}
	}
}

