package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
