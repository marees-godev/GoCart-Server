package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
)

type UserRepository interface {
	CreateUser(ctx context.Context, user *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	UpdateUser(ctx context.Context, user *model.User) error
	DeactivateUser(ctx context.Context, userID, performedBy string, reason *string) error
	ReactivateUser(ctx context.Context, userID, performedBy string) error
	DeleteUser(ctx context.Context, userID, performedBy string, reason *string) error
	GetExpiredDeactivatedUserIDs(ctx context.Context, cutoff time.Time) ([]string, error)
	GetUserAuditLogs(ctx context.Context, userID string) ([]*model.UserAuditLog, error)
}

type pgUserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) UserRepository {
	return &pgUserRepository{pool: pool}
}

func (r *pgUserRepository) CreateUser(ctx context.Context, user *model.User) error {
	if user == nil {
		slog.WarnContext(ctx, "user model is nil in CreateUser")
		return appErrors.BadRequest("user model cannot be nil")
	}
	query := `
		INSERT INTO users (id, email, first_name, last_name, status, created_at, updated_at)
		VALUES (
			$1::uuid,
			$2,
			$3,
			$4,
			COALESCE(NULLIF($5, ''),'active')::user_status,
			NOW(),
			NOW()
		)
		RETURNING created_at, updated_at
	`
	err := r.pool.QueryRow(ctx, query,
		user.ID,
		user.Email,
		user.FirstName,
		user.LastName,
		user.Status,
	).Scan(&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			slog.WarnContext(ctx, "user with email already exists in CreateUser", "email", user.Email)
			return appErrors.Conflict("user with this email already exists")
		}
		slog.ErrorContext(ctx, "failed to create user in repository", "user_id", user.ID, "email", user.Email, "error", err)
		return appErrors.Internal(err, "failed to create user")
	}
	slog.InfoContext(ctx, "user created in repository", "user_id", user.ID, "email", user.Email)
	return nil
}

func (r *pgUserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	query := `
		SELECT id, username, email, first_name, last_name, phone, alternate_phone, date_of_birth, gender, bio, avatar_url, status, created_at, updated_at, deactivated_at, deleted_at
		FROM users
		WHERE id = $1
	`
	var u model.User
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&u.ID,
		&u.Username,
		&u.Email,
		&u.FirstName,
		&u.LastName,
		&u.PhoneNumber,
		&u.AlternatePhone,
		&u.DateOfBirth,
		&u.Gender,
		&u.Bio,
		&u.AvatarURL,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.DeactivatedAt,
		&u.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "user not found by id in repository", "user_id", id)
			return nil, appErrors.NotFound("user not found")
		}
		slog.ErrorContext(ctx, "failed to query user by id in repository", "user_id", id, "error", err)
		return nil, appErrors.Internal(err, "failed to query user by id")
	}
	slog.InfoContext(ctx, "user retrieved by id in repository", "user_id", u.ID)
	return &u, nil
}

func (r *pgUserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `
		SELECT id, username, email, first_name, last_name, phone, alternate_phone, date_of_birth, gender, bio, avatar_url, status, created_at, updated_at, deactivated_at, deleted_at
		FROM users
		WHERE email = $1
	`
	var u model.User
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&u.ID,
		&u.Username,
		&u.Email,
		&u.FirstName,
		&u.LastName,
		&u.PhoneNumber,
		&u.AlternatePhone,
		&u.DateOfBirth,
		&u.Gender,
		&u.Bio,
		&u.AvatarURL,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.DeactivatedAt,
		&u.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "user not found by email in repository", "email", email)
			return nil, appErrors.NotFound("user not found")
		}
		slog.ErrorContext(ctx, "failed to query user by email in repository", "email", email, "error", err)
		return nil, appErrors.Internal(err, "failed to query user by email")
	}
	slog.InfoContext(ctx, "user retrieved by email in repository", "user_id", u.ID, "email", u.Email)
	return &u, nil
}

