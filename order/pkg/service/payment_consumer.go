package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/events"
)

// RegisterPaymentConsumer subscribes to payment.succeeded and marks orders paid.
func (s *Service) RegisterPaymentConsumer(ctx context.Context, subscriber ports.EventSubscriber) error {
	return subscriber.Subscribe(ctx, paymentSucceededSubject, s.handlePaymentSucceeded)
}

func (s *Service) handlePaymentSucceeded(ctx context.Context, _ string, payload []byte) error {
	var event events.PaymentSucceededEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payment.succeeded: %w", err)
	}
	if event.OrderID == "" || event.PaymentID == "" {
		return fmt.Errorf("payment.succeeded missing order_id or payment_id")
	}
	_, err := s.MarkOrderPaid(ctx, event.OrderID, event.PaymentID, event.AmountCents)
	if err != nil {
		return fmt.Errorf("mark order paid order_id=%s payment_id=%s: %w", event.OrderID, event.PaymentID, err)
	}
	return nil
}

// StartPendingExpiryWorker cancels unpaid pending orders past payment_due_at.
func (s *Service) StartPendingExpiryWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.expirePendingOrders(ctx); err != nil {
					log.Printf("expire pending orders: %v", err)
				}
			}
		}
	}()
}

func (s *Service) expirePendingOrders(ctx context.Context) error {
	orders, err := s.repo.ListPendingPaymentExpired(ctx, s.now())
	if err != nil {
		return err
	}
	for _, order := range orders {
		if err := s.cancelExpiredPendingOrder(ctx, order.ID); err != nil {
			log.Printf("cancel expired order %s: %v", order.ID, err)
		}
	}
	return nil
}

// cancelExpiredPendingOrder cancels an unpaid pending order past payment_due_at.
// Uses an atomic status guard so a payment that completes concurrently cannot be undone.
func (s *Service) cancelExpiredPendingOrder(ctx context.Context, orderID string) error {
	now := s.now()
	order, err := s.repo.Get(ctx, orderID)
	if err != nil {
		return err
	}
	if order.Status != domain.StatusPending || !order.IsPaymentExpired(now) {
		return nil
	}

	cancelled := cloneOrder(order)
	if err := cancelled.Cancel(now); err != nil {
		return err
	}
	events, err := s.outboxEvents(cancelled, orderUpdatedSubject)
	if err != nil {
		return err
	}

	canceledOrder, didCancel, err := s.repo.CancelIfPendingExpired(ctx, orderID, now, events)
	if err != nil || !didCancel {
		return err
	}
	s.tryDrainOutbox(ctx)
	if err := s.releaseReservationForCancel(ctx, canceledOrder.ReservationID); err != nil {
		log.Printf("cancel expired order %s: release reservation %s: %v", orderID, canceledOrder.ReservationID, err)
	}
	return nil
}

// RegisterPaymentCanceledConsumer subscribes to payment.canceled so a refunded
// order stops looking shippable.
//
// Without it the two services each knew half the story: payment moved to
// canceled while the order stayed paid, which left it in the fulfillment queue
// and passing Ship's status check. Shipping then committed the reservation and
// sent goods for an order that had already been refunded.
func (s *Service) RegisterPaymentCanceledConsumer(ctx context.Context, subscriber ports.EventSubscriber) error {
	return subscriber.Subscribe(ctx, paymentCanceledSubject, s.handlePaymentCanceled)
}

func (s *Service) handlePaymentCanceled(ctx context.Context, _ string, payload []byte) error {
	var event events.PaymentCanceledEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payment.canceled: %w", err)
	}
	if event.OrderID == "" || event.PaymentID == "" {
		return fmt.Errorf("payment.canceled missing order_id or payment_id")
	}
	return s.CancelOrderForRefund(ctx, event.OrderID, event.PaymentID, event.RemainingCents)
}

// CancelOrderForRefund cancels an order whose payment was fully refunded.
//
// Only a full refund cancels: a partial one (remainingCents > 0) leaves money
// still owed on goods the customer has not been made whole for, and silently
// cancelling that is worse than leaving it for a human. Likewise an order
// already shipped — the goods are gone, so this is a return, which the system
// has no concept of yet. Both cases are logged loudly and left alone.
//
// Idempotent: a replayed event finds the order already canceled and no-ops,
// matching how MarkOrderPaid tolerates redelivery.
func (s *Service) CancelOrderForRefund(ctx context.Context, orderID, paymentID string, remainingCents int64) error {
	order, err := s.repo.Get(ctx, strings.TrimSpace(orderID))
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			log.Printf("payment.canceled: unknown order %s (payment %s)", orderID, paymentID)
			return nil
		}
		return err
	}

	if remainingCents > 0 {
		log.Printf(
			"payment.canceled: order %s partially refunded (payment %s, %d still captured); "+
				"leaving status %s for manual review",
			order.ID, paymentID, remainingCents, order.Status,
		)
		return nil
	}

	switch order.Status {
	case domain.StatusCanceled:
		return nil // replay, or already canceled by an operator
	case domain.StatusInTransit, domain.StatusFulfilled:
		log.Printf(
			"payment.canceled: order %s was refunded (payment %s) but is already %s; "+
				"goods have shipped, so this needs a return rather than a cancel",
			order.ID, paymentID, order.Status,
		)
		return nil
	}

	if err := order.Cancel(s.now()); err != nil {
		return fmt.Errorf("cancel refunded order %s: %w", order.ID, err)
	}
	saved, err := s.saveStatusChange(ctx, order)
	if err != nil {
		return fmt.Errorf("save refunded order %s: %w", order.ID, err)
	}
	if err := s.releaseReservationForCancel(ctx, saved.ReservationID); err != nil {
		log.Printf("payment.canceled: order %s release reservation %s: %v", saved.ID, saved.ReservationID, err)
	}
	log.Printf("payment.canceled: order %s canceled after full refund (payment %s)", saved.ID, paymentID)
	return nil
}
