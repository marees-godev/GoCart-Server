package auth

import (
	"context"
	"net/http"
	"testing"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"google.golang.org/grpc/metadata"
)

func TestExtractIdentityFromGRPC(t *testing.T) {
	tests := []struct {
		name          string
		md            metadata.MD
		expectUserID  string
		expectIsAdmin bool
		expectErrCode string
	}{
		{
			name: "Admin with ROLE_ADMIN in x-user-role",
			md: metadata.Pairs(
				"x-user-id", "admin-1",
				"x-user-role", "ROLE_ADMIN",
				"x-request-id", "req-123",
			),
			expectUserID:  "admin-1",
			expectIsAdmin: true,
		},
		{
			name: "Admin with ADMIN in x-user-roles",
			md: metadata.Pairs(
				"x-user-id", "admin-2",
				"x-user-roles", "ADMIN",
				"x-request-id", "req-456",
			),
			expectUserID:  "admin-2",
			expectIsAdmin: true,
		},
		{
			name: "Admin with multiple roles in x-user-roles",
			md: metadata.Pairs(
				"x-user-id", "admin-3",
				"x-user-roles", "ROLE_MERCHANT,ROLE_ADMIN",
			),
			expectUserID:  "admin-3",
			expectIsAdmin: true,
		},
		{
			name: "Merchant with ROLE_MERCHANT",
			md: metadata.Pairs(
				"x-user-id", "merchant-1",
				"x-user-role", "ROLE_MERCHANT",
			),
			expectUserID:  "merchant-1",
			expectIsAdmin: false,
		},
		{
			name: "Customer with CUSTOMER",
			md: metadata.Pairs(
				"x-user-id", "cust-1",
				"x-user-role", "CUSTOMER",
			),
			expectUserID:  "cust-1",
			expectIsAdmin: false,
		},
		{
			name: "Missing user id header",
			md: metadata.Pairs(
				"x-user-role", "ROLE_ADMIN",
			),
			expectErrCode: appErrors.CodeUnauthorized,
		},
		{
			name: "Missing role header",
			md: metadata.Pairs(
				"x-user-id", "admin-1",
			),
			expectErrCode: appErrors.CodeUnauthorized,
		},
		{
			name:          "Empty context metadata",
			md:            metadata.Pairs(),
			expectErrCode: appErrors.CodeUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := metadata.NewIncomingContext(context.Background(), tt.md)
			id, err := ExtractIdentityFromGRPC(ctx)
			if tt.expectErrCode != "" {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				appErr := appErrors.AsAppError(err)
				if appErr == nil || appErr.Code != tt.expectErrCode {
					t.Fatalf("expected %s, got %v", tt.expectErrCode, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if id.UserID != tt.expectUserID {
					t.Fatalf("expected user id %s, got %s", tt.expectUserID, id.UserID)
				}
				if id.IsAdmin() != tt.expectIsAdmin {
					t.Fatalf("expected IsAdmin=%v, got %v", tt.expectIsAdmin, id.IsAdmin())
				}
			}
		})
	}
}

func TestRequireAdmin(t *testing.T) {
	tests := []struct {
		name          string
		id            *Identity
		expectErrCode string
		expectHTTP    int
	}{
		{
			name: "Authorized ADMIN",
			id: &Identity{
				UserID: "adm-1",
				Role:   "ADMIN",
				Roles:  []string{"ADMIN"},
			},
		},
		{
			name: "Authorized ROLE_ADMIN",
			id: &Identity{
				UserID: "adm-2",
				Role:   "ROLE_ADMIN",
				Roles:  []string{"ROLE_ADMIN"},
			},
		},
		{
			name: "Unauthorized - nil identity",
			id:   nil,
			expectErrCode: appErrors.CodeUnauthorized,
			expectHTTP:    http.StatusUnauthorized,
		},
		{
			name: "Unauthorized - empty user ID",
			id: &Identity{
				UserID: "",
				Role:   "ADMIN",
			},
			expectErrCode: appErrors.CodeUnauthorized,
			expectHTTP:    http.StatusUnauthorized,
		},
		{
			name: "Forbidden - ROLE_MERCHANT",
			id: &Identity{
				UserID: "m-1",
				Role:   "ROLE_MERCHANT",
				Roles:  []string{"ROLE_MERCHANT"},
			},
			expectErrCode: appErrors.CodeForbidden,
			expectHTTP:    http.StatusForbidden,
		},
		{
			name: "Forbidden - CUSTOMER",
			id: &Identity{
				UserID: "c-1",
				Role:   "CUSTOMER",
				Roles:  []string{"CUSTOMER"},
			},
			expectErrCode: appErrors.CodeForbidden,
			expectHTTP:    http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RequireAdmin(tt.id)
			if tt.expectErrCode != "" {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				appErr := appErrors.AsAppError(err)
				if appErr == nil || appErr.Code != tt.expectErrCode {
					t.Fatalf("expected %s, got %v", tt.expectErrCode, err)
				}
				if appErr.HTTPStatus != tt.expectHTTP {
					t.Fatalf("expected HTTP status %d, got %d", tt.expectHTTP, appErr.HTTPStatus)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestExtractIdentityFromHeaders(t *testing.T) {
	headers := map[string]string{
		"X-User-Id":    "admin-99",
		"X-User-Roles": "ROLE_ADMIN",
		"X-Request-Id": "trace-99",
	}
	getter := func(key string) string {
		return headers[key]
	}

	id, err := ExtractIdentityFromHeaders(getter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.UserID != "admin-99" || !id.IsAdmin() || id.RequestID != "trace-99" {
		t.Fatalf("unexpected extracted identity: %+v", id)
	}
}
