package domain_test

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

func TestCardSurchargeWon_RoundsDown(t *testing.T) {
	cases := []struct{ base, bps, want int64 }{
		{100000, 1000, 10000},
		{12345, 1000, 1234}, // 1,234.5 → 1,234
		{12345, 250, 308},   // 2.5%: 308.625 → 308
		{0, 1000, 0},
		{100000, 0, 0},
	}
	for _, c := range cases {
		if got := domain.CardSurchargeWon(c.base, c.bps); got != c.want {
			t.Errorf("CardSurchargeWon(%d, %d) = %d, want %d", c.base, c.bps, got, c.want)
		}
	}
}

func TestApplyPaymentMethod_OnlyOnce(t *testing.T) {
	o, err := domain.NewOrder("o-1", "c-1", "r-1", []domain.OrderItem{{SKU: "A", Quantity: 1, UnitPriceWon: 50000}}, "", 0, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := o.ApplyPaymentMethod("", 1000); err != nil {
		t.Fatal(err)
	}
	if o.TotalWon != 55000 {
		t.Fatalf("total = %d, want 55000", o.TotalWon)
	}
	if err := o.ApplyPaymentMethod("credit_card", 1000); err == nil {
		t.Fatal("second ApplyPaymentMethod accepted; it would stack a second surcharge")
	}
	if o.TotalWon != 55000 {
		t.Fatalf("total = %d after refused second call, want 55000", o.TotalWon)
	}
}
