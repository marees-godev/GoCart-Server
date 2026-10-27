package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/model"
)

var (
	ErrReturnNotFound = errors.New("return request not found")
)

type ReturnRepository interface {
	CreateReturnWithOutbox(ctx context.Context, ret *model.Return, outboxEvent *outbox.Event) error
	GetReturnByID(ctx context.Context, id string) (*model.Return, error)
	GetReturnByOrderItemID(ctx context.Context, orderItemID string) (*model.Return, error)
	ListReturnsByOrderID(ctx context.Context, orderID string) ([]*model.Return, error)
	ListReturns(ctx context.Context, limit, offset int, status, userID, returnType string) ([]*model.Return, int, error)
	UpdateReturnStatus(ctx context.Context, id string, status model.ReturnStatus) (*model.Return, error)
	CreateVerificationWithOutbox(ctx context.Context, v *model.ProductVerification, newStatus model.ReturnStatus, outboxEvent *outbox.Event) error
	GetVerificationByReturnID(ctx context.Context, returnID string) (*model.ProductVerification, error)
}

type returnRepository struct {
	db          *database.DB
	outboxStore *outbox.Store
	logger      *slog.Logger
}

func NewReturnRepository(db *database.DB, outboxStore *outbox.Store, log ...*slog.Logger) ReturnRepository {
	var l *slog.Logger
	if len(log) > 0 && log[0] != nil {
		l = log[0]
	} else {
		l = slog.Default()
	}
	return &returnRepository{
		db:          db,
		outboxStore: outboxStore,
		logger:      l,
	}
}

func (r *returnRepository) CreateReturnWithOutbox(ctx context.Context, ret *model.Return, outboxEvent *outbox.Event) error {
	return r.db.WithTransaction(ctx, func(tx pgx.Tx) error {
		retType := string(ret.Type)
		if retType == "" {
			retType = string(model.ReturnTypeReturn)
		}
		query := `
			INSERT INTO returns (
				id, order_id, order_item_id, user_id, reason, status, type,
				refund_amount, requested_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
			)
		`
		_, err := tx.Exec(ctx, query,
			ret.ID, ret.OrderID, ret.OrderItemID, ret.UserID, ret.Reason, string(ret.Status), retType,
			ret.RefundAmount, ret.RequestedAt, ret.CreatedAt, ret.UpdatedAt,
		)
		if err != nil {
			r.logger.ErrorContext(ctx, "failed to insert return",
				slog.String("return_id", ret.ID),
				slog.String("order_id", ret.OrderID),
				slog.String("order_item_id", ret.OrderItemID),
				slog.Any("error", err),
			)
			return fmt.Errorf("failed to insert return: %w", err)
		}

		if outboxEvent != nil {
			if err := r.outboxStore.Insert(ctx, tx, outboxEvent); err != nil {
				r.logger.ErrorContext(ctx, "failed to insert outbox event",
					slog.String("return_id", ret.ID),
					slog.String("event_type", outboxEvent.EventType),
					slog.Any("error", err),
				)
				return fmt.Errorf("failed to insert outbox event: %w", err)
			}
		}

		r.logger.DebugContext(ctx, "return and outbox event created successfully",
			slog.String("return_id", ret.ID),
			slog.String("order_id", ret.OrderID),
		)
		return nil
	})
}

func (r *returnRepository) GetReturnByID(ctx context.Context, id string) (*model.Return, error) {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return nil, ErrReturnNotFound
	}
	query := `
		SELECT id, order_id, order_item_id, user_id, reason, status, type,
		       refund_amount, requested_at, approved_at, completed_at, created_at, updated_at
		FROM returns
		WHERE id = $1
	`
	row := r.db.Pool.QueryRow(ctx, query, id)
	ret, err := scanReturn(row)
	if err != nil {
		if errors.Is(err, ErrReturnNotFound) {
			r.logger.DebugContext(ctx, "return not found by id", slog.String("id", id))
		} else {
			r.logger.ErrorContext(ctx, "failed to query return by id", slog.String("id", id), slog.Any("error", err))
		}
		return nil, err
	}
	v, _ := r.GetVerificationByReturnID(ctx, ret.ID)
	ret.Verification = v
	return ret, nil
}

