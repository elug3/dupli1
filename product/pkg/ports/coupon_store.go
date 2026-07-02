package ports

import "github.com/elug3/dupli1/product/pkg/domain"

type CouponStore interface {
	List() ([]domain.Coupon, error)
	Create(c domain.Coupon) error
	Update(code string, discount *float64, description, expires *string, active *bool) (*domain.Coupon, error)
	Delete(code string) error
	GetActive(code string) (*domain.Coupon, bool)
}
