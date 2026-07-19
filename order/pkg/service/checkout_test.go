package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/order/pkg/service"
)

type fakeCouponClient struct {
	code     string
	discount float64
	err      error
}

func (f *fakeCouponClient) Redeem(ctx context.Context, code string) (*ports.Coupon, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &ports.Coupon{
		Code:             f.code,
		DiscountFraction: f.discount,
	}, nil
}

func TestCheckoutSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	stock := &fakeStock{reservationID: "res-checkout"}
	svc := service.NewWithCheckout(repo , stock, &fakeCouponClient{
		code:     "SUMMER30",
		discount: 0.30,
	}, 0)

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}
	if session.Status != domain.CheckoutStatusOpen {
		t.Fatalf("session status = %q, want open", session.Status)
	}

	session, err = svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-1", Quantity: 2, UnitPriceCents: 5000,
	})
	if err != nil {
		t.Fatalf("UpsertCheckoutItem returned error: %v", err)
	}
	if session.SubtotalCents != 10000 || session.TotalCents != 10000 {
		t.Fatalf("session totals = %d/%d, want 10000/10000", session.SubtotalCents, session.TotalCents)
	}

	session, err = svc.ApplyCheckoutCoupon(ctx, session.ID, "SUMMER30")
	if err != nil {
		t.Fatalf("ApplyCheckoutCoupon returned error: %v", err)
	}
	if session.DiscountCents != 3000 || session.TotalCents != 7000 {
		t.Fatalf("discounted totals = %d/%d, want 3000/7000", session.DiscountCents, session.TotalCents)
	}

	result, err := svc.CompleteCheckout(ctx, session.ID)
	if err != nil {
		t.Fatalf("CompleteCheckout returned error: %v", err)
	}
	if result.Session.Status != domain.CheckoutStatusCompleted {
		t.Fatalf("session status = %q, want completed", result.Session.Status)
	}
	if result.Order.Status != domain.StatusPending {
		t.Fatalf("order status = %q, want pending", result.Order.Status)
	}
	if result.Order.TotalCents != 7000 {
		t.Fatalf("order total = %d, want 7000", result.Order.TotalCents)
	}
	if result.Order.CouponCode != "SUMMER30" {
		t.Fatalf("order coupon = %q, want SUMMER30", result.Order.CouponCode)
	}
	if stock.reservationID != "res-checkout" {
		t.Fatalf("stock reservation = %q, want res-checkout", stock.reservationID)
	}

	_, err = svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-2", Quantity: 1, UnitPriceCents: 1000,
	})
	if !errors.Is(err, domain.ErrSessionNotOpen) {
		t.Fatalf("UpsertCheckoutItem on completed session error = %v, want ErrSessionNotOpen", err)
	}
}

func TestCompleteCheckoutRequiresItems(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	svc := service.NewWithCheckout(repo, &fakeStock{}, nil, 0)

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}

	_, err = svc.CompleteCheckout(ctx, session.ID)
	if !errors.Is(err, domain.ErrEmptyCheckout) {
		t.Fatalf("CompleteCheckout error = %v, want ErrEmptyCheckout", err)
	}
}

func TestApplyCouponWithoutClientReturnsUnavailable(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	svc := service.NewWithCheckout(repo, &fakeStock{}, nil, 0)

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{
		CustomerID: "customer-1",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutSession returned error: %v", err)
	}
	if _, err := svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{
		SKU: "bag-1", Quantity: 1, UnitPriceCents: 1000,
	}); err != nil {
		t.Fatalf("UpsertCheckoutItem returned error: %v", err)
	}

	_, err = svc.ApplyCheckoutCoupon(ctx, session.ID, "SUMMER30")
	if !errors.Is(err, ports.ErrCouponUnavailable) {
		t.Fatalf("ApplyCheckoutCoupon error = %v, want ErrCouponUnavailable", err)
	}
}