func (r *pgUserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	query := `
		SELECT id, username, email, first_name, last_name, phone, alternate_phone, date_of_birth, gender, bio, avatar_url, status, created_at, updated_at, deactivated_at, deleted_at
		FROM users
		WHERE username = $1
	`
	var u model.User
	err := r.pool.QueryRow(ctx, query, username).Scan(
		&u.ID,
		&u.Username,
		&u.Email,
		&u.FirstName,
		&u.LastName,
		&u.PhoneNumber,
		&u.AlternatePhone,
		&u.DateOfBirth,
		&u.Gender,
		&u.Bio,
		&u.AvatarURL,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.DeactivatedAt,
		&u.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "user not found by username in repository", "username", username)
			return nil, appErrors.NotFound("user not found")
		}
		slog.ErrorContext(ctx, "failed to query user by username in repository", "username", username, "error", err)
		return nil, appErrors.Internal(err, "failed to query user by username")
	}
	slog.InfoContext(ctx, "user retrieved by username in repository", "user_id", u.ID, "username", username)
	return &u, nil
}

func (r *pgUserRepository) UpdateUser(ctx context.Context, user *model.User) error {
	query := `
		UPDATE users
		SET username = $1, email = $2, first_name = $3, last_name = $4, phone = $5, alternate_phone = $6, date_of_birth = $7, gender = $8, bio = $9, avatar_url = $10, updated_at = NOW()
		WHERE id = $11
		RETURNING updated_at
	`
	err := r.pool.QueryRow(ctx, query,
		user.Username,
		user.Email,
		user.FirstName,
		user.LastName,
		user.PhoneNumber,
		user.AlternatePhone,
		user.DateOfBirth,
		user.Gender,
		user.Bio,
		user.AvatarURL,
		user.ID,
	).Scan(&user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "user not found for update in repository", "user_id", user.ID)
			return appErrors.NotFound("user not found")
		}
		slog.ErrorContext(ctx, "failed to update user profile in repository", "user_id", user.ID, "error", err)
		return appErrors.Internal(err, "failed to update user profile")
	}
	slog.InfoContext(ctx, "user profile updated in repository", "user_id", user.ID)
	return nil
}

func (r *pgUserRepository) DeactivateUser(ctx context.Context, userID, performedBy string, reason *string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin tx in DeactivateUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	checkQuery := `SELECT status FROM users WHERE id = $1 FOR UPDATE`
	if err := tx.QueryRow(ctx, checkQuery, userID).Scan(&currentStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "user not found in DeactivateUser", "user_id", userID)
			return appErrors.NotFound("user not found")
		}
		slog.ErrorContext(ctx, "failed to fetch user status in DeactivateUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to fetch user status")
	}

	if currentStatus == "deleted" {
		slog.WarnContext(ctx, "cannot deactivate deleted user", "user_id", userID)
		return appErrors.Forbidden("cannot deactivate a deleted account")
	}

	if currentStatus == "deactivated" {
		slog.InfoContext(ctx, "user already deactivated in DeactivateUser", "user_id", userID)
		return nil
	}

	updateQuery := `
		UPDATE users
		SET status = 'deactivated'::user_status, deactivated_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`
	if _, err := tx.Exec(ctx, updateQuery, userID); err != nil {
		slog.ErrorContext(ctx, "failed to update user status in DeactivateUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to deactivate user")
	}

	auditQuery := `
		INSERT INTO user_audit_logs (user_id, action, performed_by, reason, created_at)
		VALUES ($1, 'DEACTIVATE'::user_audit_action, $2, $3, NOW())
	`
	if _, err := tx.Exec(ctx, auditQuery, userID, performedBy, reason); err != nil {
		slog.ErrorContext(ctx, "failed to insert audit log in DeactivateUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to record audit log")
	}

	payloadBytes, err := json.Marshal(map[string]any{
		"user_id":        userID,
		"status":         "deactivated",
		"performed_by":   performedBy,
		"reason":         reason,
		"deactivated_at": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to marshal outbox payload in DeactivateUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to construct event payload")
	}

	outboxQuery := `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, topic, status, retry_count, created_at)
		VALUES ('user', $1, 'user.deactivated', $2, 'user.events', 'PENDING', 0, NOW())
	`
	if _, err := tx.Exec(ctx, outboxQuery, userID, payloadBytes); err != nil {
		slog.ErrorContext(ctx, "failed to insert outbox event in DeactivateUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to record outbox event")
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit tx in DeactivateUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to commit transaction")
	}

	slog.InfoContext(ctx, "user deactivated successfully", "user_id", userID, "performed_by", performedBy)
	return nil
}

