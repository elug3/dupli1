package domain

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

// Referral commission (docs/product-referral-codes.md).
//
// A secret referral code (product, kind referral) earns its referrer a cash
// commission on every order placed with it. Staff pay it by hand once a
// month, so the report answers "what do we owe each referrer for this month":
//
//   - Only fulfilled orders earn, in the KST month they were fulfilled.
//     Fulfilled is final (a fulfilled order cannot be canceled or refunded),
//     so a refund before fulfillment simply means the order never earns, and
//     there is nothing to claw back from a month already paid.
//   - The commission base is the goods after the code's discount:
//     subtotal_won - discount_won. Shipping and the card surcharge are not
//     sales, so they earn nothing. Points the shopper paid with are part of
//     the sale and do count.
//   - Each order's commission is base × rate, rounded down to the won, using
//     the rate snapshotted on the order at checkout.
//   - 3.3% is withheld from each referrer's monthly total: income tax 3% and
//     local income tax 0.3% (a tenth of the income tax), each rounded down to
//     ten won, as a withholding statement is filed.

// ErrInvalidReferralMonth is a month that is not YYYY-MM.
var ErrInvalidReferralMonth = errors.New("month must be YYYY-MM")

// ReferralOpenStatuses are the statuses of a paid order that may still be
// fulfilled: it will earn commission once it is, and nothing if it is
// canceled first.
func ReferralOpenStatuses() []string {
	return []string{
		string(StatusPaid), string(StatusConfirmed), string(StatusInTransit),
		string(StatusDelivered), string(StatusDisputed),
	}
}

func isReferralOpen(s OrderStatus) bool {
	for _, open := range ReferralOpenStatuses() {
		if string(s) == open {
			return true
		}
	}
	return false
}

// ReferralMonth is one KST calendar month, [Start, End).
type ReferralMonth struct {
	Month string // YYYY-MM
	Start time.Time
	End   time.Time
}

// ParseReferralMonth reads "YYYY-MM" in KST. Empty means the month now is in.
func ParseReferralMonth(month string, now time.Time) (ReferralMonth, error) {
	month = strings.TrimSpace(month)
	var start time.Time
	if month == "" {
		start = reportperiod.Month.PeriodStart(now)
	} else {
		t, err := time.ParseInLocation("2006-01", month, reportperiod.Location)
		if err != nil {
			return ReferralMonth{}, ErrInvalidReferralMonth
		}
		start = t
	}
	return ReferralMonth{Month: start.Format("2006-01"), Start: start, End: start.AddDate(0, 1, 0)}, nil
}

// ReferralLine is one referred order on a statement.
type ReferralLine struct {
	OrderID     string      `json:"order_id"`
	CustomerID  string      `json:"customer_id"`
	Code        string      `json:"code"`
	Status      OrderStatus `json:"status"`
	PaidAt      *time.Time  `json:"paid_at,omitempty"`
	FulfilledAt *time.Time  `json:"fulfilled_at,omitempty"`
	SubtotalWon int64       `json:"subtotal_won"`
	DiscountWon int64       `json:"discount_won"`
	PointsWon   int64       `json:"points_won"`
	// BaseWon is what the commission is a percentage of: goods after the
	// code's discount.
	BaseWon       int64 `json:"base_won"`
	CommissionBps int   `json:"commission_bps"`
	CommissionWon int64 `json:"commission_won"`
}

// ReferrerStatement is what one referrer is owed for the month.
type ReferrerStatement struct {
	ReferrerName string   `json:"referrer_name"`
	Codes        []string `json:"codes"`
	// Fulfilled orders of the month: the ones being paid for.
	Orders         int            `json:"orders"`
	BaseWon        int64          `json:"base_won"`
	CommissionWon  int64          `json:"commission_won"`
	IncomeTaxWon   int64          `json:"income_tax_won"`
	LocalTaxWon    int64          `json:"local_tax_won"`
	WithholdingWon int64          `json:"withholding_won"`
	NetWon         int64          `json:"net_won"`
	Lines          []ReferralLine `json:"lines"`
	// Paid orders not fulfilled yet, whenever they were placed. They earn
	// nothing yet; OpenCommissionWon is what they will earn if fulfilled at
	// the rate they were placed at.
	OpenOrders        int            `json:"open_orders"`
	OpenCommissionWon int64          `json:"open_commission_won"`
	Open              []ReferralLine `json:"open"`
}

// ReferralTotals sums every statement of the month.
type ReferralTotals struct {
	Orders            int   `json:"orders"`
	BaseWon           int64 `json:"base_won"`
	CommissionWon     int64 `json:"commission_won"`
	WithholdingWon    int64 `json:"withholding_won"`
	NetWon            int64 `json:"net_won"`
	OpenOrders        int   `json:"open_orders"`
	OpenCommissionWon int64 `json:"open_commission_won"`
}

// ReferralReport is the response of GET /api/v1/orders/reports/referrals.
type ReferralReport struct {
	Month          string              `json:"month"`
	From           string              `json:"from"` // first day, YYYY-MM-DD KST
	To             string              `json:"to"`   // last day, inclusive
	Timezone       string              `json:"timezone"`
	WithholdingBps int                 `json:"withholding_bps"`
	Referrers      []ReferrerStatement `json:"referrers"`
	Totals         ReferralTotals      `json:"totals"`
}

