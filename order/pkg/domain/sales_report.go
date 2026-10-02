package domain

import (
	"errors"
	"strings"
	"time"
)

// ReportLocation is the calendar the sales report buckets by. KST has no
// daylight saving, so a fixed zone needs no tzdata in the image.
var ReportLocation = time.FixedZone("KST", 9*60*60)

// ReportGranularity is the length of one sales report period.
type ReportGranularity string

const (
	// GranularityWeek is a Monday-to-Sunday KST week.
	GranularityWeek ReportGranularity = "week"
	// GranularityMonth is a calendar month in KST.
	GranularityMonth ReportGranularity = "month"
)

// Defaults and caps on how many periods one report covers.
const (
	DefaultReportWeeks  = 12
	DefaultReportMonths = 12
	MaxReportWeeks      = 104
	MaxReportMonths     = 36
)

const reportDateLayout = "2006-01-02"

var ErrInvalidReportRange = errors.New("invalid report range")

// SalesReportRange is the half-open KST range [Start, End) a report covers,
// snapped to whole periods.
type SalesReportRange struct {
	Granularity ReportGranularity
	Start       time.Time
	End         time.Time
}

// NewSalesReportRange parses granularity and optional from/to dates
// (YYYY-MM-DD, KST, both inclusive) and widens them to whole periods. A
// missing to means the current period; a missing from means the default
// number of periods ending at to.
func NewSalesReportRange(granularity, from, to string, now time.Time) (SalesReportRange, error) {
	g := ReportGranularity(strings.ToLower(strings.TrimSpace(granularity)))
	if g == "" {
		g = GranularityWeek
	}
	if g != GranularityWeek && g != GranularityMonth {
		return SalesReportRange{}, ErrInvalidReportRange
	}

	toDay := now.In(ReportLocation)
	if strings.TrimSpace(to) != "" {
		d, err := time.ParseInLocation(reportDateLayout, strings.TrimSpace(to), ReportLocation)
		if err != nil {
			return SalesReportRange{}, ErrInvalidReportRange
		}
		toDay = d
	}
	end := g.next(g.periodStart(toDay))

	var start time.Time
	if strings.TrimSpace(from) != "" {
		d, err := time.ParseInLocation(reportDateLayout, strings.TrimSpace(from), ReportLocation)
		if err != nil {
			return SalesReportRange{}, ErrInvalidReportRange
		}
		start = g.periodStart(d)
	} else {
		start = end
		for i := 0; i < g.defaultPeriods(); i++ {
			start = g.prev(start)
		}
	}
	if !start.Before(end) {
		return SalesReportRange{}, ErrInvalidReportRange
	}

	periods := 0
	for p := start; p.Before(end); p = g.next(p) {
		periods++
		if periods > g.maxPeriods() {
			return SalesReportRange{}, ErrInvalidReportRange
		}
	}
	return SalesReportRange{Granularity: g, Start: start, End: end}, nil
}

func (g ReportGranularity) periodStart(t time.Time) time.Time {
	t = t.In(ReportLocation)
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, ReportLocation)
	if g == GranularityMonth {
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, ReportLocation)
	}
	sinceMonday := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -sinceMonday)
}

func (g ReportGranularity) next(start time.Time) time.Time {
	if g == GranularityMonth {
		return start.AddDate(0, 1, 0)
	}
	return start.AddDate(0, 0, 7)
}

func (g ReportGranularity) prev(start time.Time) time.Time {
	if g == GranularityMonth {
		return start.AddDate(0, -1, 0)
	}
	return start.AddDate(0, 0, -7)
}

func (g ReportGranularity) defaultPeriods() int {
	if g == GranularityMonth {
		return DefaultReportMonths
	}
	return DefaultReportWeeks
}

func (g ReportGranularity) maxPeriods() int {
	if g == GranularityMonth {
		return MaxReportMonths
	}
	return MaxReportWeeks
}

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
	Granularity ReportGranularity `json:"granularity"`
	Timezone    string            `json:"timezone"`
	From        string            `json:"from"`
	To          string            `json:"to"`
	Periods     []SalesPeriod     `json:"periods"`
	Totals      SalesPeriod       `json:"totals"`
}

// BuildSalesReport buckets orders into every period of r, empty periods
// included. orders may hold anything; only payments and refunds inside the
// range are counted.
func BuildSalesReport(r SalesReportRange, orders []Order) SalesReport {
	var periods []SalesPeriod
	index := map[time.Time]int{}
	for p := r.Start; p.Before(r.End); p = r.Granularity.next(p) {
		index[p] = len(periods)
		periods = append(periods, SalesPeriod{
			PeriodStart: p.Format(reportDateLayout),
			PeriodEnd:   r.Granularity.next(p).AddDate(0, 0, -1).Format(reportDateLayout),
		})
	}
	bucket := func(t *time.Time) *SalesPeriod {
		if t == nil || t.Before(r.Start) || !t.Before(r.End) {
			return nil
		}
		i, ok := index[r.Granularity.periodStart(*t)]
		if !ok {
			return nil
		}
		return &periods[i]
	}

	for i := range orders {
		o := &orders[i]
		if o.PaidAt == nil {
			continue
		}
		if p := bucket(o.PaidAt); p != nil {
			p.Orders++
			p.GrossWon += o.TotalWon
			p.DiscountWon += o.DiscountWon
			p.ShippingFeeWon += o.ShippingFeeWon
		}
		if o.Status == StatusCanceled {
			if p := bucket(o.CanceledAt); p != nil {
				p.Refunds++
				p.RefundedWon += o.TotalWon
			}
		}
	}

	totals := SalesPeriod{
		PeriodStart: r.Start.Format(reportDateLayout),
		PeriodEnd:   r.End.AddDate(0, 0, -1).Format(reportDateLayout),
	}
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
		Timezone:    "Asia/Seoul",
		From:        totals.PeriodStart,
		To:          totals.PeriodEnd,
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
