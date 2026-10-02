package domain_test

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

func kst(y int, m time.Month, d, h int) time.Time {
	return time.Date(y, m, d, h, 0, 0, 0, reportperiod.Location)
}

func TestBuildSalesReport_RefundCountsInTheWeekItHappened(t *testing.T) {
	r, err := reportperiod.NewRange("week", "2026-09-14", "2026-09-27", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	at := func(t time.Time) *time.Time { return &t }

	orders := []domain.Order{
		// Paid Sunday 23:00 KST, week of 09-14, still live.
		{ID: "a", Status: domain.StatusConfirmed, TotalWon: 10000, DiscountWon: 1000, ShippingFeeWon: 3000, PaidAt: at(kst(2026, 9, 20, 23))},
		// Paid Monday 00:00 KST, week of 09-21, refunded the same week.
		{ID: "b", Status: domain.StatusCanceled, TotalWon: 20000, PaidAt: at(kst(2026, 9, 21, 0)), CanceledAt: at(kst(2026, 9, 22, 10))},
		// Paid before the range, refunded in week of 09-14: only the refund counts.
		{ID: "c", Status: domain.StatusCanceled, TotalWon: 5000, PaidAt: at(kst(2026, 9, 1, 9)), CanceledAt: at(kst(2026, 9, 15, 9))},
		// Never paid, canceled: not a sale, not a refund.
		{ID: "d", Status: domain.StatusCanceled, TotalWon: 7000, CanceledAt: at(kst(2026, 9, 16, 9))},
	}
	rep := domain.BuildSalesReport(r, orders)

	if len(rep.Periods) != 2 {
		t.Fatalf("periods = %d, want 2", len(rep.Periods))
	}
	w1, w2 := rep.Periods[0], rep.Periods[1]
	if w1.PeriodStart != "2026-09-14" || w1.PeriodEnd != "2026-09-20" {
		t.Fatalf("week 1 = %s..%s", w1.PeriodStart, w1.PeriodEnd)
	}
	if w1.Orders != 1 || w1.GrossWon != 10000 || w1.DiscountWon != 1000 || w1.ShippingFeeWon != 3000 ||
		w1.Refunds != 1 || w1.RefundedWon != 5000 || w1.NetWon != 5000 || w1.AverageOrderWon != 10000 {
		t.Fatalf("week 1 = %+v", w1)
	}
	if w2.Orders != 1 || w2.GrossWon != 20000 || w2.Refunds != 1 || w2.RefundedWon != 20000 || w2.NetWon != 0 {
		t.Fatalf("week 2 = %+v", w2)
	}
	if rep.Totals.Orders != 2 || rep.Totals.GrossWon != 30000 || rep.Totals.RefundedWon != 25000 || rep.Totals.NetWon != 5000 {
		t.Fatalf("totals = %+v", rep.Totals)
	}
	if rep.From != "2026-09-14" || rep.To != "2026-09-27" || rep.Timezone != "Asia/Seoul" {
		t.Fatalf("report range = %s..%s %s", rep.From, rep.To, rep.Timezone)
	}
}

func TestBuildSalesReport_EmptyPeriodsAreListed(t *testing.T) {
	r, err := reportperiod.NewRange("month", "2026-01-01", "2026-03-31", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rep := domain.BuildSalesReport(r, nil)
	if len(rep.Periods) != 3 || rep.Periods[1].PeriodStart != "2026-02-01" || rep.Periods[1].PeriodEnd != "2026-02-28" {
		t.Fatalf("periods = %+v", rep.Periods)
	}
}

func TestCancelStampsCanceledAtAndReinstateClearsIt(t *testing.T) {
	now := kst(2026, 9, 1, 12)
	o, err := domain.NewOrder("o-1", "c-1", "res-1", []domain.OrderItem{{SKU: "A", Quantity: 1, UnitPriceWon: 1000}}, "", 0, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Cancel(now); err != nil {
		t.Fatal(err)
	}
	if o.CanceledAt == nil || !o.CanceledAt.Equal(now) {
		t.Fatalf("CanceledAt = %v, want %v", o.CanceledAt, now)
	}
	if err := o.ReinstateForLatePayment("res-2", now); err != nil {
		t.Fatal(err)
	}
	if o.CanceledAt != nil {
		t.Fatalf("CanceledAt = %v after reinstate, want nil", o.CanceledAt)
	}
}
