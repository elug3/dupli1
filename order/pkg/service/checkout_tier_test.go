package service_test

import (
	"errors"
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/order/pkg/service"
)

var tierCheckoutInput = service.CompleteCheckoutInput{
	RecipientName: "Kim", RecipientPhone: "010-0000-0000",
	ShippingAddress: domain.ShippingAddress{PostalCode: "06236", AddressLine1: "1 Test", City: "Seoul", Province: "Seoul"},
}

// A VIP's tier discount stacks under the code they enter: the session shows
// both, and the order records the tier's share inside discount_won.
func TestTierStacksUnderTheCode(t *testing.T) {
	ctx := t.Context()
	svc, promo := newPromoSvc(t)
	promo.tierCode, promo.tierDiscount = "VIP", 0.10
	session := openSessionWithItem(t, svc)

	shown, err := svc.ApplyCheckoutPromotion(ctx, session.ID, "SUMMER30")
	if err != nil {
		t.Fatalf("ApplyCheckoutPromotion: %v", err)
	}
	if shown.TierPromotionCode != "VIP" || shown.TierDiscountWon != 1000 ||
		shown.DiscountWon != 4000 || shown.TotalWon != 6000 {
		t.Fatalf("session = tier %q %d, discount %d, total %d; want VIP 1000, 4000, 6000",
			shown.TierPromotionCode, shown.TierDiscountWon, shown.DiscountWon, shown.TotalWon)
	}

	result, err := svc.CompleteCheckout(ctx, session.ID, tierCheckoutInput)
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	order := result.Order
	if order.PromotionCode != "SUMMER30" || order.TierPromotionCode != "VIP" ||
		order.TierDiscountWon != 1000 || order.DiscountWon != 4000 || order.TotalWon != 6000 {
		t.Fatalf("order = code %q tier %q %d, discount %d, total %d; want SUMMER30 VIP 1000, 4000, 6000",
			order.PromotionCode, order.TierPromotionCode, order.TierDiscountWon, order.DiscountWon, order.TotalWon)
	}
	// Only the code is reserved; a tier has no ledger.
	if got := promo.reserved[order.ID]; got != "SUMMER30" {
		t.Fatalf("reserved = %q, want SUMMER30", got)
	}
}

// A member with no code still gets the tier, and paying spends nothing.
func TestTierAppliesWithoutACode(t *testing.T) {
	ctx := t.Context()
	svc, promo := newPromoSvc(t)
	promo.tierCode, promo.tierDiscount = "VIP", 0.10
	session := openSessionWithItem(t, svc)

	shown, err := svc.GetCheckoutSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetCheckoutSession: %v", err)
	}
	if shown.TierDiscountWon != 1000 || shown.TotalWon != 9000 {
		t.Fatalf("session tier %d total %d, want 1000 / 9000", shown.TierDiscountWon, shown.TotalWon)
	}

	result, err := svc.CompleteCheckout(ctx, session.ID, tierCheckoutInput)
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	if result.Order.PromotionCode != "" || result.Order.TierPromotionCode != "VIP" || result.Order.TotalWon != 9000 {
		t.Fatalf("order = %+v, want no code, VIP, total 9000", result.Order)
	}
	if _, err := svc.MarkOrderPaid(ctx, result.Order.ID, "pay-1", result.Order.TotalWon); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	if promo.consumed[result.Order.ID] != 0 {
		t.Fatal("a tier-only order must not consume a ledger use")
	}
}

// Code and tier together never take more than the goods are worth.
func TestTierIsCappedByWhatTheCodeLeaves(t *testing.T) {
	ctx := t.Context()
	svc, promo := newPromoSvc(t)
	promo.discount = 0.95
	promo.tierCode, promo.tierDiscount = "VIP", 0.10
	session := openSessionWithItem(t, svc)
	if _, err := svc.ApplyCheckoutPromotion(ctx, session.ID, "SUMMER30"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	result, err := svc.CompleteCheckout(ctx, session.ID, tierCheckoutInput)
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	if result.Order.TierDiscountWon != 500 || result.Order.DiscountWon != 10000 || result.Order.TotalWon != 0 {
		t.Fatalf("order tier %d discount %d total %d, want 500 / 10000 / 0",
			result.Order.TierDiscountWon, result.Order.DiscountWon, result.Order.TotalWon)
	}
}

// A failed lookup shows no tier on a session read, but fails complete rather
// than charging a member full price without saying so.
func TestTierLookupFailure(t *testing.T) {
	ctx := t.Context()
	svc, promo := newPromoSvc(t)
	promo.tierErr = errors.New("product down")
	session := openSessionWithItem(t, svc)

	shown, err := svc.GetCheckoutSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetCheckoutSession should not fail on a tier lookup: %v", err)
	}
	if shown.TierDiscountWon != 0 || shown.TotalWon != 10000 {
		t.Fatalf("session tier %d total %d, want 0 / 10000", shown.TierDiscountWon, shown.TotalWon)
	}
	if _, err := svc.CompleteCheckout(ctx, session.ID, tierCheckoutInput); err == nil {
		t.Fatal("CompleteCheckout should fail when the tier cannot be looked up")
	} else if !errors.Is(err, ports.ErrPromotionUnavailable) {
		t.Fatalf("CompleteCheckout error = %v, want ErrPromotionUnavailable (503)", err)
	}
}
