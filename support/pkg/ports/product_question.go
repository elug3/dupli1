package ports

import (
	"context"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
)

// ProductQuestionRepository stores product questions (상품 문의).
type ProductQuestionRepository interface {
	// Save inserts or replaces a question by id.
	Save(ctx context.Context, question *domain.ProductQuestion) error
	// FindByID returns nil, nil when there is no such question.
	FindByID(ctx context.Context, id string) (*domain.ProductQuestion, error)
	// ListByCustomer returns what one shopper asked, newest first; with a
	// productID, only their questions about that product.
	ListByCustomer(ctx context.Context, customerID, productID string) ([]domain.ProductQuestion, error)
	// ListForStaff returns the console's queue.
	ListForStaff(ctx context.Context, filter ProductQuestionFilter) ([]domain.ProductQuestion, error)
	Delete(ctx context.Context, id string) error
	// PurgeBodies replaces the question text of everything asked before the
	// cutoff with placeholder and drops its fit, keeping the row and the
	// answer. Returns how many were purged.
	PurgeBodies(ctx context.Context, olderThan time.Time, placeholder string) (int, error)
}

// Staff queue views.
const (
	QuestionQueueWaiting  = "waiting"
	QuestionQueueAnswered = "answered"
	QuestionQueueHidden   = "hidden"
)

// ProductQuestionFilter narrows the console's queue.
type ProductQuestionFilter struct {
	// Queue is QuestionQueueWaiting (oldest first, so the longest wait is on
	// top), QuestionQueueAnswered or QuestionQueueHidden (newest first).
	Queue     string
	Type      string
	ProductID string
	Limit     int
}
