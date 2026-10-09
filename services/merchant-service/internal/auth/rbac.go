package auth

import (
	"context"
	"strings"

	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"google.golang.org/grpc/metadata"
)

// Identity captures the extracted downstream identity propagated from the API Gateway.
type Identity struct {
	UserID    string
	Role      string
	Roles     []string
	RequestID string
}

// IsAdmin returns true if the identity holds the ADMIN or ROLE_ADMIN role.
func (id *Identity) IsAdmin() bool {
	if id == nil {
		return false
	}
	for _, r := range id.Roles {
		trimmed := strings.ToUpper(strings.TrimSpace(r))
		if trimmed == "ADMIN" || trimmed == "ROLE_ADMIN" {
			return true
		}
	}
	trimmed := strings.ToUpper(strings.TrimSpace(id.Role))
	return trimmed == "ADMIN" || trimmed == "ROLE_ADMIN"
}

// ExtractIdentityFromGRPC extracts user identity and roles from gRPC incoming metadata or pkg/auth.
func ExtractIdentityFromGRPC(ctx context.Context) (*Identity, error) {
	var userID, rawRole, reqID string

	// Check pkg/auth context first
	if u, ok := auth.FromContext(ctx); ok && u != nil {
		userID = strings.TrimSpace(u.UserID)
		rawRole = strings.TrimSpace(u.Role)
	}

	// Read gRPC metadata
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if userID == "" {
			for _, key := range []string{"x-user-id", "user-id", "user_id"} {
				if vals := md.Get(key); len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
					userID = strings.TrimSpace(vals[0])
					break
				}
			}
		}

		// Read role or roles
		if rawRole == "" {
			for _, key := range []string{"x-user-roles", "user-roles", "user_roles", "x-user-role", "user-role", "user_role", "role"} {
				if vals := md.Get(key); len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
					rawRole = strings.TrimSpace(vals[0])
					break
				}
			}
		}

		for _, key := range []string{"x-request-id", "request-id", "request_id", "x-correlation-id"} {
			if vals := md.Get(key); len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
				reqID = strings.TrimSpace(vals[0])
				break
			}
		}
	}

	if userID == "" || rawRole == "" {
		return nil, appErrors.Unauthorized("missing or invalid gateway authentication headers")
	}

	roles := parseRoles(rawRole)
	return &Identity{
		UserID:    userID,
		Role:      rawRole,
		Roles:     roles,
		RequestID: reqID,
	}, nil
}

// ExtractIdentityFromHeaders extracts user identity and roles from standard map-based or HTTP headers.
func ExtractIdentityFromHeaders(getHeader func(key string) string) (*Identity, error) {
	userID := getHeader("X-User-Id")
	if userID == "" {
		userID = getHeader("x-user-id")
	}

	rawRole := getHeader("X-User-Roles")
	if rawRole == "" {
		rawRole = getHeader("x-user-roles")
	}
	if rawRole == "" {
		rawRole = getHeader("X-User-Role")
	}
	if rawRole == "" {
		rawRole = getHeader("x-user-role")
	}
	if rawRole == "" {
		rawRole = getHeader("Role")
	}
	if rawRole == "" {
		rawRole = getHeader("role")
	}

	reqID := getHeader("X-Request-Id")
	if reqID == "" {
		reqID = getHeader("x-request-id")
	}

	userID = strings.TrimSpace(userID)
	rawRole = strings.TrimSpace(rawRole)

	if userID == "" || rawRole == "" {
		return nil, appErrors.Unauthorized("missing or invalid gateway authentication headers")
	}

	roles := parseRoles(rawRole)
	return &Identity{
		UserID:    userID,
		Role:      rawRole,
		Roles:     roles,
		RequestID: strings.TrimSpace(reqID),
	}, nil
}

// RequireAdmin evaluates that the caller possesses the ADMIN or ROLE_ADMIN privilege.
// Returns 401 Unauthorized if identity headers are missing/invalid, or 403 Forbidden if not an admin.
func RequireAdmin(id *Identity) error {
	if id == nil || id.UserID == "" {
		return appErrors.Unauthorized("missing or invalid gateway authentication headers")
	}
	if !id.IsAdmin() {
		return appErrors.Forbidden("forbidden: administrator privileges required")
	}
	return nil
}

func parseRoles(raw string) []string {
	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
