package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
			// Succeeded payments (e.g. Bypass) re-enqueue so a prior save+failed-publish
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

	if err := s.persistSucceeded(ctx, payment); err != nil {
		return nil, err
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
		payment.MarkSucceeded(s.now())
	}
	if err := s.persistSucceeded(ctx, payment); err != nil {
		return nil, err
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

// persistSucceeded saves the payment and enqueues payment.succeeded in one transaction,
// then best-effort drains the outbox (soft-success: save wins even if NATS is down).
func (s *Service) persistSucceeded(ctx context.Context, payment *domain.Payment) error {
	events, err := s.paymentSucceededOutbox(payment)
	if err != nil {
		return err
	}
	if err := s.repo.SaveWithOutbox(ctx, payment, events); err != nil {
		return err
	}
	s.tryDrainOutbox(ctx)
	return nil
}

func (s *Service) paymentSucceededOutbox(payment *domain.Payment) ([]ports.OutboxEvent, error) {
	payload, err := json.Marshal(ports.PaymentSucceededEvent{
		EventType:   ports.PaymentSucceededSubject,
		OrderID:     payment.OrderID,
		PaymentID:   payment.ID,
		AmountCents: payment.AmountCents,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal payment.succeeded: %w", err)
	}
	return []ports.OutboxEvent{{
		AggregateID: payment.ID,
		Subject:     ports.PaymentSucceededSubject,
		Payload:     payload,
	}}, nil
}

// StartOutboxWorker periodically publishes pending outbox rows.
func (s *Service) StartOutboxWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.DrainOutbox(ctx); err != nil {
					log.Printf("payment outbox drain: %v", err)
				}
			}
		}
	}()
}

// StartReconcileWorker re-publishes recent succeeded payments so order can catch
// up if a prior Core NATS delivery was lost after publish (MarkOrderPaid is idempotent).
func (s *Service) StartReconcileWorker(ctx context.Context, interval, lookback time.Duration) {
	if interval <= 0 {
		interval = 1 * time.Minute
	}
	if lookback <= 0 {
		lookback = 2 * time.Hour
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.ReconcileSucceededPayments(ctx, lookback); err != nil {
					log.Printf("payment succeed reconcile: %v", err)
				}
			}
		}
	}()
}

func (s *Service) tryDrainOutbox(ctx context.Context) {
	if err := s.DrainOutbox(ctx); err != nil {
		log.Printf("payment outbox drain: %v", err)
	}
}

// DrainOutbox publishes pending outbox messages. Failures are recorded and retried later.
func (s *Service) DrainOutbox(ctx context.Context) error {
	if s.events == nil {
		msgs, err := s.repo.ListPendingOutbox(ctx, 100)
		if err != nil {
			return err
		}
		for _, msg := range msgs {
			if err := s.repo.MarkOutboxPublished(ctx, msg.ID); err != nil {
				return err
			}
		}
		return nil
	}

	msgs, err := s.repo.ListPendingOutbox(ctx, 50)
	if err != nil {
		return err
	}
	var firstErr error
	for _, msg := range msgs {
		var event ports.PaymentSucceededEvent
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			_ = s.repo.RecordOutboxAttempt(ctx, msg.ID, err.Error())
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := s.events.Publish(ctx, msg.Subject, event); err != nil {
			_ = s.repo.RecordOutboxAttempt(ctx, msg.ID, err.Error())
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := s.repo.MarkOutboxPublished(ctx, msg.ID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ReconcileSucceededPayments republishes payment.succeeded for recent succeeded rows.
func (s *Service) ReconcileSucceededPayments(ctx context.Context, lookback time.Duration) error {
	if s.events == nil {
		return nil
	}
	if lookback <= 0 {
		lookback = 2 * time.Hour
	}
	payments, err := s.repo.ListSucceededSince(ctx, s.now().Add(-lookback), 100)
	if err != nil {
		return err
	}
	var firstErr error
	for i := range payments {
		p := payments[i]
		if err := s.events.Publish(ctx, ports.PaymentSucceededSubject, ports.PaymentSucceededEvent{
			EventType:   ports.PaymentSucceededSubject,
			OrderID:     p.OrderID,
			PaymentID:   p.ID,
			AmountCents: p.AmountCents,
		}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
