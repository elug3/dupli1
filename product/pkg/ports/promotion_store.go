package ports

import (
	"context"

	"github.com/elug3/dupli1/product/pkg/domain"
)

type PromotionStore interface {
	List(ctx context.Context) ([]domain.Promotion, error)
	Create(ctx context.Context, c domain.Promotion) error
	Update(ctx context.Context, code string, discount *float64, description, expires *string, active *bool) (*domain.Promotion, error)
	Delete(ctx context.Context, code string) error
	GetActive(ctx context.Context, code string) (*domain.Promotion, bool)
}
