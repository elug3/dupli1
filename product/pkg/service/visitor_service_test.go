package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

func TestVisitorService_ReportCountsEachBrowserOncePerPeriod(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 28, 1, 0, 0, 0, time.UTC) // Mon 10:00 KST
	svc := service.NewVisitorService(memory.NewVisitorStore()).WithClock(func() time.Time { return now })

	record := func(guest string) {
		t.Helper()
		if _, err := svc.RecordVisit(ctx, guest); err != nil {
			t.Fatalf("RecordVisit(%s): %v", guest, err)
		}
	}
	record("a")
	record("a")
	record("b")
	now = now.Add(24 * time.Hour) // Tuesday
	record("a")
	now = now.Add(7 * 24 * time.Hour) // next Tuesday
	record("c")

	report, err := svc.Report(ctx, "week", "2026-09-28", "")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if report.Timezone != reportperiod.Timezone || report.From != "2026-09-28" || report.To != "2026-10-11" {
		t.Fatalf("report range = %s..%s %s", report.From, report.To, report.Timezone)
	}
	if len(report.Periods) != 2 {
		t.Fatalf("periods=%+v, want 2", report.Periods)
	}
	if p := report.Periods[0]; p.UniqueVisitors != 2 || p.VisitorDays != 3 {
		t.Fatalf("week 1 = %+v, want 2 unique / 3 visitor-days", p)
	}
	if p := report.Periods[1]; p.UniqueVisitors != 1 || p.VisitorDays != 1 {
		t.Fatalf("week 2 = %+v, want 1 unique / 1 visitor-day", p)
	}
	if report.TotalUniqueVisitors != 3 {
		t.Fatalf("total=%d, want 3", report.TotalUniqueVisitors)
	}
	if report.Today.Date != "2026-10-06" || report.Today.UniqueVisitors != 1 {
		t.Fatalf("today=%+v, want 2026-10-06 with 1", report.Today)
	}
}

func TestVisitorService_DayRollsOverAtKSTMidnight(t *testing.T) {
	ctx := context.Background()
	// 14:59 UTC is 23:59 KST; a minute later is the next KST day.
	now := time.Date(2026, 9, 28, 14, 59, 0, 0, time.UTC)
	svc := service.NewVisitorService(memory.NewVisitorStore()).WithClock(func() time.Time { return now })

	if _, err := svc.RecordVisit(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	inserted, err := svc.RecordVisit(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("visit after KST midnight should count as a new day")
	}
}

func TestVisitorService_RejectsBadInput(t *testing.T) {
	svc := service.NewVisitorService(memory.NewVisitorStore())
	if _, err := svc.RecordVisit(context.Background(), " "); err != service.ErrInvalidGuestID {
		t.Fatalf("empty guest: err=%v", err)
	}
	if _, err := svc.Report(context.Background(), "day", "", ""); err != reportperiod.ErrInvalidRange {
		t.Fatalf("granularity day: err=%v", err)
	}
}
