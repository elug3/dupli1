package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/ports"
)

// ProductQuestionRepository keeps product questions in a map.
type ProductQuestionRepository struct {
	mu   sync.RWMutex
	rows map[string]domain.ProductQuestion
}

func NewProductQuestionRepository() *ProductQuestionRepository {
	return &ProductQuestionRepository{rows: make(map[string]domain.ProductQuestion)}
}

func (r *ProductQuestionRepository) Save(_ context.Context, question *domain.ProductQuestion) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := *question
	if question.Fit != nil {
		fit := *question.Fit
		row.Fit = &fit
	}
	r.rows[question.ID] = row
	return nil
}

func (r *ProductQuestionRepository) FindByID(_ context.Context, id string) (*domain.ProductQuestion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	row, ok := r.rows[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (r *ProductQuestionRepository) ListByCustomer(_ context.Context, customerID, productID string) ([]domain.ProductQuestion, error) {
	return r.list(func(q domain.ProductQuestion) bool {
		return q.CustomerID == customerID && (productID == "" || q.ProductID == productID)
	}, false), nil
}

func (r *ProductQuestionRepository) ListForStaff(_ context.Context, filter ports.ProductQuestionFilter) ([]domain.ProductQuestion, error) {
	out := r.list(func(q domain.ProductQuestion) bool {
		if filter.Type != "" && q.Type != filter.Type {
			return false
		}
		if filter.ProductID != "" && q.ProductID != filter.ProductID {
			return false
		}
		switch filter.Queue {
		case ports.QuestionQueueHidden:
			return q.Hidden
		case ports.QuestionQueueAnswered:
			return !q.Hidden && q.Status == domain.QuestionAnswered
		default:
			return !q.Hidden && q.Status == domain.QuestionWaiting
		}
	}, filter.Queue == "" || filter.Queue == ports.QuestionQueueWaiting)
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

func (r *ProductQuestionRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, id)
	return nil
}

func (r *ProductQuestionRepository) PurgeBodies(_ context.Context, olderThan time.Time, placeholder string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	purged := 0
	for id, row := range r.rows {
		if !row.CreatedAt.Before(olderThan) || row.Body == placeholder || row.Body == domain.ForgottenQuestionBody {
			continue
		}
		row.Body, row.Fit = placeholder, nil
		r.rows[id] = row
		purged++
	}
	return purged, nil
}

func (r *ProductQuestionRepository) list(keep func(domain.ProductQuestion) bool, oldestFirst bool) []domain.ProductQuestion {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []domain.ProductQuestion
	for _, row := range r.rows {
		if keep(row) {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID != oldestFirst
		}
		return out[i].CreatedAt.After(out[j].CreatedAt) != oldestFirst
	})
	return out
}
