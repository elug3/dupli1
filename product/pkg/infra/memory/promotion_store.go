package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

type PromotionStore struct {
	mu      sync.RWMutex
	promotions map[string]domain.Promotion
}

func NewPromotionStore() *PromotionStore {
	s := &PromotionStore{promotions: make(map[string]domain.Promotion)}
	s.promotions["SUMMER30"] = domain.Promotion{
		Code:        "SUMMER30",
		Discount:    0.30,
		Description: "Summer sale — all items",
		Expires:     "Aug 31, 2026",
		Active:      true,
	}
	return s
}

func (s *PromotionStore) List(ctx context.Context, ) ([]domain.Promotion, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Promotion, 0, len(s.promotions))
	for _, c := range s.promotions {
		out = append(out, c)
	}
	return out, nil
}

func (s *PromotionStore) Create(ctx context.Context, c domain.Promotion) error {
	_ = ctx
	code := strings.ToUpper(strings.TrimSpace(c.Code))
	if code == "" {
		return ports.Invalid("code is required")
	}
	c.Code = code
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.promotions[code]; exists {
		return ports.Conflict("promotion already exists")
	}
	s.promotions[code] = c
	return nil
}

func (s *PromotionStore) Update(ctx context.Context, code string, discount *float64, description, expires *string, active *bool) (*domain.Promotion, error) {
	_ = ctx
	code = strings.ToUpper(strings.TrimSpace(code))
	s.mu.Lock()
	defer s.mu.Unlock()
	c, exists := s.promotions[code]
	if !exists {
		return nil, fmt.Errorf("promotion %s: %w", code, ports.ErrNotFound)
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
	s.promotions[code] = c
	return &c, nil
}

func (s *PromotionStore) Delete(ctx context.Context, code string) error {
	_ = ctx
	code = strings.ToUpper(strings.TrimSpace(code))
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.promotions[code]; !exists {
		return fmt.Errorf("promotion %s: %w", code, ports.ErrNotFound)
	}
	delete(s.promotions, code)
	return nil
}

func (s *PromotionStore) GetActive(ctx context.Context, code string) (*domain.Promotion, bool) {
	_ = ctx
	code = strings.ToUpper(strings.TrimSpace(code))
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.promotions[code]
	if !ok || !c.Active {
		return nil, false
	}
	return &c, true
}
