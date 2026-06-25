package domain_test

import (
	"testing"
	"time"

	"github.com/elug3/schick/order/pkg/domain"
)

func TestCheckoutSessionTotalsWithCoupon(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	session, err := domain.NewCheckoutSession("cs_000001", "customer-1", now, time.Hour)
	if err != nil {
		t.Fatalf("NewCheckoutSession returned error: %v", err)
	}

	if err := session.UpsertItem(domain.OrderItem{
		SKU: "BAG-1", Quantity: 1, UnitPriceCents: 10000,
	}, now); err != nil {
		t.Fatalf("UpsertItem returned error: %v", err)
	}
	if err := session.ApplyCoupon("SUMMER30", 0.30, now); err != nil {
		t.Fatalf("ApplyCoupon returned error: %v", err)
	}

	if session.SubtotalCents != 10000 || session.DiscountCents != 3000 || session.TotalCents != 7000 {
		t.Fatalf("totals = %d/%d/%d, want 10000/3000/7000", session.SubtotalCents, session.DiscountCents, session.TotalCents)
	}
}

func TestCheckoutSessionExpires(t *testing.T) {
	start := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	session, err := domain.NewCheckoutSession("cs_000002", "customer-1", start, time.Minute)
	if err != nil {
		t.Fatalf("NewCheckoutSession returned error: %v", err)
	}

	err = session.EnsureOpen(start.Add(2 * time.Minute))
	if err != domain.ErrSessionExpired {
		t.Fatalf("EnsureOpen error = %v, want ErrSessionExpired", err)
	}
	if session.Status != domain.CheckoutStatusExpired {
		t.Fatalf("session status = %q, want expired", session.Status)
	}
}
