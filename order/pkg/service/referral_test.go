package service_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/order/pkg/service"
)

// A referral code's referrer and rate are kept on the order, and never shown
// in the order a shopper reads back.
func TestCompleteCheckoutSnapshotsTheReferral(t *testing.T) {
	ctx := t.Context()
	svc, promo := newPromoSvc(t)
	promo.code = "HUH15"
	promo.discount = 0.15
	promo.referral = &ports.PromotionReferral{ReferrerName: "허경호", CommissionBps: 500}
	session := openSessionWithItem(t, svc)

	if _, err := svc.ApplyCheckoutPromotion(ctx, session.ID, "HUH15"); err != nil {
		t.Fatalf("ApplyCheckoutPromotion: %v", err)
	}
	result, err := svc.CompleteCheckout(ctx, session.ID, service.CompleteCheckoutInput{
		RecipientName: "Kim", RecipientPhone: "010-0000-0000",
		ShippingAddress: domain.ShippingAddress{PostalCode: "06236", AddressLine1: "1 Test", City: "Seoul", Province: "Seoul"},
	})
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	stored, err := svc.GetOrder(ctx, result.Order.ID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if stored.ReferrerName != "허경호" || stored.ReferralCommissionBps != 500 {
		t.Fatalf("stored referral = %q/%d, want 허경호/500", stored.ReferrerName, stored.ReferralCommissionBps)
	}
	raw, _ := json.Marshal(stored)
	for _, secret := range []string{"referr", "commission", "허경호"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("order JSON leaks %q: %s", secret, raw)
		}
	}
}

// An ordinary code leaves no referral on the order.
func TestCompleteCheckoutWithAnOrdinaryCodeHasNoReferral(t *testing.T) {
	ctx := t.Context()
	svc, _ := newPromoSvc(t)
	session := openSessionWithItem(t, svc)
	if _, err := svc.ApplyCheckoutPromotion(ctx, session.ID, "SUMMER30"); err != nil {
		t.Fatalf("ApplyCheckoutPromotion: %v", err)
	}
	result, err := svc.CompleteCheckout(ctx, session.ID, service.CompleteCheckoutInput{
		RecipientName: "Kim", RecipientPhone: "010-0000-0000",
		ShippingAddress: domain.ShippingAddress{PostalCode: "06236", AddressLine1: "1 Test", City: "Seoul", Province: "Seoul"},
	})
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	stored, _ := svc.GetOrder(ctx, result.Order.ID)
	if stored.ReferrerName != "" || stored.ReferralCommissionBps != 0 {
		t.Fatalf("stored referral = %q/%d, want none", stored.ReferrerName, stored.ReferralCommissionBps)
	}
}

func TestReferralReportReadsTheMonthsFulfilledOrders(t *testing.T) {
	ctx := t.Context()
	repo := memory.NewRepository()
	kst := time.FixedZone("KST", 9*3600)
	at := func(y int, m time.Month, d, h int) *time.Time {
		t := time.Date(y, m, d, h, 0, 0, 0, kst).UTC()
		return &t
	}
	save := func(o domain.Order) {
		t.Helper()
		o.Items = []domain.OrderItem{{SKU: "BAG-1", Quantity: 1, UnitPriceWon: o.SubtotalWon}}
		o.ReservationID = "res"
		if err := repo.Save(ctx, &o); err != nil {
			t.Fatalf("save: %v", err)
		}
	}
	save(domain.Order{ID: "in-month", CustomerID: "c1", Status: domain.StatusFulfilled, PromotionCode: "HUH15",
		SubtotalWon: 1_000_000, DiscountWon: 150_000, ReferrerName: "허경호", ReferralCommissionBps: 500,
		PaidAt: at(2026, 10, 28, 12), FulfilledAt: at(2026, 11, 1, 0)})
	save(domain.Order{ID: "last-month", CustomerID: "c1", Status: domain.StatusFulfilled, PromotionCode: "HUH15",
		SubtotalWon: 500_000, DiscountWon: 75_000, ReferrerName: "허경호", ReferralCommissionBps: 500,
		PaidAt: at(2026, 10, 1, 12), FulfilledAt: at(2026, 10, 31, 23)})
	save(domain.Order{ID: "shipping", CustomerID: "c2", Status: domain.StatusInTransit, PromotionCode: "HUH15",
		SubtotalWon: 200_000, DiscountWon: 30_000, ReferrerName: "허경호", ReferralCommissionBps: 500,
		PaidAt: at(2026, 11, 10, 12)})
	save(domain.Order{ID: "refunded", CustomerID: "c2", Status: domain.StatusCanceled, PromotionCode: "HUH15",
		SubtotalWon: 300_000, DiscountWon: 45_000, ReferrerName: "허경호", ReferralCommissionBps: 500,
		PaidAt: at(2026, 11, 3, 12), CanceledAt: at(2026, 11, 4, 12)})
	save(domain.Order{ID: "no-code", CustomerID: "c3", Status: domain.StatusFulfilled,
		SubtotalWon: 300_000, PaidAt: at(2026, 11, 3, 12), FulfilledAt: at(2026, 11, 20, 12)})

	svc := service.New(repo, &fakeStock{reservationID: "res-1"})
	report, err := svc.ReferralReport(ctx, "2026-11")
	if err != nil {
		t.Fatalf("ReferralReport: %v", err)
	}
	if len(report.Referrers) != 1 {
		t.Fatalf("referrers = %+v, want one", report.Referrers)
	}
	s := report.Referrers[0]
	if s.Orders != 1 || len(s.Lines) != 1 || s.Lines[0].OrderID != "in-month" {
		t.Fatalf("lines = %+v, want only the order fulfilled in November", s.Lines)
	}
	// 850,000 × 5% = 42,500; income tax 1,270 (1,275 → ten won), local 120.
	if s.CommissionWon != 42_500 || s.IncomeTaxWon != 1_270 || s.LocalTaxWon != 120 || s.NetWon != 41_110 {
		t.Fatalf("statement = %+v", s)
	}
	if s.OpenOrders != 1 || s.Open[0].OrderID != "shipping" || s.OpenCommissionWon != 8_500 {
		t.Fatalf("open = %+v, want the order still in transit", s.Open)
	}

	if _, err := svc.ReferralReport(ctx, "2026-13"); err == nil {
		t.Fatal("a bad month should be refused")
	}
}
