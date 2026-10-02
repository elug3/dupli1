package reportperiod_test

import (
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

func kst(y int, m time.Month, d, h int) time.Time {
	return time.Date(y, m, d, h, 0, 0, 0, reportperiod.Location)
}

func TestNewRange_DefaultWeeksEndWithTheCurrentKSTWeek(t *testing.T) {
	// Thursday 23:30 UTC is already Friday 08:30 KST.
	now := time.Date(2026, 10, 1, 23, 30, 0, 0, time.UTC)
	r, err := reportperiod.NewRange("", "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Granularity != reportperiod.Week {
		t.Fatalf("granularity = %q, want week", r.Granularity)
	}
	if !r.End.Equal(kst(2026, 10, 5, 0)) || !r.Start.Equal(kst(2026, 7, 13, 0)) {
		t.Fatalf("range = [%v, %v), want [2026-07-13, 2026-10-05) KST", r.Start, r.End)
	}
	if n := len(r.Periods()); n != reportperiod.DefaultWeeks {
		t.Fatalf("periods = %d, want %d", n, reportperiod.DefaultWeeks)
	}
}

func TestNewRange_WeekStartsMonday(t *testing.T) {
	// Sunday 2026-09-20 belongs to the week of Monday 09-14.
	r, err := reportperiod.NewRange("week", "2026-09-20", "2026-09-21", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ps := r.Periods()
	if len(ps) != 2 || ps[0].StartDate() != "2026-09-14" || ps[0].EndDate() != "2026-09-20" || ps[1].StartDate() != "2026-09-21" {
		t.Fatalf("periods = %+v", ps)
	}
}

func TestNewRange_SnapsToWholeMonths(t *testing.T) {
	r, err := reportperiod.NewRange("month", "2026-01-15", "2026-03-02", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r.StartDate() != "2026-01-01" || r.EndDate() != "2026-03-31" {
		t.Fatalf("range = %s..%s, want 2026-01-01..2026-03-31", r.StartDate(), r.EndDate())
	}
	if ps := r.Periods(); len(ps) != 3 || ps[1].EndDate() != "2026-02-28" {
		t.Fatalf("periods = %+v", ps)
	}
}

func TestNewRange_Rejects(t *testing.T) {
	now := kst(2026, 10, 2, 12)
	for name, args := range map[string][3]string{
		"granularity":     {"day", "", ""},
		"bad date":        {"week", "2026/01/01", ""},
		"from after to":   {"week", "2026-10-01", "2026-09-01"},
		"too many weeks":  {"week", "2023-01-01", "2026-10-01"},
		"too many months": {"month", "2020-01-01", "2026-10-01"},
	} {
		if _, err := reportperiod.NewRange(args[0], args[1], args[2], now); !errors.Is(err, reportperiod.ErrInvalidRange) {
			t.Errorf("%s: err = %v, want ErrInvalidRange", name, err)
		}
	}
}

func TestIndex(t *testing.T) {
	r, err := reportperiod.NewRange("week", "2026-09-14", "2026-09-27", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		at   time.Time
		want int
	}{
		{kst(2026, 9, 14, 0), 0},
		{kst(2026, 9, 20, 23), 0},
		{kst(2026, 9, 21, 0), 1},
		// 2026-09-20 15:00 UTC is Monday 00:00 KST.
		{time.Date(2026, 9, 20, 15, 0, 0, 0, time.UTC), 1},
		{kst(2026, 9, 13, 23), -1},
		{kst(2026, 9, 28, 0), -1},
	}
	for _, c := range cases {
		if got := r.Index(c.at); got != c.want {
			t.Errorf("Index(%v) = %d, want %d", c.at, got, c.want)
		}
	}
}
