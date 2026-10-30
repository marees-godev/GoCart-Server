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
	ErrReturnNotFound   = errors.New("return request not found")
	ErrExchangeNotFound = errors.New("exchange record not found")
)

type ReturnRepository interface {
	CreateReturnWithOutbox(ctx context.Context, ret *model.Return, outboxEvent *outbox.Event) error
	GetReturnByID(ctx context.Context, id string) (*model.Return, error)
	GetReturnByOrderItemID(ctx context.Context, orderItemID string) (*model.Return, error)
	ListReturnsByOrderID(ctx context.Context, orderID string) ([]*model.Return, error)
	ListReturns(ctx context.Context, limit, offset int, status, userID, returnType string) ([]*model.Return, int, error)
	UpdateReturnStatus(ctx context.Context, id string, status model.ReturnStatus) (*model.Return, error)
	UpdateReturnStatusWithOutbox(ctx context.Context, id string, status model.ReturnStatus, rejectionReason string, outboxEvents ...*outbox.Event) (*model.Return, error)
	CreateVerificationWithOutbox(ctx context.Context, v *model.ProductVerification, newStatus model.ReturnStatus, outboxEvent *outbox.Event) error
	GetVerificationByReturnID(ctx context.Context, returnID string) (*model.ProductVerification, error)

	CreateExchangeWithOutbox(ctx context.Context, exc *model.Exchange, answers []*model.ExchangeQuestionAnswer, outboxEvent *outbox.Event) error
	GetExchangeByID(ctx context.Context, id string) (*model.Exchange, error)
	GetExchangeByOrderID(ctx context.Context, orderID string) (*model.Exchange, error)
	CreateExchangeVerificationWithOutbox(ctx context.Context, v *model.ExchangeVerification, newStatus model.ExchangeStatus, finalDiscount float64, outboxEvents ...*outbox.Event) error
	GetExchangeVerification(ctx context.Context, exchangeID string) (*model.ExchangeVerification, error)
	GetExchangeQuestionAnswers(ctx context.Context, exchangeID string) ([]*model.ExchangeQuestionAnswer, error)
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
				refund_amount, requested_at, created_at, updated_at, rejection_reason
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
			)
		`
		_, err := tx.Exec(ctx, query,
			ret.ID, ret.OrderID, ret.OrderItemID, ret.UserID, ret.Reason, string(ret.Status), retType,
			ret.RefundAmount, ret.RequestedAt, ret.CreatedAt, ret.UpdatedAt, ret.RejectionReason,
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
		       refund_amount, requested_at, approved_at, completed_at, created_at, updated_at, rejection_reason
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
		       refund_amount, requested_at, approved_at, completed_at, created_at, updated_at, rejection_reason
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
		       refund_amount, requested_at, approved_at, completed_at, created_at, updated_at, rejection_reason
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
		       refund_amount, requested_at, approved_at, completed_at, created_at, updated_at, rejection_reason
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
	return r.UpdateReturnStatusWithOutbox(ctx, id, status, "")
}

func (r *returnRepository) UpdateReturnStatusWithOutbox(ctx context.Context, id string, status model.ReturnStatus, rejectionReason string, outboxEvents ...*outbox.Event) (*model.Return, error) {
	var updatedReturn *model.Return
	err := r.db.WithTransaction(ctx, func(tx pgx.Tx) error {
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
			    rejection_reason = COALESCE(NULLIF($5, ''), rejection_reason),
			    updated_at = $6
			WHERE id = $1
			RETURNING id, order_id, order_item_id, user_id, reason, status, type,
			          refund_amount, requested_at, approved_at, completed_at, created_at, updated_at, rejection_reason
		`
		row := tx.QueryRow(ctx, query, id, string(status), approvedAt, completedAt, rejectionReason, now)
		ret, err := scanReturn(row)
		if err != nil {
			if errors.Is(err, ErrReturnNotFound) {
				r.logger.WarnContext(ctx, "return not found for status update", slog.String("id", id), slog.String("status", string(status)))
			} else {
				r.logger.ErrorContext(ctx, "failed to update return status", slog.String("id", id), slog.String("status", string(status)), slog.Any("error", err))
			}
			return err
		}

		for _, evt := range outboxEvents {
			if evt != nil {
				if err := r.outboxStore.Insert(ctx, tx, evt); err != nil {
					r.logger.ErrorContext(ctx, "failed to insert outbox event on return status update",
						slog.String("return_id", id),
						slog.String("event_type", evt.EventType),
						slog.Any("error", err),
					)
					return fmt.Errorf("failed to insert outbox event: %w", err)
				}
			}
		}

		updatedReturn = ret
		return nil
	})
	if err != nil {
		return nil, err
	}

	v, _ := r.GetVerificationByReturnID(ctx, updatedReturn.ID)
	updatedReturn.Verification = v
	r.logger.InfoContext(ctx, "return status updated in repository", slog.String("id", id), slog.String("status", string(status)))
	return updatedReturn, nil
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
	var rejReason *string
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
		&rejReason,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReturnNotFound
		}
		return nil, fmt.Errorf("failed to scan return row: %w", err)
	}
	if rejReason != nil {
		ret.RejectionReason = *rejReason
	}
	ret.Status = model.ReturnStatus(statusStr)
	if typeStr != "" {
		ret.Type = model.ReturnType(typeStr)
	} else {
		ret.Type = model.ReturnTypeReturn
	}
	return &ret, nil
}