// WithholdingBps is the 3.3% withheld from a referrer's commission.
const WithholdingBps = 330

// ReferralCommission is base × rate, rounded down to the won.
func ReferralCommission(baseWon int64, bps int) int64 {
	if baseWon <= 0 || bps <= 0 {
		return 0
	}
	return baseWon * int64(bps) / 10000
}

// ReferralWithholding splits the 3.3% withheld from a monthly commission into
// income tax (3%) and local income tax (a tenth of it), each rounded down to
// ten won.
func ReferralWithholding(commissionWon int64) (incomeTax, localTax int64) {
	if commissionWon <= 0 {
		return 0, 0
	}
	incomeTax = commissionWon * 3 / 100 / 10 * 10
	localTax = incomeTax / 10 / 10 * 10
	return incomeTax, localTax
}

func referralLine(o *Order) ReferralLine {
	base := o.SubtotalWon - o.DiscountWon
	if base < 0 {
		base = 0
	}
	return ReferralLine{
		OrderID:       o.ID,
		CustomerID:    o.CustomerID,
		Code:          o.PromotionCode,
		Status:        o.Status,
		PaidAt:        o.PaidAt,
		FulfilledAt:   o.FulfilledAt,
		SubtotalWon:   o.SubtotalWon,
		DiscountWon:   o.DiscountWon,
		PointsWon:     o.PointsWon,
		BaseWon:       base,
		CommissionBps: o.ReferralCommissionBps,
		CommissionWon: ReferralCommission(base, o.ReferralCommissionBps),
	}
}

// BuildReferralReport groups referred orders by referrer for month m. orders
// may hold anything: only referred orders fulfilled in the month, and
// referred orders still open, are counted.
func BuildReferralReport(m ReferralMonth, orders []Order) ReferralReport {
	byName := map[string]*ReferrerStatement{}
	codes := map[string]map[string]bool{}
	statement := func(name string) *ReferrerStatement {
		s, ok := byName[name]
		if !ok {
			s = &ReferrerStatement{ReferrerName: name, Lines: []ReferralLine{}, Open: []ReferralLine{}}
			byName[name] = s
			codes[name] = map[string]bool{}
		}
		return s
	}
	inMonth := func(t *time.Time) bool { return t != nil && !t.Before(m.Start) && t.Before(m.End) }

	for i := range orders {
		o := &orders[i]
		if o.ReferrerName == "" {
			continue
		}
		switch {
		case o.Status == StatusFulfilled && inMonth(o.FulfilledAt):
			s := statement(o.ReferrerName)
			line := referralLine(o)
			s.Lines = append(s.Lines, line)
			s.Orders++
			s.BaseWon += line.BaseWon
			s.CommissionWon += line.CommissionWon
			codes[o.ReferrerName][o.PromotionCode] = true
		case isReferralOpen(o.Status):
			s := statement(o.ReferrerName)
			line := referralLine(o)
			s.Open = append(s.Open, line)
			s.OpenOrders++
			s.OpenCommissionWon += line.CommissionWon
			codes[o.ReferrerName][o.PromotionCode] = true
		}
	}

	report := ReferralReport{
		Month:          m.Month,
		From:           m.Start.Format(reportperiod.DateLayout),
		To:             m.End.AddDate(0, 0, -1).Format(reportperiod.DateLayout),
		Timezone:       reportperiod.Timezone,
		WithholdingBps: WithholdingBps,
		Referrers:      make([]ReferrerStatement, 0, len(byName)),
	}
	for name, s := range byName {
		s.IncomeTaxWon, s.LocalTaxWon = ReferralWithholding(s.CommissionWon)
		s.WithholdingWon = s.IncomeTaxWon + s.LocalTaxWon
		s.NetWon = s.CommissionWon - s.WithholdingWon
		for code := range codes[name] {
			s.Codes = append(s.Codes, code)
		}
		sort.Strings(s.Codes)
		sort.Slice(s.Lines, func(i, j int) bool { return lineTime(s.Lines[i]).Before(lineTime(s.Lines[j])) })
		sort.Slice(s.Open, func(i, j int) bool { return lineTime(s.Open[i]).Before(lineTime(s.Open[j])) })
		report.Referrers = append(report.Referrers, *s)

		t := &report.Totals
		t.Orders += s.Orders
		t.BaseWon += s.BaseWon
		t.CommissionWon += s.CommissionWon
		t.WithholdingWon += s.WithholdingWon
		t.NetWon += s.NetWon
		t.OpenOrders += s.OpenOrders
		t.OpenCommissionWon += s.OpenCommissionWon
	}
	sort.Slice(report.Referrers, func(i, j int) bool {
		return report.Referrers[i].ReferrerName < report.Referrers[j].ReferrerName
	})
	return report
}

func lineTime(l ReferralLine) time.Time {
	if l.FulfilledAt != nil {
		return *l.FulfilledAt
	}
	if l.PaidAt != nil {
		return *l.PaidAt
	}
	return time.Time{}
}
