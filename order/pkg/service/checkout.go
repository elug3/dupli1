package service

import (
	"context"
	"errors"
	"strings"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/ports"
)

type CreateCheckoutSessionInput struct {
	CustomerID string
}

type CompleteCheckoutResult struct {
	Session *domain.CheckoutSession `json:"session"`
	Order   *domain.Order           `json:"order"`
}

func (s *Service) CreateCheckoutSession(ctx context.Context, input CreateCheckoutSessionInput) (*domain.CheckoutSession, error) {
	sessionID, err := s.repo.NextCheckoutSessionID(ctx)
	if err != nil {
		return nil, err
	}

	session, err := domain.NewCheckoutSession(sessionID, input.CustomerID, s.now(), s.checkoutTTL)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveCheckoutSession(ctx, session); err != nil {
		return nil, err
	}
	return cloneCheckoutSession(session), nil
}

func (s *Service) GetCheckoutSession(ctx context.Context, id string) (*domain.CheckoutSession, error) {
	session, err := s.repo.GetCheckoutSession(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if err := session.EnsureOpen(s.now()); err != nil && !errors.Is(err, domain.ErrSessionNotOpen) {
		_ = s.repo.SaveCheckoutSession(ctx, session)
		return nil, err
	}
	return cloneCheckoutSession(session), nil
}

func (s *Service) SetCheckoutItems(ctx context.Context, sessionID string, items []domain.OrderItem) (*domain.CheckoutSession, error) {
	session, err := s.getOpenCheckoutSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	priced, err := s.priceItems(ctx, items)
	if err != nil {
		return nil, err
	}
	if err := session.SetItems(priced, s.now()); err != nil {
		return nil, err
	}
	return s.saveCheckoutSession(ctx, session)
}

func (s *Service) UpsertCheckoutItem(ctx context.Context, sessionID string, item domain.OrderItem) (*domain.CheckoutSession, error) {
	session, err := s.getOpenCheckoutSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	priced, err := s.priceItems(ctx, []domain.OrderItem{item})
	if err != nil {
		return nil, err
	}
	if err := session.UpsertItem(priced[0], s.now()); err != nil {
		return nil, err
	}
	return s.saveCheckoutSession(ctx, session)
}

func (s *Service) RemoveCheckoutItem(ctx context.Context, sessionID, sku string) (*domain.CheckoutSession, error) {
	session, err := s.getOpenCheckoutSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := session.RemoveItem(sku, s.now()); err != nil {
		return nil, err
	}
	return s.saveCheckoutSession(ctx, session)
}

func (s *Service) RemoveCheckoutItemBySkuID(ctx context.Context, sessionID, skuID string) (*domain.CheckoutSession, error) {
	session, err := s.getOpenCheckoutSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := session.RemoveItemBySkuID(skuID, s.now()); err != nil {
		return nil, err
	}
	return s.saveCheckoutSession(ctx, session)
}

func (s *Service) ApplyCheckoutCoupon(ctx context.Context, sessionID, code string) (*domain.CheckoutSession, error) {
	if s.couponClient == nil {
		return nil, ports.ErrCouponUnavailable
	}

	session, err := s.getOpenCheckoutSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(session.Items) == 0 {
		return nil, domain.ErrEmptyCheckout
	}

	coupon, err := s.couponClient.Redeem(ctx, code)
	if err != nil {
		return nil, err
	}
	if err := session.ApplyCoupon(coupon.Code, coupon.DiscountFraction, s.now()); err != nil {
		return nil, err
	}
	return s.saveCheckoutSession(ctx, session)
}

func (s *Service) CompleteCheckout(ctx context.Context, sessionID string) (*CompleteCheckoutResult, error) {
	session, err := s.getOpenCheckoutSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(session.Items) == 0 {
		return nil, domain.ErrEmptyCheckout
	}

	order, err := s.CreateOrder(ctx, CreateOrderInput{
		CustomerID:    session.CustomerID,
		Items:         cloneOrderItems(session.Items),
		CouponCode:    session.CouponCode,
		DiscountCents: session.DiscountCents,
	})
	if err != nil {
		return nil, err
	}
	if err := session.Complete(order.ID, s.now()); err != nil {
		_, _ = s.CancelOrder(ctx, order.ID)
		return nil, err
	}
	if _, err := s.saveCheckoutSession(ctx, session); err != nil {
		return nil, err
	}

	return &CompleteCheckoutResult{
		Session: cloneCheckoutSession(session),
		Order:   order,
	}, nil
}

func (s *Service) getOpenCheckoutSession(ctx context.Context, sessionID string) (*domain.CheckoutSession, error) {
	session, err := s.repo.GetCheckoutSession(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	if err := session.EnsureOpen(s.now()); err != nil {
		_ = s.repo.SaveCheckoutSession(ctx, session)
		return nil, err
	}
	return session, nil
}

func (s *Service) saveCheckoutSession(ctx context.Context, session *domain.CheckoutSession) (*domain.CheckoutSession, error) {
	if err := s.repo.SaveCheckoutSession(ctx, session); err != nil {
		return nil, err
	}
	return cloneCheckoutSession(session), nil
}

func cloneCheckoutSession(session *domain.CheckoutSession) *domain.CheckoutSession {
	if session == nil {
		return nil
	}
	copied := *session
	copied.Items = cloneOrderItems(session.Items)
	return &copied
}

func cloneOrderItems(items []domain.OrderItem) []domain.OrderItem {
	copied := make([]domain.OrderItem, len(items))
	copy(copied, items)
	return copied
}