func (r *pgUserRepository) ReactivateUser(ctx context.Context, userID, performedBy string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin tx in ReactivateUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	var deactivatedAt *time.Time
	checkQuery := `SELECT status, deactivated_at FROM users WHERE id = $1 FOR UPDATE`
	if err := tx.QueryRow(ctx, checkQuery, userID).Scan(&currentStatus, &deactivatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "user not found in ReactivateUser", "user_id", userID)
			return appErrors.NotFound("user not found")
		}
		slog.ErrorContext(ctx, "failed to fetch user status in ReactivateUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to fetch user status")
	}

	if currentStatus == "deleted" {
		slog.WarnContext(ctx, "cannot reactivate deleted user", "user_id", userID)
		return appErrors.Forbidden("cannot reactivate a deleted account")
	}

	if currentStatus == "active" {
		slog.InfoContext(ctx, "user already active in ReactivateUser", "user_id", userID)
		return nil
	}

	if deactivatedAt != nil && time.Since(*deactivatedAt) > 30*24*time.Hour {
		slog.WarnContext(ctx, "deactivation grace period expired in ReactivateUser; permanently soft-deleting account", "user_id", userID, "deactivated_at", *deactivatedAt)
		_ = tx.Rollback(ctx)
		reason := "30-day deactivation grace period expired"
		_ = r.DeleteUser(ctx, userID, "SYSTEM", &reason)
		return appErrors.Forbidden("account deactivation period of 30 days has expired; account has been permanently deleted")
	}

	updateQuery := `
		UPDATE users
		SET status = 'active'::user_status, deactivated_at = NULL, updated_at = NOW()
		WHERE id = $1
	`
	if _, err := tx.Exec(ctx, updateQuery, userID); err != nil {
		slog.ErrorContext(ctx, "failed to reactivate user in repository", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to reactivate user")
	}

	auditQuery := `
		INSERT INTO user_audit_logs (user_id, action, performed_by, reason, created_at)
		VALUES ($1, 'REACTIVATE'::user_audit_action, $2, 'Account reactivated within 30-day grace period', NOW())
	`
	if _, err := tx.Exec(ctx, auditQuery, userID, performedBy); err != nil {
		slog.ErrorContext(ctx, "failed to insert audit log in ReactivateUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to record audit log")
	}

	payloadBytes, err := json.Marshal(map[string]any{
		"user_id":        userID,
		"status":         "active",
		"performed_by":   performedBy,
		"reactivated_at": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to marshal outbox payload in ReactivateUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to construct event payload")
	}

	outboxQuery := `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, topic, status, retry_count, created_at)
		VALUES ('user', $1, 'user.reactivated', $2, 'user.events', 'PENDING', 0, NOW())
	`
	if _, err := tx.Exec(ctx, outboxQuery, userID, payloadBytes); err != nil {
		slog.ErrorContext(ctx, "failed to insert outbox event in ReactivateUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to record outbox event")
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit tx in ReactivateUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to commit transaction")
	}

	slog.InfoContext(ctx, "user reactivated successfully", "user_id", userID, "performed_by", performedBy)
	return nil
}

func (r *pgUserRepository) DeleteUser(ctx context.Context, userID, performedBy string, reason *string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin tx in DeleteUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	checkQuery := `SELECT status FROM users WHERE id = $1 FOR UPDATE`
	if err := tx.QueryRow(ctx, checkQuery, userID).Scan(&currentStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "user not found in DeleteUser", "user_id", userID)
			return appErrors.NotFound("user not found")
		}
		slog.ErrorContext(ctx, "failed to fetch user status in DeleteUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to fetch user status")
	}

	if currentStatus == "deleted" {
		slog.InfoContext(ctx, "user already deleted in DeleteUser", "user_id", userID)
		return nil
	}

	anonymizedEmail := fmt.Sprintf("deleted_%s@deleted.local", userID)
	anonymizeQuery := `
		UPDATE users
		SET email = $2,
		    username = NULL,
		    first_name = 'Deleted',
		    last_name = 'User',
		    phone = NULL,
		    alternate_phone = NULL,
		    date_of_birth = NULL,
		    gender = NULL,
		    bio = NULL,
		    avatar_url = NULL,
		    status = 'deleted'::user_status,
		    deactivated_at = NULL,
		    deleted_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`
	if _, err := tx.Exec(ctx, anonymizeQuery, userID, anonymizedEmail); err != nil {
		slog.ErrorContext(ctx, "failed to anonymize user in DeleteUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to anonymize user data")
	}

	deleteAddressesQuery := `DELETE FROM user_addresses WHERE user_id = $1`
	if _, err := tx.Exec(ctx, deleteAddressesQuery, userID); err != nil {
		slog.ErrorContext(ctx, "failed to delete user addresses in DeleteUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to delete user addresses")
	}

	auditQuery := `
		INSERT INTO user_audit_logs (user_id, action, performed_by, reason, created_at)
		VALUES ($1, 'DELETE'::user_audit_action, $2, $3, NOW())
	`
	if _, err := tx.Exec(ctx, auditQuery, userID, performedBy, reason); err != nil {
		slog.ErrorContext(ctx, "failed to insert audit log in DeleteUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to record audit log")
	}

	payloadBytes, err := json.Marshal(map[string]any{
		"user_id":      userID,
		"status":       "deleted",
		"performed_by": performedBy,
		"reason":       reason,
		"deleted_at":   time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to marshal outbox payload in DeleteUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to construct event payload")
	}

	outboxQuery := `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, topic, status, retry_count, created_at)
		VALUES ('user', $1, 'user.deleted', $2, 'user.events', 'PENDING', 0, NOW())
	`
	if _, err := tx.Exec(ctx, outboxQuery, userID, payloadBytes); err != nil {
		slog.ErrorContext(ctx, "failed to insert outbox event in DeleteUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to record outbox event")
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit tx in DeleteUser", "user_id", userID, "error", err)
		return appErrors.Internal(err, "failed to commit transaction")
	}

	slog.InfoContext(ctx, "user deleted and anonymized successfully", "user_id", userID, "performed_by", performedBy)
	return nil
}

