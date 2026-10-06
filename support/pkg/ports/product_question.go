package ports

import (
	"context"

	"github.com/elug3/dupli1/support/pkg/domain"
)

// ProductQuestionRepository stores product questions (상품 문의).
type ProductQuestionRepository interface {
	// Save inserts or replaces a question by id.
	Save(ctx context.Context, question *domain.ProductQuestion) error
	// FindByID returns nil, nil when there is no such question.
	FindByID(ctx context.Context, id string) (*domain.ProductQuestion, error)
	// ListByProduct returns a product's questions, newest first, hidden ones
	// included only when includeHidden. A product collects tens of questions,
	// not thousands, so filtering and paging happen in the service.
	ListByProduct(ctx context.Context, productID string, includeHidden bool) ([]domain.ProductQuestion, error)
	// ListByCustomer returns what one shopper asked, newest first.
	ListByCustomer(ctx context.Context, customerID string) ([]domain.ProductQuestion, error)
	// ListForStaff returns the console's queue.
	ListForStaff(ctx context.Context, filter ProductQuestionFilter) ([]domain.ProductQuestion, error)
	Delete(ctx context.Context, id string) error
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
