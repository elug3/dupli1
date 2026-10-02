package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

// ErrInvalidGuestID is returned when a visit names no browser.
var ErrInvalidGuestID = errors.New("guest id is required")

// VisitorService counts unique storefront visitors: one visit per browser
// (the dupli1_guest cookie) per KST day, read back per week or month.
type VisitorService struct {
	store ports.VisitorStore
	now   func() time.Time
}

func NewVisitorService(store ports.VisitorStore) *VisitorService {
	return &VisitorService{store: store, now: time.Now}
}

// WithClock replaces time.Now, for tests.
func (s *VisitorService) WithClock(now func() time.Time) *VisitorService {
	s.now = now
	return s
}

// RecordVisit counts guestID as a visitor today (KST). Repeat visits the same
// day are no-ops; it reports whether this one was new.
func (s *VisitorService) RecordVisit(ctx context.Context, guestID string) (bool, error) {
	guestID = strings.TrimSpace(guestID)
	if guestID == "" {
		return false, ErrInvalidGuestID
	}
	return s.store.RecordVisit(ctx, kstDay(s.now()), guestID)
}

// Report counts unique visitors per week or month; see reportperiod.NewRange
// for how granularity, from and to are read.
func (s *VisitorService) Report(ctx context.Context, granularity, from, to string) (domain.VisitorReport, error) {
	now := s.now()
	r, err := reportperiod.NewRange(granularity, from, to, now)
	if err != nil {
		return domain.VisitorReport{}, err
	}
	counts, total, err := s.store.VisitorCounts(ctx, r.Granularity, r.Start, r.End)
	if err != nil {
		return domain.VisitorReport{}, err
	}
	today := kstDay(now)
	_, todayTotal, err := s.store.VisitorCounts(ctx, r.Granularity, today, today.AddDate(0, 0, 1))
	if err != nil {
		return domain.VisitorReport{}, err
	}
	day := domain.VisitorDay{Date: today.Format(reportperiod.DateLayout), UniqueVisitors: todayTotal}
	return domain.BuildVisitorReport(r, counts, total, day), nil
}

// kstDay is midnight KST on t's day.
func kstDay(t time.Time) time.Time {
	t = t.In(reportperiod.Location)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, reportperiod.Location)
}
