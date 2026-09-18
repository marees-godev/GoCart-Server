package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
)

type UserRepository interface {
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

func (r *pgUserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	query := `
		SELECT id, username, email, first_name, last_name, phone, alternate_phone, date_of_birth, gender, bio, avatar_url, role, status, created_at, updated_at
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
		&u.Phone,
		&u.AlternatePhone,
		&u.DateOfBirth,
		&u.Gender,
		&u.Bio,
		&u.AvatarURL,
		&u.Role,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("user not found")
		}
		return nil, appErrors.Internal(err, "failed to query user by id")
	}
	return &u, nil
}

func (r *pgUserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `
		SELECT id, username, email, first_name, last_name, phone, alternate_phone, date_of_birth, gender, bio, avatar_url, role, status, created_at, updated_at
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
		&u.Phone,
		&u.AlternatePhone,
		&u.DateOfBirth,
		&u.Gender,
		&u.Bio,
		&u.AvatarURL,
		&u.Role,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("user not found")
		}
		return nil, appErrors.Internal(err, "failed to query user by email")
	}
	return &u, nil
}

func (r *pgUserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	query := `
		SELECT id, username, email, first_name, last_name, phone, alternate_phone, date_of_birth, gender, bio, avatar_url, role, status, created_at, updated_at
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
		&u.Phone,
		&u.AlternatePhone,
		&u.DateOfBirth,
		&u.Gender,
		&u.Bio,
		&u.AvatarURL,
		&u.Role,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appErrors.NotFound("user not found")
		}
		return nil, appErrors.Internal(err, "failed to query user by username")
	}
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
		user.Phone,
		user.AlternatePhone,
		user.DateOfBirth,
		user.Gender,
		user.Bio,
		user.AvatarURL,
		user.ID,
	).Scan(&user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return appErrors.NotFound("user not found")
		}
		return appErrors.Internal(err, "failed to update user profile")
	}
	return nil
}
