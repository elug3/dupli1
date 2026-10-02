package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

// VisitorStore is an in-memory ports.VisitorStore for tests.
type VisitorStore struct {
	mu     sync.Mutex
	visits map[visitKey]struct{}
}

type visitKey struct {
	day     string // YYYY-MM-DD KST
	guestID string
}

func NewVisitorStore() *VisitorStore {
	return &VisitorStore{visits: make(map[visitKey]struct{})}
}

func (s *VisitorStore) RecordVisit(_ context.Context, day time.Time, guestID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := visitKey{day: day.In(reportperiod.Location).Format(reportperiod.DateLayout), guestID: guestID}
	if _, ok := s.visits[k]; ok {
		return false, nil
	}
	s.visits[k] = struct{}{}
	return true, nil
}

func (s *VisitorStore) VisitorCounts(_ context.Context, g reportperiod.Granularity, start, end time.Time) ([]domain.VisitorPeriodCount, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	type bucket struct {
		start  time.Time
		guests map[string]struct{}
		days   int
	}
	buckets := map[time.Time]*bucket{}
	all := map[string]struct{}{}
	for k := range s.visits {
		day, err := time.ParseInLocation(reportperiod.DateLayout, k.day, reportperiod.Location)
		if err != nil || day.Before(start) || !day.Before(end) {
			continue
		}
		p := g.PeriodStart(day)
		b := buckets[p]
		if b == nil {
			b = &bucket{start: p, guests: map[string]struct{}{}}
			buckets[p] = b
		}
		b.guests[k.guestID] = struct{}{}
		b.days++
		all[k.guestID] = struct{}{}
	}
	out := make([]domain.VisitorPeriodCount, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, domain.VisitorPeriodCount{Start: b.start, UniqueVisitors: len(b.guests), VisitorDays: b.days})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, len(all), nil
}
