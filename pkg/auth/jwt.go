package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

type contextKey string

const userContextKey contextKey = "auth_user_context"

type UserContext struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	Email  string `json:"email,omitempty"`
}

type UserClaims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	Email  string `json:"email,omitempty"`
	jwt.RegisteredClaims
}

func GenerateToken(user UserContext, secret string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("jwt secret cannot be empty")
	}

	now := time.Now()
	claims := UserClaims{
		UserID: user.UserID,
		Role:   user.Role,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.UserID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			Issuer:    "gocart-api-gateway",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ValidateToken(tokenStr, secret string) (*UserContext, error) {
	if tokenStr == "" {
		return nil, appErrors.Unauthorized("authorization token is required")
	}
	if secret == "" {
		return nil, appErrors.Internal(nil, "jwt secret not configured")
	}

	tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")
	tokenStr = strings.TrimSpace(tokenStr)

	token, err := jwt.ParseWithClaims(tokenStr, &UserClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil || !token.Valid {
		return nil, appErrors.Unauthorized("invalid or expired token")
	}

	claims, ok := token.Claims.(*UserClaims)
	if !ok {
		return nil, appErrors.Unauthorized("invalid token claims")
	}

	userID := claims.UserID
	if userID == "" {
		userID = claims.Subject
	}

	if userID == "" {
		return nil, appErrors.Unauthorized("invalid token: missing user identity")
	}

	return &UserContext{
		UserID: userID,
		Role:   claims.Role,
		Email:  claims.Email,
	}, nil
}

func WithUser(ctx context.Context, user *UserContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, userContextKey, user)
}

func UserFromContext(ctx context.Context) (*UserContext, bool) {
	if ctx == nil {
		return nil, false
	}
	user, ok := ctx.Value(userContextKey).(*UserContext)
	return user, ok && user != nil
}

func FromContext(ctx context.Context) (*UserContext, bool) {
	return UserFromContext(ctx)
}
