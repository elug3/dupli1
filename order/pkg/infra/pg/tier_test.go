package pg

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

// The tier's share of the discount must survive every read path: a VIP order
// that loses it in one list would show the whole discount as the code's.
func TestOrderTierSurvivesEveryReadPath(t *testing.T) {
	repo := shippingFeeRepo(t, "order_tier_roundtrip_test")
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Millisecond)

	order, err := domain.NewOrder("ord-tier-1", "cust-1", "res-1", []domain.OrderItem{{
		SkuID: "sku-1", SKU: "BAG-001", Quantity: 1, UnitPriceWon: 200000,
	}}, "SUMMER", 30000, 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := order.SetTier("VIP", 20000); err != nil {
		t.Fatalf("SetTier: %v", err)
	}
	// Points and the card surcharge ride the same read paths. New orders have
	// no surcharge since SER-110; this one stands in for an order placed
	// before, which must still read back what it was charged.
	if err := order.ApplyPoints(10000); err != nil {
		t.Fatalf("ApplyPoints: %v", err)
	}
	if err := order.ApplyPaymentMethod("credit_card"); err != nil {
		t.Fatalf("ApplyPaymentMethod: %v", err)
	}
	order.CardSurchargeWon = 16000
	order.TotalWon += order.CardSurchargeWon
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	check := func(where string, got *domain.Order) {
		t.Helper()
		if got.TierPromotionCode != "VIP" || got.TierDiscountWon != 20000 || got.DiscountWon != 30000 {
			t.Fatalf("%s: tier %q %d discount %d, want VIP 20000 / 30000",
				where, got.TierPromotionCode, got.TierDiscountWon, got.DiscountWon)
		}
		if got.PaymentMethod != "credit_card" || got.PointsWon != 10000 || got.CardSurchargeWon != 16000 || got.TotalWon != 176000 {
			t.Fatalf("%s: method %q points %d surcharge %d total %d, want credit_card 10000 / 16000 / 176000",
				where, got.PaymentMethod, got.PointsWon, got.CardSurchargeWon, got.TotalWon)
		}
	}

	got, err := repo.Get(ctx, "ord-tier-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	check("Get", got)

	byCustomer, err := repo.ListByCustomer(ctx, "cust-1")
	if err != nil || len(byCustomer) != 1 {
		t.Fatalf("ListByCustomer: %v %d", err, len(byCustomer))
	}
	check("ListByCustomer", &byCustomer[0])

	all, err := repo.ListAll(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("ListAll: %v %d", err, len(all))
	}
	check("ListAll", &all[0])

	expired, err := repo.ListPendingPaymentExpired(ctx, now.Add(time.Hour))
	if err != nil || len(expired) != 1 {
		t.Fatalf("ListPendingPaymentExpired: %v %d", err, len(expired))
	}
	check("ListPendingPaymentExpired", &expired[0])
}