func (r *pgUserRepository) GetExpiredDeactivatedUserIDs(ctx context.Context, cutoff time.Time) ([]string, error) {
	query := `
		SELECT id
		FROM users
		WHERE status = 'deactivated'::user_status
		  AND deactivated_at IS NOT NULL
		  AND deactivated_at <= $1
		ORDER BY deactivated_at ASC
	`
	rows, err := r.pool.Query(ctx, query, cutoff)
	if err != nil {
		slog.ErrorContext(ctx, "failed to query expired deactivated users", "cutoff", cutoff, "error", err)
		return nil, appErrors.Internal(err, "failed to query expired deactivated users")
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			slog.ErrorContext(ctx, "failed to scan expired user id", "error", err)
			return nil, appErrors.Internal(err, "failed to scan expired user id")
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *pgUserRepository) GetUserAuditLogs(ctx context.Context, userID string) ([]*model.UserAuditLog, error) {
	query := `
		SELECT id, user_id, action, performed_by, reason, metadata, created_at
		FROM user_audit_logs
		WHERE user_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to query audit logs", "user_id", userID, "error", err)
		return nil, appErrors.Internal(err, "failed to query audit logs")
	}
	defer rows.Close()

	var logs []*model.UserAuditLog
	for rows.Next() {
		var l model.UserAuditLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.PerformedBy, &l.Reason, &l.Metadata, &l.CreatedAt); err != nil {
			slog.ErrorContext(ctx, "failed to scan audit log", "user_id", userID, "error", err)
			return nil, appErrors.Internal(err, "failed to scan audit log")
		}
		logs = append(logs, &l)
	}
	if err := rows.Err(); err != nil {
		slog.ErrorContext(ctx, "row error in audit logs", "user_id", userID, "error", err)
		return nil, appErrors.Internal(err, "row error in audit logs")
	}

	return logs, nil
}
