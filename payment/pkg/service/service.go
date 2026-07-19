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
	repo     ports.Repository
	orders   ports.OrderClient
	checkout ports.CheckoutProvider
	events   ports.EventPublisher
	now      func() time.Time
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
	Method         string
	Note           string
	CreatedBy      string
	// BypassABAC skips customer ownership check (payment.create / admin / *).
	BypassABAC bool
	// AllowMethodBypass permits method=bypass (payment.bypass / admin / *).
	AllowMethodBypass bool
}

func (s *Service) CreatePayment(ctx context.Context, input CreatePaymentInput) (*domain.Payment, error) {
	if input.IdempotencyKey != "" {
		if existing, err := s.repo.FindByIdempotencyKey(ctx, input.IdempotencyKey); err == nil {
			// Succeeded payments (e.g. Bypass) republish so a prior save+failed-publish
			// retry still notifies order.
			if existing.Status == domain.StatusSucceeded {
				return s.CompletePayment(ctx, existing.ID)
			}
			return existing, nil
		} else if !errors.Is(err, ports.ErrNotFound) {
			return nil, err
		}
	}

	method, err := domain.NormalizeMethod(input.Method)
	if err != nil {
		return nil, err
	}
	switch method {
	case domain.MethodBitcoin:
		return nil, ports.ErrMethodUnavailable
	case domain.MethodBypass:
		return s.createBypassPayment(ctx, input)
	default:
		return s.createCardPayment(ctx, input)
	}
}

func (s *Service) createCardPayment(ctx context.Context, input CreatePaymentInput) (*domain.Payment, error) {
	order, err := s.loadPendingOrder(ctx, input, false)
	if err != nil {
		return nil, err
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
		Currency:    domain.DefaultCurrency,
		CustomerID:  order.CustomerID,
	})
	if err != nil {
		return nil, err
	}

	provider := domain.ProviderStripe
	if strings.HasPrefix(session.ProviderRef, "dev_") {
		provider = domain.ProviderDev
	}

	payment, err := domain.NewPayment(paymentID, order.ID, order.CustomerID, order.TotalCents, domain.DefaultCurrency, provider, session.ProviderRef, session.CheckoutURL, now)
	if err != nil {
		return nil, err
	}
	payment.Method = domain.MethodCreditCard
	if input.CreatedBy != "" {
		payment.CreatedBy = input.CreatedBy
	}
	payment.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if err := s.repo.Save(ctx, payment); err != nil {
		return nil, err
	}
	return payment, nil
}

func (s *Service) createBypassPayment(ctx context.Context, input CreatePaymentInput) (*domain.Payment, error) {
	if !input.AllowMethodBypass {
		return nil, ports.ErrPaymentForbidden
	}

	order, err := s.loadPendingOrder(ctx, input, true)
	if err != nil {
		return nil, err
	}

	paymentID, err := s.repo.NextPaymentID(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	providerRef := "bypass_" + paymentID
	payment, err := domain.NewPayment(paymentID, order.ID, order.CustomerID, order.TotalCents, domain.DefaultCurrency, domain.ProviderBypass, providerRef, "", now)
	if err != nil {
		return nil, err
	}
	payment.Method = domain.MethodBypass
	payment.CreatedBy = strings.TrimSpace(input.CreatedBy)
	payment.Note = strings.TrimSpace(input.Note)
	payment.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
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

// loadPendingOrder fetches the order and enforces pending + ownership rules.
// skipABAC forces ownership skip (used for Bypass, which is manager-only).
func (s *Service) loadPendingOrder(ctx context.Context, input CreatePaymentInput, skipABAC bool) (*ports.OrderSummary, error) {
	order, err := s.orders.GetOrder(ctx, input.BearerToken, input.OrderID)
	if err != nil {
		return nil, err
	}
	if !skipABAC && !input.BypassABAC && order.CustomerID != input.CustomerID {
		return nil, ports.ErrOrderForbidden
	}
	if order.Status != "pending" {
		return nil, ports.ErrOrderNotPending
	}
	return order, nil
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
	if payment.Status != domain.StatusSucceeded {
		now := s.now()
		payment.MarkSucceeded(now)
		if err := s.repo.Save(ctx, payment); err != nil {
			return nil, err
		}
	}
	// Always (re)publish when succeeded so a prior save+failed-publish retry
	// still notifies order. MarkOrderPaid is idempotent for the same payment.
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