func (r *returnRepository) CreateExchangeWithOutbox(ctx context.Context, exc *model.Exchange, answers []*model.ExchangeQuestionAnswer, outboxEvent *outbox.Event) error {
	return r.db.WithTransaction(ctx, func(tx pgx.Tx) error {
		query := `
			INSERT INTO exchanges (
				id, order_id, order_item_id, user_id, new_product_id,
				old_brand, old_model, old_variant, serial_or_imei,
				base_exchange_value, estimated_discount, final_discount, price_difference,
				status, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9,
				$10, $11, $12, $13,
				$14, $15, $16
			)
		`
		_, err := tx.Exec(ctx, query,
			exc.ID, exc.OrderID, exc.OrderItemID, exc.UserID, exc.NewProductID,
			exc.OldBrand, exc.OldModel, exc.OldVariant, exc.SerialOrIMEI,
			exc.BaseExchangeValue, exc.EstimatedDiscount, exc.FinalDiscount, exc.PriceDifference,
			string(exc.Status), exc.CreatedAt, exc.UpdatedAt,
		)
		if err != nil {
			r.logger.ErrorContext(ctx, "failed to insert exchange", slog.String("exchange_id", exc.ID), slog.Any("error", err))
			return fmt.Errorf("failed to insert exchange: %w", err)
		}

		for _, ans := range answers {
			if ans.ID == "" {
				ans.ID = uuid.New().String()
			}
			ans.ExchangeID = exc.ID
			ans.CreatedAt = exc.CreatedAt
			ansQuery := `
				INSERT INTO exchange_questionnaire_answers (
					id, exchange_id, question_key, answer_value, deduction_amount, created_at
				) VALUES ($1, $2, $3, $4, $5, $6)
			`
			_, err := tx.Exec(ctx, ansQuery, ans.ID, ans.ExchangeID, ans.QuestionKey, ans.AnswerValue, ans.DeductionAmount, ans.CreatedAt)
			if err != nil {
				r.logger.ErrorContext(ctx, "failed to insert exchange questionnaire answer", slog.String("exchange_id", exc.ID), slog.Any("error", err))
				return fmt.Errorf("failed to insert questionnaire answer: %w", err)
			}
		}

		if outboxEvent != nil {
			if err := r.outboxStore.Insert(ctx, tx, outboxEvent); err != nil {
				r.logger.ErrorContext(ctx, "failed to insert exchange outbox event", slog.String("exchange_id", exc.ID), slog.Any("error", err))
				return fmt.Errorf("failed to insert exchange outbox event: %w", err)
			}
		}

		return nil
	})
}

