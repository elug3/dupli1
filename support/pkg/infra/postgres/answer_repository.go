package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// AnswerRepository reads the copy each menu node opens with.
type AnswerRepository struct {
	db *sql.DB
}

func NewAnswerRepository(db *sql.DB) *AnswerRepository {
	return &AnswerRepository{db: db}
}

func (r *AnswerRepository) Body(ctx context.Context, node, language string) (string, error) {
	const query = `SELECT body FROM support_answers WHERE node = $1 AND language = $2`

	var body string
	err := r.db.QueryRowContext(ctx, query, node, language).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load support answer: %w", err)
	}
	return body, nil
}

// All returns every node's copy for a language, for the editor.
func (r *AnswerRepository) All(ctx context.Context, language string) (map[string]string, error) {
	const query = `SELECT node, body FROM support_answers WHERE language = $1`

	rows, err := r.db.QueryContext(ctx, query, language)
	if err != nil {
		return nil, fmt.Errorf("list support answers: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var node, body string
		if err := rows.Scan(&node, &body); err != nil {
			return nil, fmt.Errorf("scan support answer: %w", err)
		}
		out[node] = body
	}
	return out, rows.Err()
}

// Put replaces one node's copy, recording who changed it.
func (r *AnswerRepository) Put(ctx context.Context, node, language, body, updatedBy string) error {
	const query = `
		INSERT INTO support_answers (node, language, body, updated_by, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), now())
		ON CONFLICT (node, language) DO UPDATE SET
			body       = EXCLUDED.body,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at`

	if _, err := r.db.ExecContext(ctx, query, node, language, body, updatedBy); err != nil {
		return fmt.Errorf("save support answer: %w", err)
	}
	return nil
}
