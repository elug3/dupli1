package pg

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

func kstDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, reportperiod.Location)
}

func TestVisitorStore_CountsUniquePerPeriodAndRange(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, "visitor_store_test")
	ctx := t.Context()

	store, err := NewVisitorStore(pool)
	if err != nil {
		t.Fatalf("NewVisitorStore: %v", err)
	}
	// 2026-09-28 is a Monday.
	visits := []struct {
		day   time.Time
		guest string
	}{
		{kstDate(2026, 9, 28), "a"},
		{kstDate(2026, 9, 28), "a"}, // same day again: not counted
		{kstDate(2026, 9, 29), "a"},
		{kstDate(2026, 9, 29), "b"},
		{kstDate(2026, 10, 5), "a"}, // next week
	}
	for i, v := range visits {
		inserted, err := store.RecordVisit(ctx, v.day, v.guest)
		if err != nil {
			t.Fatalf("RecordVisit %d: %v", i, err)
		}
		if want := i != 1; inserted != want {
			t.Fatalf("RecordVisit %d inserted=%v, want %v", i, inserted, want)
		}
	}

	counts, total, err := store.VisitorCounts(ctx, reportperiod.Week, kstDate(2026, 9, 28), kstDate(2026, 10, 12))
	if err != nil {
		t.Fatalf("VisitorCounts: %v", err)
	}
	if total != 2 {
		t.Fatalf("total=%d, want 2", total)
	}
	if len(counts) != 2 {
		t.Fatalf("counts=%+v, want 2 weeks", counts)
	}
	if !counts[0].Start.Equal(kstDate(2026, 9, 28)) || counts[0].UniqueVisitors != 2 || counts[0].VisitorDays != 3 {
		t.Fatalf("week 1 = %+v, want start 09-28, 2 unique, 3 visitor-days", counts[0])
	}
	if !counts[1].Start.Equal(kstDate(2026, 10, 5)) || counts[1].UniqueVisitors != 1 {
		t.Fatalf("week 2 = %+v, want start 10-05, 1 unique", counts[1])
	}

	months, _, err := store.VisitorCounts(ctx, reportperiod.Month, kstDate(2026, 9, 1), kstDate(2026, 11, 1))
	if err != nil {
		t.Fatalf("VisitorCounts month: %v", err)
	}
	if len(months) != 2 || months[0].UniqueVisitors != 2 || months[1].UniqueVisitors != 1 {
		t.Fatalf("months=%+v, want Sep 2 unique, Oct 1 unique", months)
	}
}
