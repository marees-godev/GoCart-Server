package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
)

type AuthRepository interface {
	GetByEmail(ctx context.Context, email string) (*model.AuthCredential, error)
	GetByPhone(ctx context.Context, phone string) (*model.AuthCredential, error)
	CreateWithOutbox(ctx context.Context, cred *model.AuthCredential, outboxEvt *outbox.Event) error
}

type authRepository struct {
	db          *database.DB
	outboxStore *outbox.Store
}

func NewAuthRepository(db *database.DB, outboxStore *outbox.Store) AuthRepository {
	return &authRepository{
		db:          db,
		outboxStore: outboxStore,
	}
}

func (r *authRepository) GetByEmail(ctx context.Context, email string) (*model.AuthCredential, error) {
	query := `
		SELECT 
			id, 
			user_id, 
			email, 
			phone, 
			password_hash, 
			role, 
			email_verified, 
			is_active, 
			failed_login_count, 
			locked_until, 
			created_at, 
			updated_at
		FROM auth_credentials
		WHERE email = $1`

	row := r.db.QueryRow(ctx, query, email)
	cred := &model.AuthCredential{}
	err := row.Scan(
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
			return nil, nil
		}
		return nil, err
	}
	return cred, nil
}

func (r *authRepository) GetByPhone(ctx context.Context, phone string) (*model.AuthCredential, error) {
	query := `
		SELECT 
			id, 
			user_id, 
			email, 
			phone, 
			password_hash, 
			role, 
			email_verified, 
			is_active, 
			failed_login_count, 
			locked_until, 
			created_at, 
			updated_at
		FROM auth_credentials
		WHERE phone = $1`

	row := r.db.QueryRow(ctx, query, phone)
	cred := &model.AuthCredential{}
	err := row.Scan(
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
			return nil, nil
		}
		return nil, err
	}
	return cred, nil
}

func (r *authRepository) CreateWithOutbox(ctx context.Context, cred *model.AuthCredential, outboxEvt *outbox.Event) error {
	return r.db.WithTransaction(ctx, func(tx pgx.Tx) error {
		query := `
			INSERT INTO auth_credentials (
				id, 
				user_id, 
				email, 
				phone, 
				password_hash, 
				role, 
				email_verified, 
				is_active, 
				failed_login_count, 
				locked_until, 
				created_at, 
				updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
			)`
		_, err := tx.Exec(ctx, query,
			cred.ID,
			cred.UserID,
			cred.Email,
			cred.Phone,
			cred.PasswordHash,
			cred.Role,
			cred.EmailVerified,
			cred.IsActive,
			cred.FailedLoginCount,
			cred.LockedUntil,
			cred.CreatedAt,
			cred.UpdatedAt,
		)
		if err != nil {
			return err
		}

		if outboxEvt != nil {
			if err := r.outboxStore.Insert(ctx, tx, outboxEvt); err != nil {
				return err
			}
		}

		return nil
	})
}
