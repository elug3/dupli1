package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/elug3/dupli1/support/pkg/domain"
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

// SeedAnswers writes the default copy for any node that has none.
//
// ON CONFLICT DO NOTHING is the whole point: staff edit this copy from the
// manager inbox, and a deploy that overwrote their wording on every restart
// would make the editor pointless.
func SeedAnswers(ctx context.Context, db *sql.DB, language string) (int, error) {
	const query = `
		INSERT INTO support_answers (node, language, body)
		VALUES ($1, $2, $3)
		ON CONFLICT (node, language) DO NOTHING`

	seeded := 0
	for node, body := range domain.DefaultAnswers {
		result, err := db.ExecContext(ctx, query, node, language, body)
		if err != nil {
			return seeded, fmt.Errorf("seed support answer %q: %w", node, err)
		}
		if rows, err := result.RowsAffected(); err == nil {
			seeded += int(rows)
		}
	}
	return seeded, nil
}
