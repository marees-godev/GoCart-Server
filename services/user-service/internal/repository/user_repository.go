package repository

import (
	"context"
	"errors"
	"log/slog"

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
		SELECT id, username, email, first_name, last_name, phone, alternate_phone, date_of_birth, gender, bio, avatar_url, status, created_at, updated_at
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
		SELECT id, username, email, first_name, last_name, phone, alternate_phone, date_of_birth, gender, bio, avatar_url, status, created_at, updated_at
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
		SELECT id, username, email, first_name, last_name, phone, alternate_phone, date_of_birth, gender, bio, avatar_url, status, created_at, updated_at
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
