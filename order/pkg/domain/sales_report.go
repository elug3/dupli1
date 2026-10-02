package domain

import (
	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

// SalesPeriod is one week or month of the sales report. Sales count in the
// period they were paid; refunds (a cancel of a paid order) count in the
// period they happened, so a period's net is what moved in that period.
type SalesPeriod struct {
	PeriodStart string `json:"period_start"` // first day, YYYY-MM-DD KST
	PeriodEnd   string `json:"period_end"`   // last day, inclusive
	// Paid orders and their amounts, by paid_at.
	Orders         int   `json:"orders"`
	GrossWon       int64 `json:"gross_won"`
	DiscountWon    int64 `json:"discount_won"`
	ShippingFeeWon int64 `json:"shipping_fee_won"`
	// Refunds of paid orders, by canceled_at.
	Refunds     int   `json:"refunds"`
	RefundedWon int64 `json:"refunded_won"`
	NetWon      int64 `json:"net_won"`
	// AverageOrderWon is GrossWon / Orders, 0 with no orders.
	AverageOrderWon int64 `json:"average_order_won"`
}

// SalesReport is the response of GET /api/v1/orders/reports/sales.
type SalesReport struct {
	Granularity reportperiod.Granularity `json:"granularity"`
	Timezone    string                   `json:"timezone"`
	From        string                   `json:"from"`
	To          string                   `json:"to"`
	Periods     []SalesPeriod            `json:"periods"`
	Totals      SalesPeriod              `json:"totals"`
}

// BuildSalesReport buckets orders into every period of r, empty periods
// included. orders may hold anything; only payments and refunds inside the
// range are counted.
func BuildSalesReport(r reportperiod.Range, orders []Order) SalesReport {
	periods := make([]SalesPeriod, 0)
	for _, p := range r.Periods() {
		periods = append(periods, SalesPeriod{PeriodStart: p.StartDate(), PeriodEnd: p.EndDate()})
	}

	for i := range orders {
		o := &orders[i]
		if o.PaidAt == nil {
			continue
		}
		if i := r.Index(*o.PaidAt); i >= 0 {
			p := &periods[i]
			p.Orders++
			p.GrossWon += o.TotalWon
			p.DiscountWon += o.DiscountWon
			p.ShippingFeeWon += o.ShippingFeeWon
		}
		if o.Status == StatusCanceled && o.CanceledAt != nil {
			if i := r.Index(*o.CanceledAt); i >= 0 {
				periods[i].Refunds++
				periods[i].RefundedWon += o.TotalWon
			}
		}
	}

	totals := SalesPeriod{PeriodStart: r.StartDate(), PeriodEnd: r.EndDate()}
	for i := range periods {
		p := &periods[i]
		p.finish()
		totals.Orders += p.Orders
		totals.GrossWon += p.GrossWon
		totals.DiscountWon += p.DiscountWon
		totals.ShippingFeeWon += p.ShippingFeeWon
		totals.Refunds += p.Refunds
		totals.RefundedWon += p.RefundedWon
	}
	totals.finish()

	return SalesReport{
		Granularity: r.Granularity,
		Timezone:    reportperiod.Timezone,
		From:        r.StartDate(),
		To:          r.EndDate(),
		Periods:     periods,
		Totals:      totals,
	}
}

func (p *SalesPeriod) finish() {
	p.NetWon = p.GrossWon - p.RefundedWon
	if p.Orders > 0 {
		p.AverageOrderWon = p.GrossWon / int64(p.Orders)
	}
}
