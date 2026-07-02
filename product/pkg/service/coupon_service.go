package service

import (
	"fmt"
	"strings"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

type CouponService struct {
	store ports.CouponStore
}

func NewCouponService(store ports.CouponStore) *CouponService {
	return &CouponService{store: store}
}

func (s *CouponService) List() []domain.Coupon {
	coupons, err := s.store.List()
	if err != nil {
		return nil
	}
	return coupons
}

func (s *CouponService) Create(c domain.Coupon) (*domain.Coupon, error) {
	code := strings.ToUpper(strings.TrimSpace(c.Code))
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}
	c.Code = code
	if err := s.store.Create(c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *CouponService) Update(code string, discount *float64, description, expires *string, active *bool) (*domain.Coupon, error) {
	return s.store.Update(code, discount, description, expires, active)
}

func (s *CouponService) Delete(code string) error {
	return s.store.Delete(code)
}

func (s *CouponService) Redeem(code string) (*domain.Coupon, bool) {
	return s.store.GetActive(code)
}
