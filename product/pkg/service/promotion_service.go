package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

type PromotionService struct {
	store ports.PromotionStore
}

func NewPromotionService(store ports.PromotionStore) *PromotionService {
	return &PromotionService{store: store}
}

func (s *PromotionService) List(ctx context.Context) []domain.Promotion {
	promotions, err := s.store.List(ctx)
	if err != nil {
		return nil
	}
	return promotions
}

func (s *PromotionService) Create(ctx context.Context, c domain.Promotion) (*domain.Promotion, error) {
	code := strings.ToUpper(strings.TrimSpace(c.Code))
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}
	c.Code = code
	if err := s.store.Create(ctx, c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *PromotionService) Update(ctx context.Context, code string, discount *float64, description, expires *string, active *bool) (*domain.Promotion, error) {
	return s.store.Update(ctx, code, discount, description, expires, active)
}

func (s *PromotionService) Delete(ctx context.Context, code string) error {
	return s.store.Delete(ctx, code)
}

func (s *PromotionService) Redeem(ctx context.Context, code string) (*domain.Promotion, bool) {
	return s.store.GetActive(ctx, code)
}
