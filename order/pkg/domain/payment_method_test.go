package domain_test

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

// A card order costs exactly the goods after discounts plus delivery: no
// surcharge since SER-110.
func TestApplyPaymentMethod_CardHasNoSurcharge(t *testing.T) {
	o, err := domain.NewOrder("o-1", "c-1", "r-1", []domain.OrderItem{{SKU: "A", Quantity: 1, UnitPriceWon: 50000}}, "", 0, 3000, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := o.ApplyPaymentMethod(""); err != nil {
		t.Fatal(err)
	}
	if o.PaymentMethod != domain.PaymentMethodCreditCard || o.CardSurchargeWon != 0 || o.TotalWon != 53000 {
		t.Fatalf("method/surcharge/total = %q/%d/%d, want credit_card/0/53000", o.PaymentMethod, o.CardSurchargeWon, o.TotalWon)
	}
}

func TestApplyPaymentMethod_OnlyOnce(t *testing.T) {
	o, err := domain.NewOrder("o-1", "c-1", "r-1", []domain.OrderItem{{SKU: "A", Quantity: 1, UnitPriceWon: 50000}}, "", 0, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := o.ApplyPaymentMethod(""); err != nil {
		t.Fatal(err)
	}
	if err := o.ApplyPaymentMethod("bypass"); err == nil {
		t.Fatal("second ApplyPaymentMethod accepted")
	}
	if o.PaymentMethod != domain.PaymentMethodCreditCard || o.TotalWon != 50000 {
		t.Fatalf("method/total = %q/%d after refused second call, want credit_card/50000", o.PaymentMethod, o.TotalWon)
	}
}