func (r *returnRepository) GetReturnByOrderItemID(ctx context.Context, orderItemID string) (*model.Return, error) {
	if _, err := uuid.Parse(strings.TrimSpace(orderItemID)); err != nil {
		return nil, ErrReturnNotFound
	}
	query := `
		SELECT id, order_id, order_item_id, user_id, reason, status, type,
		       refund_amount, requested_at, approved_at, completed_at, created_at, updated_at
		FROM returns
		WHERE order_item_id = $1
	`
	row := r.db.Pool.QueryRow(ctx, query, orderItemID)
	ret, err := scanReturn(row)
	if err != nil {
		if errors.Is(err, ErrReturnNotFound) {
			r.logger.DebugContext(ctx, "return not found by order item id", slog.String("order_item_id", orderItemID))
		} else {
			r.logger.ErrorContext(ctx, "failed to query return by order item id", slog.String("order_item_id", orderItemID), slog.Any("error", err))
		}
		return nil, err
	}
	v, _ := r.GetVerificationByReturnID(ctx, ret.ID)
	ret.Verification = v
	return ret, nil
}

func (r *returnRepository) ListReturnsByOrderID(ctx context.Context, orderID string) ([]*model.Return, error) {
	if _, err := uuid.Parse(strings.TrimSpace(orderID)); err != nil {
		return []*model.Return{}, nil
	}
	query := `
		SELECT id, order_id, order_item_id, user_id, reason, status, type,
		       refund_amount, requested_at, approved_at, completed_at, created_at, updated_at
		FROM returns
		WHERE order_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.Pool.Query(ctx, query, orderID)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to query returns by order id", slog.String("order_id", orderID), slog.Any("error", err))
		return nil, fmt.Errorf("failed to query returns: %w", err)
	}
	defer rows.Close()

	var returnsList []*model.Return
	for rows.Next() {
		ret, err := scanReturn(rows)
		if err != nil {
			r.logger.ErrorContext(ctx, "failed to scan return row in list", slog.String("order_id", orderID), slog.Any("error", err))
			return nil, err
		}
		v, _ := r.GetVerificationByReturnID(ctx, ret.ID)
		ret.Verification = v
		returnsList = append(returnsList, ret)
	}
	if err := rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "row iteration error listing returns", slog.String("order_id", orderID), slog.Any("error", err))
		return nil, err
	}
	return returnsList, nil
}

func (r *returnRepository) ListReturns(ctx context.Context, limit, offset int, status, userID, returnType string) ([]*model.Return, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var conditions []string
	var args []any
	argIdx := 1

	if status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}
	if userID != "" {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
		args = append(args, userID)
		argIdx++
	}
	if returnType != "" {
		conditions = append(conditions, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, returnType)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM returns %s", whereClause)
	var total int
	if err := r.db.Pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		r.logger.ErrorContext(ctx, "failed to count returns", slog.Any("error", err))
		return nil, 0, fmt.Errorf("failed to count returns: %w", err)
	}

	listQuery := fmt.Sprintf(`
		SELECT id, order_id, order_item_id, user_id, reason, status, type,
		       refund_amount, requested_at, approved_at, completed_at, created_at, updated_at
		FROM returns
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)
	rows, err := r.db.Pool.Query(ctx, listQuery, args...)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to list returns", slog.Any("error", err))
		return nil, 0, fmt.Errorf("failed to list returns: %w", err)
	}
	defer rows.Close()

	var returnsList []*model.Return
	for rows.Next() {
		ret, err := scanReturn(rows)
		if err != nil {
			return nil, 0, err
		}
		v, _ := r.GetVerificationByReturnID(ctx, ret.ID)
		ret.Verification = v
		returnsList = append(returnsList, ret)
	}
	return returnsList, total, nil
}

func (r *returnRepository) UpdateReturnStatus(ctx context.Context, id string, status model.ReturnStatus) (*model.Return, error) {
	now := time.Now().UTC()
	var approvedAt, completedAt *time.Time
	if status == model.ReturnStatusApproved {
		approvedAt = &now
	}
	if status == model.ReturnStatusCompleted {
		completedAt = &now
	}

	query := `
		UPDATE returns
		SET status = $2,
		    approved_at = COALESCE($3, approved_at),
		    completed_at = COALESCE($4, completed_at),
		    updated_at = $5
		WHERE id = $1
		RETURNING id, order_id, order_item_id, user_id, reason, status, type,
		          refund_amount, requested_at, approved_at, completed_at, created_at, updated_at
	`
	row := r.db.Pool.QueryRow(ctx, query, id, string(status), approvedAt, completedAt, now)
	ret, err := scanReturn(row)
	if err != nil {
		if errors.Is(err, ErrReturnNotFound) {
			r.logger.WarnContext(ctx, "return not found for status update", slog.String("id", id), slog.String("status", string(status)))
		} else {
			r.logger.ErrorContext(ctx, "failed to update return status", slog.String("id", id), slog.String("status", string(status)), slog.Any("error", err))
		}
		return nil, err
	}
	v, _ := r.GetVerificationByReturnID(ctx, ret.ID)
	ret.Verification = v
	r.logger.InfoContext(ctx, "return status updated in repository", slog.String("id", id), slog.String("status", string(status)))
	return ret, nil
}

