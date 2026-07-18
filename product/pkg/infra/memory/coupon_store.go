package memory

import (
	"fmt"
	"strings"
	"sync"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

type CouponStore struct {
	mu      sync.RWMutex
	coupons map[string]domain.Coupon
}

func NewCouponStore() *CouponStore {
	s := &CouponStore{coupons: make(map[string]domain.Coupon)}
	s.coupons["SUMMER30"] = domain.Coupon{
		Code:        "SUMMER30",
		Discount:    0.30,
		Description: "Summer sale — all items",
		Expires:     "Aug 31, 2026",
		Active:      true,
	}
	return s
}

func (s *CouponStore) List() ([]domain.Coupon, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Coupon, 0, len(s.coupons))
	for _, c := range s.coupons {
		out = append(out, c)
	}
	return out, nil
}

func (s *CouponStore) Create(c domain.Coupon) error {
	code := strings.ToUpper(strings.TrimSpace(c.Code))
	if code == "" {
		return ports.Invalid("code is required")
	}
	c.Code = code
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.coupons[code]; exists {
		return ports.Conflict("coupon already exists")
	}
	s.coupons[code] = c
	return nil
}

func (s *CouponStore) Update(code string, discount *float64, description, expires *string, active *bool) (*domain.Coupon, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	s.mu.Lock()
	defer s.mu.Unlock()
	c, exists := s.coupons[code]
	if !exists {
		return nil, fmt.Errorf("coupon %s: %w", code, ports.ErrNotFound)
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

func (s *CouponStore) Delete(code string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.coupons[code]; !exists {
		return fmt.Errorf("coupon %s: %w", code, ports.ErrNotFound)
	}
	delete(s.coupons, code)
	return nil
}

func (s *CouponStore) GetActive(code string) (*domain.Coupon, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.coupons[code]
	if !ok || !c.Active {
		return nil, false
	}
	return &c, true
}
