package ports

import (
	"context"

	"github.com/elug3/dupli1/product/pkg/domain"
)

type CouponStore interface {
	List(ctx context.Context) ([]domain.Coupon, error)
	Create(ctx context.Context, c domain.Coupon) error
	Update(ctx context.Context, code string, discount *float64, description, expires *string, active *bool) (*domain.Coupon, error)
	Delete(ctx context.Context, code string) error
	GetActive(ctx context.Context, code string) (*domain.Coupon, bool)
}
