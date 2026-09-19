package memory

import (
	"context"
	"sync"

	"github.com/elug3/dupli1/support/pkg/domain"
)

// AnswerRepository serves the seeded copy from memory, for local dev and tests
// without a database. Edits are not persisted anywhere — that needs Postgres.
type AnswerRepository struct {
	mu   sync.RWMutex
	rows map[string]string
}

// NewAnswerRepository starts from the seeded default copy.
func NewAnswerRepository() *AnswerRepository {
	rows := make(map[string]string, len(domain.DefaultAnswers))
	for node, body := range domain.DefaultAnswers {
		rows[key(node, domain.DefaultLanguage)] = body
	}
	return &AnswerRepository{rows: rows}
}

func (r *AnswerRepository) Body(_ context.Context, node, language string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.rows[key(node, language)], nil
}

func key(node, language string) string { return node + "\x00" + language }