func (r *returnRepository) CreateVerificationWithOutbox(ctx context.Context, v *model.ProductVerification, newStatus model.ReturnStatus, outboxEvent *outbox.Event) error {
	return r.db.WithTransaction(ctx, func(tx pgx.Tx) error {
		vQuery := `
			INSERT INTO return_verifications (
				id, return_id, verified_by, condition, is_approved, notes, verified_at, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (return_id) DO UPDATE SET
				verified_by = EXCLUDED.verified_by,
				condition = EXCLUDED.condition,
				is_approved = EXCLUDED.is_approved,
				notes = EXCLUDED.notes,
				verified_at = EXCLUDED.verified_at,
				updated_at = EXCLUDED.updated_at
			RETURNING id
		`
		err := tx.QueryRow(ctx, vQuery,
			v.ID, v.ReturnID, v.VerifiedBy, string(v.Condition), v.IsApproved, v.Notes, v.VerifiedAt, v.CreatedAt, v.UpdatedAt,
		).Scan(&v.ID)
		if err != nil {
			r.logger.ErrorContext(ctx, "failed to insert verification", slog.Any("error", err))
			return fmt.Errorf("failed to insert verification: %w", err)
		}

		if len(v.ProofImages) > 0 {
			_, _ = tx.Exec(ctx, "DELETE FROM return_verification_images WHERE verification_id = $1", v.ID)
			for _, imgURL := range v.ProofImages {
				imgQuery := `
					INSERT INTO return_verification_images (id, verification_id, image_url, created_at)
					VALUES ($1, $2, $3, $4)
				`
				if _, err := tx.Exec(ctx, imgQuery, uuid.New().String(), v.ID, imgURL, v.CreatedAt); err != nil {
					return fmt.Errorf("failed to insert verification image: %w", err)
				}
			}
		}

		updateQuery := `
			UPDATE returns
			SET status = $2, updated_at = $3
			WHERE id = $1
		`
		if _, err := tx.Exec(ctx, updateQuery, v.ReturnID, string(newStatus), time.Now().UTC()); err != nil {
			return fmt.Errorf("failed to update return status during verification: %w", err)
		}

		if outboxEvent != nil {
			if err := r.outboxStore.Insert(ctx, tx, outboxEvent); err != nil {
				return fmt.Errorf("failed to insert outbox event: %w", err)
			}
		}
		return nil
	})
}

func (r *returnRepository) GetVerificationByReturnID(ctx context.Context, returnID string) (*model.ProductVerification, error) {
	if _, err := uuid.Parse(strings.TrimSpace(returnID)); err != nil {
		return nil, nil
	}
	query := `
		SELECT id, return_id, verified_by, condition, is_approved, notes, verified_at, created_at, updated_at
		FROM return_verifications
		WHERE return_id = $1
	`
	var v model.ProductVerification
	var condStr string
	err := r.db.Pool.QueryRow(ctx, query, returnID).Scan(
		&v.ID, &v.ReturnID, &v.VerifiedBy, &condStr, &v.IsApproved, &v.Notes, &v.VerifiedAt, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	v.Condition = model.VerificationCondition(condStr)

	imgQuery := `
		SELECT image_url
		FROM return_verification_images
		WHERE verification_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.db.Pool.Query(ctx, imgQuery, v.ID)
	if err != nil {
		return &v, nil
	}
	defer rows.Close()

	var images []string
	for rows.Next() {
		var img string
		if err := rows.Scan(&img); err == nil {
			images = append(images, img)
		}
	}
	v.ProofImages = images
	return &v, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanReturn(s rowScanner) (*model.Return, error) {
	var ret model.Return
	var statusStr, typeStr string
	err := s.Scan(
		&ret.ID,
		&ret.OrderID,
		&ret.OrderItemID,
		&ret.UserID,
		&ret.Reason,
		&statusStr,
		&typeStr,
		&ret.RefundAmount,
		&ret.RequestedAt,
		&ret.ApprovedAt,
		&ret.CompletedAt,
		&ret.CreatedAt,
		&ret.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReturnNotFound
		}
		return nil, fmt.Errorf("failed to scan return row: %w", err)
	}
	ret.Status = model.ReturnStatus(statusStr)
	if typeStr != "" {
		ret.Type = model.ReturnType(typeStr)
	} else {
		ret.Type = model.ReturnTypeReturn
	}
	return &ret, nil
}

