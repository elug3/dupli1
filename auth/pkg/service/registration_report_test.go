package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

type fakeRegistrationStats struct {
	accountType string
	start, end  time.Time
	times       []time.Time
	undated     int
}

func (f *fakeRegistrationStats) RegistrationTimes(_ context.Context, accountType string, start, end time.Time) ([]time.Time, int, error) {
	f.accountType, f.start, f.end = accountType, start, end
	return f.times, f.undated, nil
}

func TestRegisterStampsCreatedAt(t *testing.T) {
	repo := &fakeUserRepository{}
	svc := NewService(repo, nil)
	now := time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	if _, err := svc.Register(t.Context(), "new@example.com", "supersecret", domain.AccountTypeCustomer); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if repo.saved.CreatedAt == nil || !repo.saved.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %v, want %v", repo.saved.CreatedAt, now)
	}
}

func TestRegistrationReportCountsCustomersPerWeek(t *testing.T) {
	stats := &fakeRegistrationStats{
		times:   []time.Time{time.Date(2026, 9, 30, 12, 0, 0, 0, reportperiod.Location)},
		undated: 4,
	}
	svc := NewService(&fakeUserRepository{}, nil, WithRegistrationStats(stats))
	svc.now = func() time.Time { return time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC) }

	rep, err := svc.RegistrationReport(t.Context(), "", "", "")
	if err != nil {
		t.Fatalf("RegistrationReport: %v", err)
	}
	if stats.accountType != domain.AccountTypeCustomer {
		t.Fatalf("queried account type %q, want customer", stats.accountType)
	}
	if len(rep.Periods) != reportperiod.DefaultWeeks || rep.Periods[len(rep.Periods)-1].NewCustomers != 1 {
		t.Fatalf("periods = %+v", rep.Periods)
	}
	if rep.UndatedCustomers != 4 || rep.To != "2026-10-04" {
		t.Fatalf("report = %+v", rep)
	}
}

func TestRegistrationReportErrors(t *testing.T) {
	svc := NewService(&fakeUserRepository{}, nil)
	if _, err := svc.RegistrationReport(t.Context(), "day", "", ""); !errors.Is(err, reportperiod.ErrInvalidRange) {
		t.Fatalf("bad granularity err = %v, want ErrInvalidRange", err)
	}
	if _, err := svc.RegistrationReport(t.Context(), "week", "", ""); !errors.Is(err, ErrReportsUnavailable) {
		t.Fatalf("no stats err = %v, want ErrReportsUnavailable", err)
	}
}
