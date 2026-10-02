package domain_test

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

func TestBuildRegistrationReport(t *testing.T) {
	r, err := reportperiod.NewRange("week", "2026-09-14", "2026-09-27", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	kst := func(d, h int) time.Time { return time.Date(2026, 9, d, h, 0, 0, 0, reportperiod.Location) }
	rep := domain.BuildRegistrationReport(r, []time.Time{
		kst(14, 0), kst(20, 23), // week of 09-14
		kst(21, 0),  // week of 09-21
		kst(13, 23), // before the range
	}, 7)

	if len(rep.Periods) != 2 || rep.Periods[0].NewCustomers != 2 || rep.Periods[1].NewCustomers != 1 {
		t.Fatalf("periods = %+v", rep.Periods)
	}
	if rep.TotalNewCustomers != 3 || rep.UndatedCustomers != 7 {
		t.Fatalf("total = %d undated = %d, want 3 and 7", rep.TotalNewCustomers, rep.UndatedCustomers)
	}
	if rep.From != "2026-09-14" || rep.To != "2026-09-27" || rep.Periods[1].PeriodEnd != "2026-09-27" {
		t.Fatalf("report = %+v", rep)
	}
}
