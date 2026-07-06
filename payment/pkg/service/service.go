package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/ports"
)

type Service struct {
	repo      ports.Repository
	orders    ports.OrderClient
	checkout  ports.CheckoutProvider
	events    ports.EventPublisher
	now       func() time.Time
}

func New(repo ports.Repository, orders ports.OrderClient, checkout ports.CheckoutProvider, events ports.EventPublisher) *Service {
	return &Service{
		repo:     repo,
		orders:   orders,
		checkout: checkout,
		events:   events,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

type CreatePaymentInput struct {
	OrderID        string
	CustomerID     string
	BearerToken    string
	IdempotencyKey string
}

func (s *Service) CreatePayment(ctx context.Context, input CreatePaymentInput) (*domain.Payment, error) {
	if input.IdempotencyKey != "" {
		if existing, err := s.repo.FindByIdempotencyKey(ctx, input.IdempotencyKey); err == nil {
			return existing, nil
		} else if !errors.Is(err, ports.ErrNotFound) {
			return nil, err
		}
	}

	order, err := s.orders.GetOrder(ctx, input.BearerToken, input.OrderID)
	if err != nil {
		return nil, err
	}
	if order.CustomerID != input.CustomerID {
		return nil, ports.ErrOrderForbidden
	}
	if order.Status != "pending" {
		return nil, ports.ErrOrderNotPending
	}

	paymentID, err := s.repo.NextPaymentID(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	session, err := s.checkout.CreateSession(ctx, ports.CheckoutSessionInput{
		OrderID:     order.ID,
		PaymentID:   paymentID,
		AmountCents: order.TotalCents,
		Currency:    "usd",
		CustomerID:  order.CustomerID,
	})
	if err != nil {
		return nil, err
	}

	provider := "stripe"
	if strings.HasPrefix(session.ProviderRef, "dev_") {
		provider = "dev"
	}

	payment, err := domain.NewPayment(paymentID, order.ID, order.CustomerID, order.TotalCents, "usd", provider, session.ProviderRef, session.CheckoutURL, now)
	if err != nil {
		return nil, err
	}
	payment.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if err := s.repo.Save(ctx, payment); err != nil {
		return nil, err
	}
	return payment, nil
}

func (s *Service) GetPayment(ctx context.Context, paymentID, customerID string) (*domain.Payment, error) {
	payment, err := s.repo.Get(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	if customerID != "" && payment.CustomerID != customerID {
		return nil, ports.ErrOrderForbidden
	}
	return payment, nil
}

func (s *Service) CompletePayment(ctx context.Context, paymentID string) (*domain.Payment, error) {
	payment, err := s.repo.Get(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	if payment.Status == domain.StatusSucceeded {
		return payment, nil
	}
	now := s.now()
	payment.MarkSucceeded(now)
	if err := s.repo.Save(ctx, payment); err != nil {
		return nil, err
	}
	if s.events != nil {
		if err := s.events.Publish(ctx, ports.PaymentSucceededSubject, ports.PaymentSucceededEvent{
			EventType:   ports.PaymentSucceededSubject,
			OrderID:     payment.OrderID,
			PaymentID:   payment.ID,
			AmountCents: payment.AmountCents,
		}); err != nil {
			return nil, err
		}
	}
	return payment, nil
}

func (s *Service) HandleStripeCheckoutCompleted(ctx context.Context, sessionID, orderID, paymentID string, amountTotal int64) error {
	payment, err := s.repo.Get(ctx, paymentID)
	if err != nil {
		payment, err = s.repo.GetByProviderRef(ctx, sessionID)
		if err != nil {
			return err
		}
	}
	if amountTotal > 0 && amountTotal != payment.AmountCents {
		return domain.ErrInvalidPayment
	}
	_, err = s.CompletePayment(ctx, payment.ID)
	return err
}
