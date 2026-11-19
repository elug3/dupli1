package domain_test

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

func TestReferralWithholdingRoundsEachTaxDownToTenWon(t *testing.T) {
	cases := []struct{ commission, income, local int64 }{
		{0, 0, 0},
		{42_500, 1_270, 120},
		{100_000, 3_000, 300},
		{333, 0, 0},
		{12_345, 370, 30},
	}
	for _, c := range cases {
		income, local := domain.ReferralWithholding(c.commission)
		if income != c.income || local != c.local {
			t.Errorf("withholding(%d) = %d/%d, want %d/%d", c.commission, income, local, c.income, c.local)
		}
	}
}

func TestReferralCommissionRoundsDown(t *testing.T) {
	if got := domain.ReferralCommission(12_345, 500); got != 617 {
		t.Fatalf("commission = %d, want 617", got)
	}
	if got := domain.ReferralCommission(-5, 500); got != 0 {
		t.Fatalf("negative base commission = %d, want 0", got)
	}
}

func TestEveryRouteToFulfilledRecordsWhen(t *testing.T) {
	now := time.Date(2026, 11, 19, 3, 0, 0, 0, time.UTC)
	for name, fulfil := range map[string]func(o *domain.Order) error{
		"receipt":  func(o *domain.Order) error { o.Status = domain.StatusDelivered; return o.ConfirmReceipt(now) },
		"override": func(o *domain.Order) error { o.Status = domain.StatusDelivered; return o.Fulfill(now) },
		"dispute":  func(o *domain.Order) error { o.Status = domain.StatusDisputed; return o.ResolveDisputeFulfilled(now) },
	} {
		o := &domain.Order{}
		if err := fulfil(o); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if o.FulfilledAt == nil || !o.FulfilledAt.Equal(now) {
			t.Fatalf("%s: fulfilled_at = %v, want %v", name, o.FulfilledAt, now)
		}
	}
}

func TestSetReferralOnlyOnAPendingCodeOrder(t *testing.T) {
	o := &domain.Order{Status: domain.StatusPending, PromotionCode: "HUH15"}
	if err := o.SetReferral(" 허경호 ", 500); err != nil || o.ReferrerName != "허경호" {
		t.Fatalf("SetReferral = %v, name %q", err, o.ReferrerName)
	}
	if err := (&domain.Order{Status: domain.StatusPending}).SetReferral("허경호", 500); err == nil {
		t.Fatal("an order without a code cannot carry a referral")
	}
	if err := (&domain.Order{Status: domain.StatusPaid, PromotionCode: "HUH15"}).SetReferral("허경호", 500); err == nil {
		t.Fatal("a paid order cannot gain a referral")
	}
}
