// Package reportperiod splits time into the weeks and months admin reports
// are read in: KST, weeks Monday to Sunday, calendar months. Order's sales
// report and auth's sign-up counts share it so their periods always line up.
package reportperiod

import (
	"errors"
	"strings"
	"time"
)

// Location is the reporting calendar. KST has no daylight saving, so a fixed
// zone needs no tzdata in the image.
var Location = time.FixedZone("KST", 9*60*60)

// Timezone is Location's IANA name, for responses.
const Timezone = "Asia/Seoul"

// DateLayout is how period boundaries are read and written.
const DateLayout = "2006-01-02"

// Granularity is the length of one period.
type Granularity string

const (
	Week  Granularity = "week"  // Monday to Sunday, KST
	Month Granularity = "month" // calendar month, KST
)

// Defaults and caps on how many periods one report covers.
const (
	DefaultWeeks  = 12
	DefaultMonths = 12
	MaxWeeks      = 104
	MaxMonths     = 36
)

var ErrInvalidRange = errors.New("granularity must be week or month, from/to must be YYYY-MM-DD with from not after to, and the range at most 104 weeks or 36 months")

// Range is the half-open span [Start, End) of whole periods a report covers.
type Range struct {
	Granularity Granularity
	Start       time.Time
	End         time.Time
}

// Period is one week or month of a Range.
type Period struct {
	Start time.Time
	End   time.Time // exclusive
}

// StartDate is the period's first day, YYYY-MM-DD.
func (p Period) StartDate() string { return p.Start.Format(DateLayout) }

// EndDate is the period's last day (inclusive), YYYY-MM-DD.
func (p Period) EndDate() string { return p.End.AddDate(0, 0, -1).Format(DateLayout) }

// NewRange parses granularity (default week) and optional from/to dates
// (YYYY-MM-DD, KST, both inclusive) and widens them to whole periods. A
// missing to means the period containing now; a missing from means the
// default number of periods ending with to's.
func NewRange(granularity, from, to string, now time.Time) (Range, error) {
	g := Granularity(strings.ToLower(strings.TrimSpace(granularity)))
	if g == "" {
		g = Week
	}
	if g != Week && g != Month {
		return Range{}, ErrInvalidRange
	}

	toDay := now
	if s := strings.TrimSpace(to); s != "" {
		d, err := time.ParseInLocation(DateLayout, s, Location)
		if err != nil {
			return Range{}, ErrInvalidRange
		}
		toDay = d
	}
	end := g.next(g.PeriodStart(toDay))

	var start time.Time
	if s := strings.TrimSpace(from); s != "" {
		d, err := time.ParseInLocation(DateLayout, s, Location)
		if err != nil {
			return Range{}, ErrInvalidRange
		}
		start = g.PeriodStart(d)
	} else {
		start = end
		for i := 0; i < g.defaultPeriods(); i++ {
			start = g.prev(start)
		}
	}
	if !start.Before(end) {
		return Range{}, ErrInvalidRange
	}

	r := Range{Granularity: g, Start: start, End: end}
	if len(r.Periods()) > g.maxPeriods() {
		return Range{}, ErrInvalidRange
	}
	return r, nil
}

// Periods lists every period of r in order.
func (r Range) Periods() []Period {
	var out []Period
	for p := r.Start; p.Before(r.End); p = r.Granularity.next(p) {
		out = append(out, Period{Start: p, End: r.Granularity.next(p)})
	}
	return out
}

// Index is the position in Periods of the period holding t, or -1 when t is
// outside r.
func (r Range) Index(t time.Time) int {
	if t.Before(r.Start) || !t.Before(r.End) {
		return -1
	}
	target := r.Granularity.PeriodStart(t)
	i := 0
	for p := r.Start; p.Before(target); p = r.Granularity.next(p) {
		i++
	}
	return i
}

// StartDate is r's first day, YYYY-MM-DD.
func (r Range) StartDate() string { return r.Start.Format(DateLayout) }

// EndDate is r's last day (inclusive), YYYY-MM-DD.
func (r Range) EndDate() string { return r.End.AddDate(0, 0, -1).Format(DateLayout) }

// PeriodStart is midnight KST on the first day of the period holding t.
func (g Granularity) PeriodStart(t time.Time) time.Time {
	t = t.In(Location)
	if g == Month {
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, Location)
	}
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, Location)
	sinceMonday := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -sinceMonday)
}

func (g Granularity) next(start time.Time) time.Time {
	if g == Month {
		return start.AddDate(0, 1, 0)
	}
	return start.AddDate(0, 0, 7)
}

func (g Granularity) prev(start time.Time) time.Time {
	if g == Month {
		return start.AddDate(0, -1, 0)
	}
	return start.AddDate(0, 0, -7)
}

func (g Granularity) defaultPeriods() int {
	if g == Month {
		return DefaultMonths
	}
	return DefaultWeeks
}

func (g Granularity) maxPeriods() int {
	if g == Month {
		return MaxMonths
	}
	return MaxWeeks
}