func (r *returnRepository) GetExchangeByID(ctx context.Context, id string) (*model.Exchange, error) {
	query := `
		SELECT id, order_id, order_item_id, user_id, new_product_id,
		       old_brand, old_model, COALESCE(old_variant, ''), COALESCE(serial_or_imei, ''),
		       base_exchange_value, estimated_discount, COALESCE(final_discount, 0.0), COALESCE(price_difference, 0.0),
		       status, created_at, updated_at
		FROM exchanges
		WHERE id = $1
	`
	row := r.db.QueryRow(ctx, query, id)
	exc, err := scanExchange(row)
	if err != nil {
		return nil, err
	}

	answers, _ := r.GetExchangeQuestionAnswers(ctx, exc.ID)
	exc.Answers = answers
	v, _ := r.GetExchangeVerification(ctx, exc.ID)
	exc.Verification = v

	return exc, nil
}

func (r *returnRepository) GetExchangeByOrderID(ctx context.Context, orderID string) (*model.Exchange, error) {
	query := `
		SELECT id, order_id, order_item_id, user_id, new_product_id,
		       old_brand, old_model, COALESCE(old_variant, ''), COALESCE(serial_or_imei, ''),
		       base_exchange_value, estimated_discount, COALESCE(final_discount, 0.0), COALESCE(price_difference, 0.0),
		       status, created_at, updated_at
		FROM exchanges
		WHERE order_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	row := r.db.QueryRow(ctx, query, orderID)
	exc, err := scanExchange(row)
	if err != nil {
		return nil, err
	}

	answers, _ := r.GetExchangeQuestionAnswers(ctx, exc.ID)
	exc.Answers = answers
	v, _ := r.GetExchangeVerification(ctx, exc.ID)
	exc.Verification = v

	return exc, nil
}

func (r *returnRepository) GetExchangeQuestionAnswers(ctx context.Context, exchangeID string) ([]*model.ExchangeQuestionAnswer, error) {
	query := `
		SELECT id, exchange_id, question_key, answer_value, deduction_amount, created_at
		FROM exchange_questionnaire_answers
		WHERE exchange_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.db.Query(ctx, query, exchangeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var answers []*model.ExchangeQuestionAnswer
	for rows.Next() {
		var a model.ExchangeQuestionAnswer
		if err := rows.Scan(&a.ID, &a.ExchangeID, &a.QuestionKey, &a.AnswerValue, &a.DeductionAmount, &a.CreatedAt); err != nil {
			return nil, err
		}
		answers = append(answers, &a)
	}
	return answers, nil
}

func (r *returnRepository) CreateExchangeVerificationWithOutbox(ctx context.Context, v *model.ExchangeVerification, newStatus model.ExchangeStatus, finalDiscount float64, outboxEvents ...*outbox.Event) error {
	return r.db.WithTransaction(ctx, func(tx pgx.Tx) error {
		vQuery := `
			INSERT INTO exchange_verifications (
				id, exchange_id, verified_by, condition_rating, is_approved,
				screen_intact, body_intact, functional, serial_matches, adjusted_discount,
				notes, verified_at, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (exchange_id) DO UPDATE SET
				verified_by = EXCLUDED.verified_by,
				condition_rating = EXCLUDED.condition_rating,
				is_approved = EXCLUDED.is_approved,
				screen_intact = EXCLUDED.screen_intact,
				body_intact = EXCLUDED.body_intact,
				functional = EXCLUDED.functional,
				serial_matches = EXCLUDED.serial_matches,
				adjusted_discount = EXCLUDED.adjusted_discount,
				notes = EXCLUDED.notes,
				verified_at = EXCLUDED.verified_at,
				updated_at = EXCLUDED.updated_at
			RETURNING id
		`
		err := tx.QueryRow(ctx, vQuery,
			v.ID, v.ExchangeID, v.VerifiedBy, string(v.ConditionRating), v.IsApproved,
			v.ScreenIntact, v.BodyIntact, v.Functional, v.SerialMatches, v.AdjustedDiscount,
			v.Notes, v.VerifiedAt, v.CreatedAt, v.UpdatedAt,
		).Scan(&v.ID)
		if err != nil {
			r.logger.ErrorContext(ctx, "failed to insert exchange verification", slog.Any("error", err))
			return fmt.Errorf("failed to insert exchange verification: %w", err)
		}

		if len(v.ProofImages) > 0 {
			_, _ = tx.Exec(ctx, "DELETE FROM exchange_verification_images WHERE verification_id = $1", v.ID)
			for _, imgURL := range v.ProofImages {
				imgQuery := `
					INSERT INTO exchange_verification_images (id, verification_id, image_url, created_at)
					VALUES ($1, $2, $3, $4)
				`
				_, err := tx.Exec(ctx, imgQuery, uuid.New().String(), v.ID, imgURL, v.CreatedAt)
				if err != nil {
					r.logger.ErrorContext(ctx, "failed to insert exchange verification image", slog.Any("error", err))
					return fmt.Errorf("failed to insert verification image: %w", err)
				}
			}
		}

		updateQuery := `
			UPDATE exchanges
			SET status = $2, final_discount = $3, updated_at = $4
			WHERE id = $1
		`
		if _, err := tx.Exec(ctx, updateQuery, v.ExchangeID, string(newStatus), finalDiscount, v.UpdatedAt); err != nil {
			r.logger.ErrorContext(ctx, "failed to update exchange status on verification", slog.Any("error", err))
			return fmt.Errorf("failed to update exchange: %w", err)
		}

		for _, evt := range outboxEvents {
			if evt != nil {
				if err := r.outboxStore.Insert(ctx, tx, evt); err != nil {
					r.logger.ErrorContext(ctx, "failed to insert outbox event on exchange verification", slog.Any("error", err))
					return fmt.Errorf("failed to insert outbox event: %w", err)
				}
			}
		}

		return nil
	})
}

func (r *returnRepository) GetExchangeVerification(ctx context.Context, exchangeID string) (*model.ExchangeVerification, error) {
	query := `
		SELECT id, exchange_id, verified_by, condition_rating, is_approved,
		       screen_intact, body_intact, functional, serial_matches, adjusted_discount,
		       COALESCE(notes, ''), verified_at, created_at, updated_at
		FROM exchange_verifications
		WHERE exchange_id = $1
	`
	row := r.db.QueryRow(ctx, query, exchangeID)
	var v model.ExchangeVerification
	var ratingStr string
	err := row.Scan(
		&v.ID, &v.ExchangeID, &v.VerifiedBy, &ratingStr, &v.IsApproved,
		&v.ScreenIntact, &v.BodyIntact, &v.Functional, &v.SerialMatches, &v.AdjustedDiscount,
		&v.Notes, &v.VerifiedAt, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	v.ConditionRating = model.ExchangeConditionRating(ratingStr)

	imgRows, err := r.db.Query(ctx, "SELECT image_url FROM exchange_verification_images WHERE verification_id = $1", v.ID)
	if err == nil {
		defer imgRows.Close()
		for imgRows.Next() {
			var img string
			if err := imgRows.Scan(&img); err == nil && img != "" {
				v.ProofImages = append(v.ProofImages, img)
			}
		}
	}

	return &v, nil
}

func scanExchange(row pgx.Row) (*model.Exchange, error) {
	var exc model.Exchange
	var statusStr string
	err := row.Scan(
		&exc.ID,
		&exc.OrderID,
		&exc.OrderItemID,
		&exc.UserID,
		&exc.NewProductID,
		&exc.OldBrand,
		&exc.OldModel,
		&exc.OldVariant,
		&exc.SerialOrIMEI,
		&exc.BaseExchangeValue,
		&exc.EstimatedDiscount,
		&exc.FinalDiscount,
		&exc.PriceDifference,
		&statusStr,
		&exc.CreatedAt,
		&exc.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrExchangeNotFound
		}
		return nil, fmt.Errorf("failed to scan exchange row: %w", err)
	}
	exc.Status = model.ExchangeStatus(statusStr)
	return &exc, nil
}

