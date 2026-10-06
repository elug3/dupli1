package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/ports"
)

// ProductQuestionRepository stores product questions.
type ProductQuestionRepository struct {
	db *sql.DB
}

func NewProductQuestionRepository(db *sql.DB) *ProductQuestionRepository {
	return &ProductQuestionRepository{db: db}
}

const productQuestionColumns = `id, product_id, COALESCE(sku_id, ''), COALESCE(variant_label, ''),
	COALESCE(product_name, ''), customer_id, COALESCE(customer_email, ''), author_mask, type, body,
	fit, secret, status, COALESCE(answer, ''), COALESCE(answered_by, ''), answered_at,
	hidden, COALESCE(hidden_by, ''), hidden_at, created_at, updated_at`

func (r *ProductQuestionRepository) Save(ctx context.Context, q *domain.ProductQuestion) error {
	var fit []byte
	if !q.Fit.IsZero() {
		encoded, err := json.Marshal(q.Fit)
		if err != nil {
			return fmt.Errorf("encode fit: %w", err)
		}
		fit = encoded
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO support_product_questions (
			id, product_id, sku_id, variant_label, product_name, customer_id, customer_email,
			author_mask, type, body, fit, secret, status, answer, answered_by, answered_at,
			hidden, hidden_by, hidden_at, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), $6, NULLIF($7, ''),
			$8, $9, $10, $11, $12, $13, NULLIF($14, ''), NULLIF($15, ''), $16,
			$17, NULLIF($18, ''), $19, $20, $21)
		ON CONFLICT (id) DO UPDATE SET
			sku_id = EXCLUDED.sku_id, variant_label = EXCLUDED.variant_label,
			product_name = EXCLUDED.product_name, customer_email = EXCLUDED.customer_email,
			author_mask = EXCLUDED.author_mask, type = EXCLUDED.type, body = EXCLUDED.body,
			fit = EXCLUDED.fit, secret = EXCLUDED.secret, status = EXCLUDED.status,
			answer = EXCLUDED.answer, answered_by = EXCLUDED.answered_by,
			answered_at = EXCLUDED.answered_at, hidden = EXCLUDED.hidden,
			hidden_by = EXCLUDED.hidden_by, hidden_at = EXCLUDED.hidden_at,
			updated_at = EXCLUDED.updated_at`,
		q.ID, q.ProductID, q.SkuID, q.VariantLabel, q.ProductName, q.CustomerID, q.CustomerEmail,
		q.AuthorMask, q.Type, q.Body, fit, q.Secret, q.Status, q.Answer, q.AnsweredBy, q.AnsweredAt,
		q.Hidden, q.HiddenBy, q.HiddenAt, q.CreatedAt, q.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save product question: %w", err)
	}
	return nil
}

func (r *ProductQuestionRepository) FindByID(ctx context.Context, id string) (*domain.ProductQuestion, error) {
	q, err := scanProductQuestion(r.db.QueryRowContext(ctx,
		`SELECT `+productQuestionColumns+` FROM support_product_questions WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find product question: %w", err)
	}
	return q, nil
}

func (r *ProductQuestionRepository) ListByProduct(ctx context.Context, productID string, includeHidden bool) ([]domain.ProductQuestion, error) {
	return r.query(ctx, `
		SELECT `+productQuestionColumns+`
		  FROM support_product_questions
		 WHERE product_id = $1 AND ($2::boolean OR NOT hidden)
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1000`, productID, includeHidden)
}

func (r *ProductQuestionRepository) ListByCustomer(ctx context.Context, customerID string) ([]domain.ProductQuestion, error) {
	return r.query(ctx, `
		SELECT `+productQuestionColumns+`
		  FROM support_product_questions
		 WHERE customer_id = $1
		 ORDER BY created_at DESC, id DESC
		 LIMIT 500`, customerID)
}

func (r *ProductQuestionRepository) ListForStaff(ctx context.Context, filter ports.ProductQuestionFilter) ([]domain.ProductQuestion, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	where, order := `NOT hidden AND status = 'waiting'`, `created_at ASC, id ASC`
	switch filter.Queue {
	case ports.QuestionQueueAnswered:
		where, order = `NOT hidden AND status = 'answered'`, `answered_at DESC NULLS LAST, id DESC`
	case ports.QuestionQueueHidden:
		where, order = `hidden`, `hidden_at DESC NULLS LAST, id DESC`
	}
	return r.query(ctx, `
		SELECT `+productQuestionColumns+`
		  FROM support_product_questions
		 WHERE `+where+`
		   AND ($1 = '' OR type = $1)
		   AND ($2 = '' OR product_id = $2)
		 ORDER BY `+order+`
		 LIMIT $3`, filter.Type, filter.ProductID, limit)
}

func (r *ProductQuestionRepository) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM support_product_questions WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete product question: %w", err)
	}
	return nil
}

func (r *ProductQuestionRepository) query(ctx context.Context, query string, args ...any) ([]domain.ProductQuestion, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list product questions: %w", err)
	}
	defer rows.Close()
	var out []domain.ProductQuestion
	for rows.Next() {
		q, err := scanProductQuestion(rows)
		if err != nil {
			return nil, fmt.Errorf("scan product question: %w", err)
		}
		out = append(out, *q)
	}
	return out, rows.Err()
}

func scanProductQuestion(row rowScanner) (*domain.ProductQuestion, error) {
	var (
		q          domain.ProductQuestion
		fit        []byte
		answeredAt sql.NullTime
		hiddenAt   sql.NullTime
	)
	err := row.Scan(&q.ID, &q.ProductID, &q.SkuID, &q.VariantLabel, &q.ProductName, &q.CustomerID,
		&q.CustomerEmail, &q.AuthorMask, &q.Type, &q.Body, &fit, &q.Secret, &q.Status, &q.Answer,
		&q.AnsweredBy, &answeredAt, &q.Hidden, &q.HiddenBy, &hiddenAt, &q.CreatedAt, &q.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(fit) > 0 {
		var decoded domain.Fit
		if json.Unmarshal(fit, &decoded) == nil && !decoded.IsZero() {
			q.Fit = &decoded
		}
	}
	if answeredAt.Valid {
		t := answeredAt.Time
		q.AnsweredAt = &t
	}
	if hiddenAt.Valid {
		t := hiddenAt.Time
		q.HiddenAt = &t
	}
	return &q, nil
}
