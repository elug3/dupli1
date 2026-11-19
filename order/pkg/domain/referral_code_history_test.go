package domain

import (
	"testing"
	"time"
)

func TestBuildReferralCodeHistory(t *testing.T) {
	kst := time.FixedZone("KST", 9*3600)
	at := func(m time.Month, d int) *time.Time { t := time.Date(2026, m, d, 12, 0, 0, 0, kst); return &t }
	o := func(id, cust, code, name string, st OrderStatus, paid, fulfilled *time.Time, sub, disc int64) Order {
		return Order{ID: id, CustomerID: cust, PromotionCode: code, ReferrerName: name, ReferralCommissionBps: 500,
			Status: st, PaidAt: paid, FulfilledAt: fulfilled, SubtotalWon: sub, DiscountWon: disc}
	}
	orders := []Order{
		o("oct", "c1", "HUH15", "허경호", StatusFulfilled, at(10, 25), at(10, 31), 840000, 126000),
		// Fulfilled 1 Nov 00:30 KST is November, though it is 31 Oct in UTC.
		{ID: "nov-edge", CustomerID: "c2", PromotionCode: "HUH15", ReferrerName: "허경호", ReferralCommissionBps: 500,
			Status: StatusFulfilled, PaidAt: at(10, 28), FulfilledAt: func() *time.Time { t := time.Date(2026, 11, 1, 0, 30, 0, 0, kst); return &t }(),
			SubtotalWon: 100000, DiscountWon: 15000},
		o("nov", "c1", "HUH15", "허경호 (renamed)", StatusFulfilled, at(11, 2), at(11, 10), 900000, 135000),
		o("open", "c3", "HUH15", "허경호 (renamed)", StatusInTransit, at(11, 16), nil, 360000, 54000),
		o("gone", "c4", "HUH15", "허경호 (renamed)", StatusCanceled, at(11, 6), nil, 540000, 81000),
		// Not counted: another code, an ordinary code, unpaid.
		o("other", "c5", "OTHER", "someone", StatusFulfilled, at(11, 3), at(11, 9), 100000, 0),
		o("plain", "c5", "HUH15", "", StatusFulfilled, at(11, 3), at(11, 9), 100000, 0),
		o("unpaid", "c5", "HUH15", "허경호", StatusCanceled, nil, nil, 100000, 15000),
	}
	h := BuildReferralCodeHistory("HUH15", orders)

	if len(h.Orders) != 5 || h.Orders[0].OrderID != "open" || h.Orders[4].OrderID != "oct" {
		t.Fatalf("orders = %+v, want 5 newest payment first", h.Orders)
	}
	if len(h.ReferrerNames) != 2 || h.ReferrerNames[0] != "허경호" {
		t.Fatalf("referrer names = %v", h.ReferrerNames)
	}
	s := h.Summary
	if s.Orders != 5 || s.FulfilledOrders != 3 || s.OpenOrders != 1 || s.CanceledOrders != 1 || s.Customers != 3 {
		t.Fatalf("summary counts = %+v", s)
	}
	if s.SubtotalWon != 840000+100000+900000+360000 || s.DiscountWon != 126000+15000+135000+54000 {
		t.Fatalf("summary money = %+v", s)
	}
	// 714000×5% + 85000×5% + 765000×5%
	if s.CommissionWon != 35700+4250+38250 || s.OpenCommissionWon != 15300 {
		t.Fatalf("commission = %d open %d", s.CommissionWon, s.OpenCommissionWon)
	}
	if len(h.Months) != 2 || h.Months[0].Month != "2026-11" || h.Months[1].Month != "2026-10" {
		t.Fatalf("months = %+v", h.Months)
	}
	nov := h.Months[0]
	if nov.Orders != 2 || nov.CommissionWon != 42500 || nov.IncomeTaxWon != 1270 || nov.LocalTaxWon != 120 || nov.NetWon != 42500-1390 {
		t.Fatalf("november = %+v", nov)
	}
	if h.Orders[1].CanceledAt != nil || h.WithholdingBps != 330 {
		t.Fatalf("history = %+v", h)
	}
}

func TestNormalizeReferralCode(t *testing.T) {
	if c, err := NormalizeReferralCode(" huh15 "); err != nil || c != "HUH15" {
		t.Fatalf("= %q, %v", c, err)
	}
	if _, err := NormalizeReferralCode("  "); err != ErrInvalidReferralCode {
		t.Fatalf("empty = %v", err)
	}
}
