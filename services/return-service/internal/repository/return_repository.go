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
	CreateVerificationWithOutbox(ctx context.Context, v *model.ProductVerification, newStatus model.ReturnStatus, outboxEvents ...*outbox.Event) error
	GetVerificationByReturnID(ctx context.Context, returnID string) (*model.ProductVerification, error)

	CreateExchangeWithOutbox(ctx context.Context, exc *model.Exchange, answers []*model.ExchangeQuestionAnswer, outboxEvent *outbox.Event) error
	GetExchangeByID(ctx context.Context, id string) (*model.Exchange, error)
	GetExchangeByOrderID(ctx context.Context, orderID string) (*model.Exchange, error)
	CreateExchangeVerificationWithOutbox(ctx context.Context, v *model.ExchangeVerification, newStatus model.ExchangeStatus, finalDiscount float64, outboxEvents ...*outbox.Event) error
	GetExchangeVerification(ctx context.Context, exchangeID string) (*model.ExchangeVerification, error)
	GetExchangeQuestionAnswers(ctx context.Context, exchangeID string) ([]*model.ExchangeQuestionAnswer, error)

	ListExchangeDiagnosticQuestions(ctx context.Context, categoryCode string) ([]*model.ExchangeDiagnosticQuestion, error)
	ListExchangeDeviceCatalog(ctx context.Context, categoryCode string) ([]*model.ExchangeDeviceCatalog, error)
	GetExchangeDeviceByModel(ctx context.Context, categoryCode, brand, modelName, variant string) (*model.ExchangeDeviceCatalog, error)
	GetExchangeDeviceCatalogByID(ctx context.Context, id string) (*model.ExchangeDeviceCatalog, error)
	CreateExchangeQuote(ctx context.Context, quote *model.ExchangeQuote, answers []*model.ExchangeQuoteAnswer) error
	GetExchangeQuoteByID(ctx context.Context, id string) (*model.ExchangeQuote, error)
	UpdateExchangeQuoteStatus(ctx context.Context, id string, status model.ExchangeQuoteStatus) error
	ApplyExchangeQuoteToOrderWithOutbox(ctx context.Context, exc *model.Exchange, quoteID string, answers []*model.ExchangeQuestionAnswer, outboxEvent *outbox.Event) error
	UpdateExchangeStatusAndDiscountWithOutbox(ctx context.Context, exchangeID string, status model.ExchangeStatus, finalDiscount, priceDiff float64, outboxEvent *outbox.Event) error
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
			approvedAt = &now
		}

		query := `
			UPDATE returns
			SET status = $2,
			    approved_at = COALESCE(approved_at, $3),
			    completed_at = COALESCE(completed_at, $4),
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

func (r *returnRepository) CreateVerificationWithOutbox(ctx context.Context, v *model.ProductVerification, newStatus model.ReturnStatus, outboxEvents ...*outbox.Event) error {
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

		_, _ = tx.Exec(ctx, "DELETE FROM return_verification_images WHERE verification_id = $1", v.ID)
		if len(v.ProofImages) > 0 {
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

		now := time.Now().UTC()
		var approvedAt *time.Time
		if v.IsApproved {
			approvedAt = &now
		}
		updateQuery := `
			UPDATE returns
			SET status = $2,
			    approved_at = COALESCE(approved_at, $3),
			    updated_at = $4
			WHERE id = $1
		`
		if _, err := tx.Exec(ctx, updateQuery, v.ReturnID, string(newStatus), approvedAt, now); err != nil {
			return fmt.Errorf("failed to update return status during verification: %w", err)
		}

		for _, outboxEvent := range outboxEvents {
			if outboxEvent != nil {
				if err := r.outboxStore.Insert(ctx, tx, outboxEvent); err != nil {
					return fmt.Errorf("failed to insert outbox event: %w", err)
				}
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
		var quoteUUID *string
		if exc.QuoteID != "" {
			quoteUUID = &exc.QuoteID
		}
		var newProdUUID *string
		if exc.NewProductID != "" {
			newProdUUID = &exc.NewProductID
		}
		var orderUUID *string
		if exc.OrderID != "" {
			orderUUID = &exc.OrderID
		}
		var orderItemUUID *string
		if exc.OrderItemID != "" {
			orderItemUUID = &exc.OrderItemID
		}
		var productUUID *string
		if exc.ProductID != "" {
			productUUID = &exc.ProductID
		}
		var devCatUUID *string
		if exc.ExchangeDeviceCatalogID != "" {
			devCatUUID = &exc.ExchangeDeviceCatalogID
		}
		query := `
			INSERT INTO exchanges (
				id, order_id, order_item_id, user_id, new_product_id,
				old_brand, old_model, old_variant, serial_or_imei,
				base_exchange_value, estimated_discount, final_discount, price_difference,
				status, quote_id, product_id, exchange_device_catalog_id, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9,
				$10, $11, $12, $13,
				$14, $15, $16, $17, $18, $19
			)
		`
		_, err := tx.Exec(ctx, query,
			exc.ID, orderUUID, orderItemUUID, exc.UserID, newProdUUID,
			exc.OldBrand, exc.OldModel, exc.OldVariant, exc.SerialOrIMEI,
			exc.BaseExchangeValue, exc.EstimatedDiscount, exc.FinalDiscount, exc.PriceDifference,
			string(exc.Status), quoteUUID, productUUID, devCatUUID, exc.CreatedAt, exc.UpdatedAt,
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
		       status, created_at, updated_at, COALESCE(quote_id::text, ''),
		       COALESCE(product_id::text, ''), COALESCE(exchange_device_catalog_id::text, '')
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
		       status, created_at, updated_at, COALESCE(quote_id::text, ''),
		       COALESCE(product_id::text, ''), COALESCE(exchange_device_catalog_id::text, '')
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
	var quoteID *string
	var newProductID *string
	var orderID *string
	var orderItemID *string
	var productID *string
	var devCatID *string
	err := row.Scan(
		&exc.ID,
		&orderID,
		&orderItemID,
		&exc.UserID,
		&newProductID,
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
		&quoteID,
		&productID,
		&devCatID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrExchangeNotFound
		}
		return nil, fmt.Errorf("failed to scan exchange row: %w", err)
	}
	exc.Status = model.ExchangeStatus(statusStr)
	if orderID != nil {
		exc.OrderID = *orderID
	}
	if orderItemID != nil {
		exc.OrderItemID = *orderItemID
	}
	if quoteID != nil {
		exc.QuoteID = *quoteID
	}
	if newProductID != nil {
		exc.NewProductID = *newProductID
	}
	if productID != nil {
		exc.ProductID = *productID
	}
	if devCatID != nil {
		exc.ExchangeDeviceCatalogID = *devCatID
	}
	return &exc, nil
}

func (r *returnRepository) ListExchangeDiagnosticQuestions(ctx context.Context, categoryCode string) ([]*model.ExchangeDiagnosticQuestion, error) {
	categoryCode = strings.ToUpper(strings.TrimSpace(categoryCode))
	qQuery := `
		SELECT id, category_code, question_key, question_text, display_order, created_at
		FROM exchange_diagnostic_questions
		WHERE category_code = $1
		ORDER BY display_order ASC
	`
	rows, err := r.db.Query(ctx, qQuery, categoryCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var questions []*model.ExchangeDiagnosticQuestion
	questionMap := make(map[string]*model.ExchangeDiagnosticQuestion)

	for rows.Next() {
		var q model.ExchangeDiagnosticQuestion
		if err := rows.Scan(&q.ID, &q.CategoryCode, &q.QuestionKey, &q.QuestionText, &q.DisplayOrder, &q.CreatedAt); err != nil {
			return nil, err
		}
		q.Options = []*model.ExchangeQuestionOption{}
		questions = append(questions, &q)
		questionMap[q.ID] = &q
	}

	if len(questions) == 0 {
		return questions, nil
	}

	optQuery := `
		SELECT id, question_id, option_key, option_label, deduction_amount, is_fatal_disqualifier, COALESCE(rejection_reason, ''), display_order, created_at
		FROM exchange_question_options
		WHERE question_id = ANY($1)
		ORDER BY display_order ASC
	`
	qIDs := make([]string, 0, len(questions))
	for _, q := range questions {
		qIDs = append(qIDs, q.ID)
	}

	optRows, err := r.db.Query(ctx, optQuery, qIDs)
	if err == nil {
		defer optRows.Close()
		for optRows.Next() {
			var opt model.ExchangeQuestionOption
			if err := optRows.Scan(&opt.ID, &opt.QuestionID, &opt.OptionKey, &opt.OptionLabel, &opt.DeductionAmount, &opt.IsFatalDisqualifier, &opt.RejectionReason, &opt.DisplayOrder, &opt.CreatedAt); err == nil {
				if parent, exists := questionMap[opt.QuestionID]; exists {
					parent.Options = append(parent.Options, &opt)
				}
			}
		}
	}

	return questions, nil
}

func (r *returnRepository) ListExchangeDeviceCatalog(ctx context.Context, categoryCode string) ([]*model.ExchangeDeviceCatalog, error) {
	categoryCode = strings.ToUpper(strings.TrimSpace(categoryCode))
	query := `
		SELECT id, category_code, brand, model, variant, base_exchange_value, is_active, created_at
		FROM exchange_device_catalog
		WHERE category_code = $1 AND is_active = TRUE
		ORDER BY brand ASC, model ASC, variant ASC
	`
	rows, err := r.db.Query(ctx, query, categoryCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []*model.ExchangeDeviceCatalog
	for rows.Next() {
		var d model.ExchangeDeviceCatalog
		if err := rows.Scan(&d.ID, &d.CategoryCode, &d.Brand, &d.Model, &d.Variant, &d.BaseExchangeValue, &d.IsActive, &d.CreatedAt); err != nil {
			return nil, err
		}
		devices = append(devices, &d)
	}
	return devices, nil
}

func (r *returnRepository) GetExchangeDeviceByModel(ctx context.Context, categoryCode, brand, modelName, variant string) (*model.ExchangeDeviceCatalog, error) {
	query := `
		SELECT id, category_code, brand, model, variant, base_exchange_value, is_active, created_at
		FROM exchange_device_catalog
		WHERE UPPER(category_code) = UPPER($1)
		  AND UPPER(brand) = UPPER($2)
		  AND UPPER(model) = UPPER($3)
		  AND (variant = '' OR UPPER(variant) = UPPER($4) OR $4 = '')
		  AND is_active = TRUE
		ORDER BY variant DESC
		LIMIT 1
	`
	row := r.db.QueryRow(ctx, query, categoryCode, brand, modelName, variant)
	var d model.ExchangeDeviceCatalog
	err := row.Scan(&d.ID, &d.CategoryCode, &d.Brand, &d.Model, &d.Variant, &d.BaseExchangeValue, &d.IsActive, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

func (r *returnRepository) GetExchangeDeviceCatalogByID(ctx context.Context, id string) (*model.ExchangeDeviceCatalog, error) {
	query := `
		SELECT id, category_code, brand, model, variant, base_exchange_value, is_active, created_at
		FROM exchange_device_catalog
		WHERE id = $1
	`
	row := r.db.QueryRow(ctx, query, id)
	var d model.ExchangeDeviceCatalog
	err := row.Scan(&d.ID, &d.CategoryCode, &d.Brand, &d.Model, &d.Variant, &d.BaseExchangeValue, &d.IsActive, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

func (r *returnRepository) CreateExchangeQuote(ctx context.Context, quote *model.ExchangeQuote, answers []*model.ExchangeQuoteAnswer) error {
	return r.db.WithTransaction(ctx, func(tx pgx.Tx) error {
		query := `
			INSERT INTO exchange_quotes (
				id, user_id, new_product_id, category_code, old_brand, old_model, old_variant,
				serial_or_imei, base_exchange_value, total_deductions, final_discount,
				is_disqualified, disqualification_reason, status, expires_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
			)
		`
		_, err := tx.Exec(ctx, query,
			quote.ID, quote.UserID, quote.NewProductID, quote.CategoryCode, quote.OldBrand, quote.OldModel, quote.OldVariant,
			quote.SerialOrIMEI, quote.BaseExchangeValue, quote.TotalDeductions, quote.FinalDiscount,
			quote.IsDisqualified, quote.DisqualificationReason, string(quote.Status), quote.ExpiresAt, quote.CreatedAt, quote.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert exchange quote: %w", err)
		}

		for _, ans := range answers {
			if ans.ID == "" {
				ans.ID = uuid.New().String()
			}
			ans.QuoteID = quote.ID
			ans.CreatedAt = quote.CreatedAt
			ansQuery := `
				INSERT INTO exchange_quote_answers (
					id, quote_id, question_key, option_key, deduction_amount, is_fatal, created_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7)
			`
			_, err := tx.Exec(ctx, ansQuery, ans.ID, ans.QuoteID, ans.QuestionKey, ans.OptionKey, ans.DeductionAmount, ans.IsFatal, ans.CreatedAt)
			if err != nil {
				return fmt.Errorf("failed to insert exchange quote answer: %w", err)
			}
		}

		return nil
	})
}

func (r *returnRepository) GetExchangeQuoteByID(ctx context.Context, id string) (*model.ExchangeQuote, error) {
	query := `
		SELECT id, user_id, new_product_id, category_code, old_brand, old_model, COALESCE(old_variant, ''),
		       COALESCE(serial_or_imei, ''), base_exchange_value, total_deductions, final_discount,
		       is_disqualified, COALESCE(disqualification_reason, ''), status, expires_at, created_at, updated_at
		FROM exchange_quotes
		WHERE id = $1
	`
	row := r.db.QueryRow(ctx, query, id)
	var q model.ExchangeQuote
	var statusStr string
	err := row.Scan(
		&q.ID, &q.UserID, &q.NewProductID, &q.CategoryCode, &q.OldBrand, &q.OldModel, &q.OldVariant,
		&q.SerialOrIMEI, &q.BaseExchangeValue, &q.TotalDeductions, &q.FinalDiscount,
		&q.IsDisqualified, &q.DisqualificationReason, &statusStr, &q.ExpiresAt, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("exchange quote not found")
		}
		return nil, err
	}
	q.Status = model.ExchangeQuoteStatus(statusStr)

	ansQuery := `
		SELECT id, quote_id, question_key, option_key, deduction_amount, is_fatal, created_at
		FROM exchange_quote_answers
		WHERE quote_id = $1
	`
	rows, err := r.db.Query(ctx, ansQuery, q.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var ans model.ExchangeQuoteAnswer
			if err := rows.Scan(&ans.ID, &ans.QuoteID, &ans.QuestionKey, &ans.OptionKey, &ans.DeductionAmount, &ans.IsFatal, &ans.CreatedAt); err == nil {
				q.Answers = append(q.Answers, &ans)
			}
		}
	}

	return &q, nil
}

func (r *returnRepository) UpdateExchangeQuoteStatus(ctx context.Context, id string, status model.ExchangeQuoteStatus) error {
	query := `
		UPDATE exchange_quotes
		SET status = $2, updated_at = NOW()
		WHERE id = $1
	`
	_, err := r.db.Exec(ctx, query, id, string(status))
	return err
}

func (r *returnRepository) ApplyExchangeQuoteToOrderWithOutbox(ctx context.Context, exc *model.Exchange, quoteID string, answers []*model.ExchangeQuestionAnswer, outboxEvent *outbox.Event) error {
	return r.db.WithTransaction(ctx, func(tx pgx.Tx) error {
		if quoteID != "" {
			updateQuote := `
				UPDATE exchange_quotes
				SET status = 'APPLIED', updated_at = $2
				WHERE id = $1
			`
			if _, err := tx.Exec(ctx, updateQuote, quoteID, exc.CreatedAt); err != nil {
				return fmt.Errorf("failed to update quote status to APPLIED: %w", err)
			}
		}

		query := `
			INSERT INTO exchanges (
				id, order_id, order_item_id, user_id, new_product_id,
				old_brand, old_model, old_variant, serial_or_imei,
				base_exchange_value, estimated_discount, final_discount, price_difference,
				status, quote_id, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9,
				$10, $11, $12, $13,
				$14, $15, $16, $17
			)
		`
		var quoteUUID *string
		if quoteID != "" {
			quoteUUID = &quoteID
		}

		_, err := tx.Exec(ctx, query,
			exc.ID, exc.OrderID, exc.OrderItemID, exc.UserID, exc.NewProductID,
			exc.OldBrand, exc.OldModel, exc.OldVariant, exc.SerialOrIMEI,
			exc.BaseExchangeValue, exc.EstimatedDiscount, exc.FinalDiscount, exc.PriceDifference,
			string(exc.Status), quoteUUID, exc.CreatedAt, exc.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert exchange record: %w", err)
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
			if _, err := tx.Exec(ctx, ansQuery, ans.ID, ans.ExchangeID, ans.QuestionKey, ans.AnswerValue, ans.DeductionAmount, ans.CreatedAt); err != nil {
				return fmt.Errorf("failed to insert questionnaire answer: %w", err)
			}
		}

		if outboxEvent != nil {
			if err := r.outboxStore.Insert(ctx, tx, outboxEvent); err != nil {
				return fmt.Errorf("failed to insert outbox event: %w", err)
			}
		}

		return nil
	})
}

func (r *returnRepository) UpdateExchangeStatusAndDiscountWithOutbox(ctx context.Context, exchangeID string, status model.ExchangeStatus, finalDiscount, priceDiff float64, outboxEvent *outbox.Event) error {
	return r.db.WithTransaction(ctx, func(tx pgx.Tx) error {
		query := `
			UPDATE exchanges
			SET status = $2, final_discount = $3, price_difference = $4, updated_at = NOW()
			WHERE id = $1
		`
		if _, err := tx.Exec(ctx, query, exchangeID, string(status), finalDiscount, priceDiff); err != nil {
			return fmt.Errorf("failed to update exchange status and discount: %w", err)
		}

		if outboxEvent != nil {
			if err := r.outboxStore.Insert(ctx, tx, outboxEvent); err != nil {
				return fmt.Errorf("failed to insert outbox event: %w", err)
			}
		}
		return nil
	})
}
