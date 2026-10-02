package domain

import (
	"time"

	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

// RegistrationPeriod is one week or month of customer sign-ups.
type RegistrationPeriod struct {
	PeriodStart  string `json:"period_start"` // first day, YYYY-MM-DD KST
	PeriodEnd    string `json:"period_end"`   // last day, inclusive
	NewCustomers int    `json:"new_customers"`
}

// RegistrationReport is the response of GET /api/v1/auth/reports/registrations.
type RegistrationReport struct {
	Granularity reportperiod.Granularity `json:"granularity"`
	Timezone    string                   `json:"timezone"`
	From        string                   `json:"from"`
	To          string                   `json:"to"`
	Periods     []RegistrationPeriod     `json:"periods"`
	// TotalNewCustomers sums Periods.
	TotalNewCustomers int `json:"total_new_customers"`
	// UndatedCustomers signed up before auth recorded sign-up times, so they
	// are in no period.
	UndatedCustomers int `json:"undated_customers"`
}

// BuildRegistrationReport counts sign-up times into every period of r,
// empty periods included; times outside r are ignored.
func BuildRegistrationReport(r reportperiod.Range, times []time.Time, undated int) RegistrationReport {
	periods := make([]RegistrationPeriod, 0)
	for _, p := range r.Periods() {
		periods = append(periods, RegistrationPeriod{PeriodStart: p.StartDate(), PeriodEnd: p.EndDate()})
	}
	total := 0
	for _, t := range times {
		if i := r.Index(t); i >= 0 {
			periods[i].NewCustomers++
			total++
		}
	}
	return RegistrationReport{
		Granularity:       r.Granularity,
		Timezone:          reportperiod.Timezone,
		From:              r.StartDate(),
		To:                r.EndDate(),
		Periods:           periods,
		TotalNewCustomers: total,
		UndatedCustomers:  undated,
	}
}
