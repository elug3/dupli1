package domain_test

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
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

func TestCheckoutSessionUpsertItem_MatchesBySkuIDWhenBothPopulated(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	session, err := domain.NewCheckoutSession("cs_000003", "customer-1", now, time.Hour)
	if err != nil {
		t.Fatalf("NewCheckoutSession returned error: %v", err)
	}

	if err := session.UpsertItem(domain.OrderItem{
		SkuID: "SKUID-1", SKU: "BAG-1", Quantity: 1, UnitPriceCents: 10000,
	}, now); err != nil {
		t.Fatalf("UpsertItem returned error: %v", err)
	}
	// Same SkuID, different (stale) SKU string — should update in place, not duplicate.
	if err := session.UpsertItem(domain.OrderItem{
		SkuID: "SKUID-1", SKU: "BAG-1-RENAMED", Quantity: 3, UnitPriceCents: 10000,
	}, now); err != nil {
		t.Fatalf("UpsertItem returned error: %v", err)
	}

	if len(session.Items) != 1 {
		t.Fatalf("want 1 item, got %d: %+v", len(session.Items), session.Items)
	}
	if session.Items[0].Quantity != 3 || session.Items[0].SKU != "BAG-1-RENAMED" {
		t.Fatalf("unexpected item after upsert: %+v", session.Items[0])
	}
}

func TestCheckoutSessionUpsertItem_FallsBackToSKUWhenSkuIDMissing(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	session, err := domain.NewCheckoutSession("cs_000004", "customer-1", now, time.Hour)
	if err != nil {
		t.Fatalf("NewCheckoutSession returned error: %v", err)
	}

	if err := session.UpsertItem(domain.OrderItem{
		SKU: "BAG-1", Quantity: 1, UnitPriceCents: 10000,
	}, now); err != nil {
		t.Fatalf("UpsertItem returned error: %v", err)
	}
	if err := session.UpsertItem(domain.OrderItem{
		SKU: "BAG-1", Quantity: 5, UnitPriceCents: 10000,
	}, now); err != nil {
		t.Fatalf("UpsertItem returned error: %v", err)
	}

	if len(session.Items) != 1 || session.Items[0].Quantity != 5 {
		t.Fatalf("want 1 item with quantity 5, got %+v", session.Items)
	}
}

func TestCheckoutSessionRemoveItemBySkuID(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	session, err := domain.NewCheckoutSession("cs_000005", "customer-1", now, time.Hour)
	if err != nil {
		t.Fatalf("NewCheckoutSession returned error: %v", err)
	}

	if err := session.UpsertItem(domain.OrderItem{
		SkuID: "SKUID-1", SKU: "BAG-1", Quantity: 1, UnitPriceCents: 10000,
	}, now); err != nil {
		t.Fatalf("UpsertItem returned error: %v", err)
	}
	if err := session.RemoveItemBySkuID("SKUID-1", now); err != nil {
		t.Fatalf("RemoveItemBySkuID returned error: %v", err)
	}
	if len(session.Items) != 0 {
		t.Fatalf("want empty items, got %+v", session.Items)
	}
}

func TestCheckoutSessionSetItems_AcceptsSkuIDOnlyItem(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	session, err := domain.NewCheckoutSession("cs_000006", "customer-1", now, time.Hour)
	if err != nil {
		t.Fatalf("NewCheckoutSession returned error: %v", err)
	}

	err = session.SetItems([]domain.OrderItem{
		{SkuID: "SKUID-1", Quantity: 2, UnitPriceCents: 5000},
	}, now)
	if err != nil {
		t.Fatalf("SetItems returned error: %v", err)
	}
	if len(session.Items) != 1 || session.Items[0].SkuID != "SKUID-1" {
		t.Fatalf("unexpected items: %+v", session.Items)
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
