package memory

import (
	"context"
	"strings"
	"sync"
)

// AnswerRepository keeps staff-edited copy in memory, for local dev and tests
// without a database. Like the Postgres table it starts empty: nodes no one
// edited take domain.DefaultAnswers. Edits are not persisted — that needs
// Postgres.
type AnswerRepository struct {
	mu   sync.RWMutex
	rows map[string]string
}

func NewAnswerRepository() *AnswerRepository {
	return &AnswerRepository{rows: map[string]string{}}
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
