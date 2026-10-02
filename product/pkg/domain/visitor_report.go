package domain

import (
	"time"

	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

// VisitorPeriodCount is what the store reads for one period: browsers seen
// at least once in it, and browser-days (each browser once per KST day).
type VisitorPeriodCount struct {
	Start          time.Time // period start, midnight KST
	UniqueVisitors int
	VisitorDays    int
}

// VisitorPeriod is one week or month of storefront visitors.
type VisitorPeriod struct {
	PeriodStart string `json:"period_start"` // first day, YYYY-MM-DD KST
	PeriodEnd   string `json:"period_end"`   // last day, inclusive
	// UniqueVisitors counts each browser once per period, however many days
	// it came back.
	UniqueVisitors int `json:"unique_visitors"`
	// VisitorDays counts each browser once per day it visited, so it is the
	// sum of the period's daily unique visitors.
	VisitorDays int `json:"visitor_days"`
}

// VisitorDay is the unique visitors of one KST day.
type VisitorDay struct {
	Date           string `json:"date"` // YYYY-MM-DD KST
	UniqueVisitors int    `json:"unique_visitors"`
}

// VisitorReport is the response of GET /api/v1/products/reports/visitors.
type VisitorReport struct {
	Granularity reportperiod.Granularity `json:"granularity"`
	Timezone    string                   `json:"timezone"`
	From        string                   `json:"from"`
	To          string                   `json:"to"`
	Periods     []VisitorPeriod          `json:"periods"`
	// TotalUniqueVisitors counts each browser once across the whole range, so
	// it is not the sum of Periods.
	TotalUniqueVisitors int `json:"total_unique_visitors"`
	// Today is the current KST day, whatever range was asked for.
	Today VisitorDay `json:"today"`
}

// BuildVisitorReport lays the store's counts onto every period of r, empty
// periods included; counts for a period outside r are ignored.
func BuildVisitorReport(r reportperiod.Range, counts []VisitorPeriodCount, total int, today VisitorDay) VisitorReport {
	periods := make([]VisitorPeriod, 0)
	for _, p := range r.Periods() {
		periods = append(periods, VisitorPeriod{PeriodStart: p.StartDate(), PeriodEnd: p.EndDate()})
	}
	for _, c := range counts {
		if i := r.Index(c.Start); i >= 0 {
			periods[i].UniqueVisitors += c.UniqueVisitors
			periods[i].VisitorDays += c.VisitorDays
		}
	}
	return VisitorReport{
		Granularity:         r.Granularity,
		Timezone:            reportperiod.Timezone,
		From:                r.StartDate(),
		To:                  r.EndDate(),
		Periods:             periods,
		TotalUniqueVisitors: total,
		Today:               today,
	}
}
