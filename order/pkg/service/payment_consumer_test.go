package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/ports"
)

type expiryStock struct {
	reservationID string
	released      string
}

func (f *expiryStock) Reserve(_ context.Context, _ string, _ []ports.StockItem) (string, error) {
	if f.reservationID == "" {
		f.reservationID = "res-expiry"
	}
	return f.reservationID, nil
}

func (f *expiryStock) CommitReservation(_ context.Context, _ string) error { return nil }

func (f *expiryStock) ReleaseReservation(_ context.Context, reservationID string) error {
	f.released = reservationID
	return nil
}

func TestCancelExpiredPendingOrderSkipsPaidOrder(t *testing.T) {
	ctx := context.Background()
	stock := &expiryStock{reservationID: "res-expiry"}
	repo := memory.NewRepository()
	svc := New(repo, stock)

	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	order, err := domain.NewOrder("ord_expiry_1", "customer-1", "res-expiry", []domain.OrderItem{
		{SKU: "BAG-1", Quantity: 1, UnitPriceCents: 5000},
	}, "", 0, now.Add(-10*time.Minute))
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	order.PaymentDueAt = now.Add(-time.Minute)
	if err := order.MarkPaid("pay-1", order.TotalCents, now.Add(-30*time.Second)); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := svc.cancelExpiredPendingOrder(ctx, order.ID); err != nil {
		t.Fatalf("cancelExpiredPendingOrder: %v", err)
	}
	if stock.released != "" {
		t.Fatalf("released reservation %q on paid order", stock.released)
	}
	stored, err := repo.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Status != domain.StatusPaid {
		t.Fatalf("status = %q, want paid", stored.Status)
	}
}

func TestCancelExpiredPendingOrderCancelsUnpaidPending(t *testing.T) {
	ctx := context.Background()
	stock := &expiryStock{reservationID: "res-expiry"}
	repo := memory.NewRepository()
	svc := New(repo, stock)

	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	order, err := domain.NewOrder("ord_expiry_2", "customer-1", "res-expiry", []domain.OrderItem{
		{SKU: "BAG-2", Quantity: 1, UnitPriceCents: 3000},
	}, "", 0, now.Add(-10*time.Minute))
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	order.PaymentDueAt = now.Add(-time.Minute)
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := svc.cancelExpiredPendingOrder(ctx, order.ID); err != nil {
		t.Fatalf("cancelExpiredPendingOrder: %v", err)
	}
	if stock.released != "res-expiry" {
		t.Fatalf("released = %q, want res-expiry", stock.released)
	}
	stored, err := repo.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Status != domain.StatusCanceled {
		t.Fatalf("status = %q, want canceled", stored.Status)
	}
}

// paidDuringExpiryRepo simulates MarkOrderPaid completing while expiry tries to cancel.
type paidDuringExpiryRepo struct {
	*memory.Repository
}

func (r *paidDuringExpiryRepo) CancelIfPendingExpired(ctx context.Context, orderID string, now time.Time, events []ports.OutboxEvent) (*domain.Order, bool, error) {
	order, err := r.Get(ctx, orderID)
	if err != nil {
		return nil, false, err
	}
	if order.Status == domain.StatusPending {
		if err := order.MarkPaid("pay-race", order.TotalCents, now); err != nil {
			return nil, false, err
		}
		if err := r.Repository.Save(ctx, order); err != nil {
			return nil, false, err
		}
	}
	return r.Repository.CancelIfPendingExpired(ctx, orderID, now, events)
}

func TestCancelExpiredPendingOrderSkipsWhenPaymentWinsRace(t *testing.T) {
	ctx := context.Background()
	stock := &expiryStock{reservationID: "res-expiry"}
	repo := &paidDuringExpiryRepo{Repository: memory.NewRepository()}
	svc := New(repo, stock)

	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	order, err := domain.NewOrder("ord_expiry_race", "customer-1", "res-expiry", []domain.OrderItem{
		{SKU: "BAG-RACE", Quantity: 1, UnitPriceCents: 5000},
	}, "", 0, now.Add(-10*time.Minute))
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	order.PaymentDueAt = now.Add(-time.Minute)
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := svc.cancelExpiredPendingOrder(ctx, order.ID); err != nil {
		t.Fatalf("cancelExpiredPendingOrder: %v", err)
	}
	if stock.released != "" {
		t.Fatalf("released reservation %q when payment won race", stock.released)
	}
	stored, err := repo.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Status != domain.StatusPaid {
		t.Fatalf("status = %q, want paid", stored.Status)
	}
}

func TestHandlePaymentSucceededMarksOrderPaid(t *testing.T) {
	ctx := context.Background()
	stock := &expiryStock{reservationID: "res-pay"}
	repo := memory.NewRepository()
	svc := New(repo, stock)

	now := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	order, err := domain.NewOrder("ord_pay_1", "customer-1", "res-pay", []domain.OrderItem{
		{SKU: "BAG-1", Quantity: 1, UnitPriceCents: 120000},
	}, "", 0, now.Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	payload, err := json.Marshal(map[string]any{
		"event_type":   "payment.succeeded",
		"order_id":     order.ID,
		"payment_id":   "pay-nano-1",
		"amount_cents": order.TotalCents,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := svc.handlePaymentSucceeded(ctx, paymentSucceededSubject, payload); err != nil {
		t.Fatalf("handlePaymentSucceeded: %v", err)
	}

	stored, err := repo.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Status != domain.StatusPaid {
		t.Fatalf("status = %q, want paid", stored.Status)
	}
	if stored.PaymentID != "pay-nano-1" {
		t.Fatalf("payment_id = %q, want pay-nano-1", stored.PaymentID)
	}
}

func TestHandlePaymentSucceededRejectsInvalidPayload(t *testing.T) {
	ctx := context.Background()
	svc := New(memory.NewRepository(), &expiryStock{})

	if err := svc.handlePaymentSucceeded(ctx, paymentSucceededSubject, []byte("{")); err == nil {
		t.Fatal("expected decode error for invalid JSON")
	} else if !strings.Contains(err.Error(), "decode payment.succeeded") {
		t.Fatalf("error = %v, want decode prefix", err)
	}

	payload, err := json.Marshal(map[string]any{
		"event_type":   "payment.succeeded",
		"payment_id":   "pay-1",
		"amount_cents": 1000,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := svc.handlePaymentSucceeded(ctx, paymentSucceededSubject, payload); err == nil {
		t.Fatal("expected error for missing order_id")
	} else if !strings.Contains(err.Error(), "missing order_id or payment_id") {
		t.Fatalf("error = %v, want missing fields message", err)
	}
}
