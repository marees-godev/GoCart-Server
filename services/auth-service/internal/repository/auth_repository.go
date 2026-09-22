package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
)

var ErrNotFound = errors.New("record not found")

type AuthRepository interface {
	GetByEmail(ctx context.Context, email string) (*model.AuthCredential, error)
	UpdateFailedLogin(ctx context.Context, id uuid.UUID, failedCount int, lockedUntil *time.Time) error
	ResetFailedLogin(ctx context.Context, id uuid.UUID) error
	CreateLoginSession(ctx context.Context, refreshToken *model.RefreshToken, evt *outbox.Event) error
	CreateCredential(ctx context.Context, cred *model.AuthCredential) error
	GetRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id uuid.UUID) error
}

type postgresAuthRepository struct {
	pool        *pgxpool.Pool
	outboxStore *outbox.Store
	logger      *slog.Logger
}

func NewAuthRepository(pool *pgxpool.Pool, log *slog.Logger) AuthRepository {
	if log == nil {
		log = slog.Default()
	}
	return &postgresAuthRepository{
		pool:        pool,
		outboxStore: outbox.NewStore(),
		logger:      log,
	}
}

func (r *postgresAuthRepository) GetByEmail(ctx context.Context, email string) (*model.AuthCredential, error) {
	query := `
		SELECT id, user_id, email, phone, password_hash, role, email_verified, is_active, failed_login_count, locked_until, created_at, updated_at
		FROM auth_credentials
		WHERE LOWER(email) = LOWER($1)
		LIMIT 1
	`
	var cred model.AuthCredential
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&cred.ID,
		&cred.UserID,
		&cred.Email,
		&cred.Phone,
		&cred.PasswordHash,
		&cred.Role,
		&cred.EmailVerified,
		&cred.IsActive,
		&cred.FailedLoginCount,
		&cred.LockedUntil,
		&cred.CreatedAt,
		&cred.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Warn("User not found", "email", email)
			return nil, ErrNotFound
		}
		r.logger.Error("Failed to get user by email", "email", email, "error", err)
		return nil, fmt.Errorf("repository: get by email failed: %w", err)
	}
	return &cred, nil
}

func (r *postgresAuthRepository) UpdateFailedLogin(ctx context.Context, id uuid.UUID, failedCount int, lockedUntil *time.Time) error {
	query := `
		UPDATE auth_credentials
		SET failed_login_count = $1, locked_until = $2, updated_at = NOW()
		WHERE id = $3
	`
	_, err := r.pool.Exec(ctx, query, failedCount, lockedUntil, id)
	if err != nil {
		r.logger.Error("Failed to update failed login", "id", id, "error", err)
		return fmt.Errorf("repository: update failed login failed: %w", err)
	}
	return nil
}

func (r *postgresAuthRepository) ResetFailedLogin(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE auth_credentials
		SET failed_login_count = 0, locked_until = NULL, updated_at = NOW()
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		r.logger.Error("Failed to reset failed login", "id", id, "error", err)
		return fmt.Errorf("repository: reset failed login failed: %w", err)
	}
	return nil
}

func (r *postgresAuthRepository) CreateLoginSession(ctx context.Context, refreshToken *model.RefreshToken, evt *outbox.Event) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("repository: begin tx failed: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Reset failed login counters
	_, err = tx.Exec(ctx, `
		UPDATE auth_credentials
		SET failed_login_count = 0, locked_until = NULL, updated_at = NOW()
		WHERE user_id = $1
	`, refreshToken.UserID)
	if err != nil {
		r.logger.Error("Failed to reset counters in tx", "user_id", refreshToken.UserID, "error", err)
		return fmt.Errorf("repository: reset counters in tx failed: %w", err)
	}

	// 2. Insert refresh token
	if refreshToken.ID == uuid.Nil {
		refreshToken.ID = uuid.Must(uuid.NewV7())
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, revoked, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`, refreshToken.ID, refreshToken.UserID, refreshToken.TokenHash, refreshToken.ExpiresAt, refreshToken.Revoked)
	if err != nil {
		r.logger.Error("Failed to insert refresh token", "user_id", refreshToken.UserID, "error", err)
		return fmt.Errorf("repository: insert refresh token failed: %w", err)
	}

	// 3. Insert outbox audit event if provided
	if evt != nil {
		if err := r.outboxStore.Insert(ctx, tx, evt); err != nil {
			r.logger.Error("Failed to insert outbox event", "event", evt, "error", err)
			return fmt.Errorf("repository: insert outbox event failed: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		r.logger.Error("Failed to commit tx", "error", err)
		return fmt.Errorf("repository: commit tx failed: %w", err)
	}
	return nil
}

func (r *postgresAuthRepository) CreateCredential(ctx context.Context, cred *model.AuthCredential) error {
	if cred.ID == uuid.Nil {
		cred.ID = uuid.Must(uuid.NewV7())
	}
	if cred.UserID == uuid.Nil {
		cred.UserID = uuid.Must(uuid.NewV7())
	}
	query := `
		INSERT INTO auth_credentials (id, user_id, email, phone, password_hash, role, email_verified, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
	`
	_, err := r.pool.Exec(ctx, query, cred.ID, cred.UserID, cred.Email, cred.Phone, cred.PasswordHash, cred.Role, cred.EmailVerified, cred.IsActive)
	if err != nil {
		r.logger.Error("Failed to create credential", "error", err)
		return fmt.Errorf("repository: create credential failed: %w", err)
	}
	return nil
}

func (r *postgresAuthRepository) GetRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, expires_at, revoked, created_at
		FROM refresh_tokens
		WHERE token_hash = $1
		LIMIT 1
	`
	var tok model.RefreshToken
	err := r.pool.QueryRow(ctx, query, tokenHash).Scan(
		&tok.ID,
		&tok.UserID,
		&tok.TokenHash,
		&tok.ExpiresAt,
		&tok.Revoked,
		&tok.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Warn("Refresh token not found", "token_hash", tokenHash)
			return nil, ErrNotFound
		}
		r.logger.Error("Failed to get refresh token", "token_hash", tokenHash, "error", err)
		return nil, fmt.Errorf("repository: get refresh token failed: %w", err)
	}
	return &tok, nil
}

func (r *postgresAuthRepository) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE refresh_tokens
		SET revoked = true
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		r.logger.Error("Failed to revoke refresh token", "id", id, "error", err)
		return fmt.Errorf("repository: revoke refresh token failed: %w", err)
	}
	return nil
}
