package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/elug3/dupli1/order/pkg/ports"
)

type paymentSucceededEvent struct {
	EventType   string `json:"event_type"`
	OrderID     string `json:"order_id"`
	PaymentID   string `json:"payment_id"`
	AmountCents int64  `json:"amount_cents"`
}

// RegisterPaymentConsumer subscribes to payment.succeeded and marks orders paid.
func (s *Service) RegisterPaymentConsumer(ctx context.Context, subscriber ports.EventSubscriber) error {
	return subscriber.Subscribe(ctx, paymentSucceededSubject, s.handlePaymentSucceeded)
}

func (s *Service) handlePaymentSucceeded(ctx context.Context, _ string, payload []byte) error {
	var event paymentSucceededEvent
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
		if _, err := s.CancelOrder(ctx, order.ID); err != nil {
			log.Printf("cancel expired order %s: %v", order.ID, err)
		}
	}
	return nil
}
