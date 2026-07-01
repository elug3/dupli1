package service

import (
	"fmt"
	"strings"
	"sync"

	"github.com/elug3/dupli1/product/pkg/domain"
)

type CouponService struct {
	mu      sync.RWMutex
	coupons map[string]domain.Coupon
}

func NewCouponService() *CouponService {
	s := &CouponService{coupons: make(map[string]domain.Coupon)}
	s.coupons["SUMMER30"] = domain.Coupon{
		Code:        "SUMMER30",
		Discount:    0.30,
		Description: "Summer sale — all items",
		Expires:     "Aug 31, 2026",
		Active:      true,
	}
	return s
}

func (s *CouponService) List() []domain.Coupon {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Coupon, 0, len(s.coupons))
	for _, c := range s.coupons {
		out = append(out, c)
	}
	return out
}

func (s *CouponService) Create(c domain.Coupon) (*domain.Coupon, error) {
	code := strings.ToUpper(strings.TrimSpace(c.Code))
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}
	c.Code = code
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.coupons[code]; exists {
		return nil, fmt.Errorf("coupon already exists")
	}
	s.coupons[code] = c
	return &c, nil
}

func (s *CouponService) Update(code string, discount *float64, description, expires *string, active *bool) (*domain.Coupon, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	s.mu.Lock()
	defer s.mu.Unlock()
	c, exists := s.coupons[code]
	if !exists {
		return nil, fmt.Errorf("coupon not found")
	}
	if discount != nil {
		c.Discount = *discount
	}
	if description != nil {
		c.Description = *description
	}
	if expires != nil {
		c.Expires = *expires
	}
	if active != nil {
		c.Active = *active
	}
	s.coupons[code] = c
	return &c, nil
}

func (s *CouponService) Delete(code string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.coupons[code]; !exists {
		return fmt.Errorf("coupon not found")
	}
	delete(s.coupons, code)
	return nil
}

// Redeem looks up an active coupon by code (case-insensitive).
func (s *CouponService) Redeem(code string) (*domain.Coupon, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.coupons[code]
	if !ok || !c.Active {
		return nil, false
	}
	return &c, true
}
