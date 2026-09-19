package memory

import (
	"context"
	"strings"
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

// All returns every node's copy for a language, for the editor.
func (r *AnswerRepository) All(_ context.Context, language string) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := map[string]string{}
	suffix := "\x00" + language
	for k, body := range r.rows {
		if node, found := strings.CutSuffix(k, suffix); found {
			out[node] = body
		}
	}
	return out, nil
}

// Put replaces one node's copy. Edits live only as long as the process does —
// persisting them needs Postgres.
func (r *AnswerRepository) Put(_ context.Context, node, language, body, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rows[key(node, language)] = body
	return nil
}
