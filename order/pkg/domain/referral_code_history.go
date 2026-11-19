package domain

import (
	"errors"
	"sort"
	"strings"

	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

// One referral code's whole history (docs/product-referral-codes.md), for the
// console's code detail page: every paid order placed with it, what they
// sold, and the commission it earned per KST month.
//
// The monthly payout is still the referrer statement of
// GET /api/v1/orders/reports/referrals: withholding is worked out on a
// referrer's monthly total, so when one referrer has several codes the
// withholding shown per code here can differ from theirs by the ten-won
// rounding. With one code per referrer (the usual case) they are the same.

// ErrInvalidReferralCode is an empty code.
var ErrInvalidReferralCode = errors.New("code is required")

// NormalizeReferralCode is how order stores a code: trimmed, upper case.
func NormalizeReferralCode(code string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return "", ErrInvalidReferralCode
	}
	return code, nil
}

// ReferralCodeMonth is what the code earned in one KST month: its orders
// fulfilled in that month.
type ReferralCodeMonth struct {
	Month          string `json:"month"` // YYYY-MM
	Orders         int    `json:"orders"`
	BaseWon        int64  `json:"base_won"`
	CommissionWon  int64  `json:"commission_won"`
	IncomeTaxWon   int64  `json:"income_tax_won"`
	LocalTaxWon    int64  `json:"local_tax_won"`
	WithholdingWon int64  `json:"withholding_won"`
	NetWon         int64  `json:"net_won"`
}

// ReferralCodeSummary totals the code's paid orders over its whole life.
type ReferralCodeSummary struct {
	// Orders is every paid order placed with the code, canceled ones too.
	Orders          int `json:"orders"`
	FulfilledOrders int `json:"fulfilled_orders"`
	OpenOrders      int `json:"open_orders"`
	CanceledOrders  int `json:"canceled_orders"`
	// Customers is the distinct shoppers among the orders not canceled.
	Customers int `json:"customers"`
	// Goods before the discount, and the discount the code gave, over the
	// orders not canceled (a canceled order was refunded: nothing was sold).
	SubtotalWon int64 `json:"subtotal_won"`
	DiscountWon int64 `json:"discount_won"`
	// Earned on fulfilled orders, and what the open ones will earn.
	BaseWon           int64 `json:"base_won"`
	CommissionWon     int64 `json:"commission_won"`
	OpenCommissionWon int64 `json:"open_commission_won"`
}

// ReferralCodeHistory is the response of
// GET /api/v1/orders/reports/referrals/codes/{code}.
type ReferralCodeHistory struct {
	Code string `json:"code"`
	// ReferrerNames are the names snapshotted on its orders, in order of
	// first use: more than one when the code was renamed along the way.
	ReferrerNames  []string            `json:"referrer_names"`
	Timezone       string              `json:"timezone"`
	WithholdingBps int                 `json:"withholding_bps"`
	Summary        ReferralCodeSummary `json:"summary"`
	// Months with fulfilled orders, newest first.
	Months []ReferralCodeMonth `json:"months"`
	// Every paid order, newest payment first.
	Orders []ReferralLine `json:"orders"`
}

// BuildReferralCodeHistory summarizes the paid referred orders placed with
// code. orders may hold anything: only paid referred orders with that code
// are counted.
func BuildReferralCodeHistory(code string, orders []Order) ReferralCodeHistory {
	h := ReferralCodeHistory{
		Code:           code,
		ReferrerNames:  []string{},
		Timezone:       reportperiod.Timezone,
		WithholdingBps: WithholdingBps,
		Months:         []ReferralCodeMonth{},
		Orders:         []ReferralLine{},
	}
	byMonth := map[string]*ReferralCodeMonth{}
	customers := map[string]bool{}
	names := map[string]bool{}

	// Oldest first, so referrer names come out in order of first use.
	sorted := make([]*Order, 0, len(orders))
	for i := range orders {
		o := &orders[i]
		if o.ReferrerName == "" || o.PromotionCode != code || o.PaidAt == nil {
			continue
		}
		sorted = append(sorted, o)
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].PaidAt.Before(*sorted[j].PaidAt) })

	s := &h.Summary
	for _, o := range sorted {
		if !names[o.ReferrerName] {
			names[o.ReferrerName] = true
			h.ReferrerNames = append(h.ReferrerNames, o.ReferrerName)
		}
		line := referralLine(o)
		h.Orders = append(h.Orders, line)
		s.Orders++
		if o.Status == StatusCanceled {
			s.CanceledOrders++
			continue
		}
		customers[o.CustomerID] = true
		s.SubtotalWon += o.SubtotalWon
		s.DiscountWon += o.DiscountWon
		switch {
		case o.Status == StatusFulfilled:
			s.FulfilledOrders++
			s.BaseWon += line.BaseWon
			s.CommissionWon += line.CommissionWon
			if o.FulfilledAt == nil {
				continue
			}
			key := reportperiod.Month.PeriodStart(*o.FulfilledAt).Format("2006-01")
			m, ok := byMonth[key]
			if !ok {
				m = &ReferralCodeMonth{Month: key}
				byMonth[key] = m
			}
			m.Orders++
			m.BaseWon += line.BaseWon
			m.CommissionWon += line.CommissionWon
		case isReferralOpen(o.Status):
			s.OpenOrders++
			s.OpenCommissionWon += line.CommissionWon
		}
	}
	s.Customers = len(customers)

	// Newest payment first for the order list.
	for i, j := 0, len(h.Orders)-1; i < j; i, j = i+1, j-1 {
		h.Orders[i], h.Orders[j] = h.Orders[j], h.Orders[i]
	}
	for _, m := range byMonth {
		m.IncomeTaxWon, m.LocalTaxWon = ReferralWithholding(m.CommissionWon)
		m.WithholdingWon = m.IncomeTaxWon + m.LocalTaxWon
		m.NetWon = m.CommissionWon - m.WithholdingWon
		h.Months = append(h.Months, *m)
	}
	sort.Slice(h.Months, func(i, j int) bool { return h.Months[i].Month > h.Months[j].Month })
	return h
}
