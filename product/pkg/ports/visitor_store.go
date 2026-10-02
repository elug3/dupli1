package ports

import (
	"context"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

// VisitorStore keeps one row per browser per KST day the storefront was
// visited, and counts them for the visitors report.
type VisitorStore interface {
	// RecordVisit stores (day, guestID) if new. day is midnight KST of the
	// visit's day. Reports whether this was the browser's first visit that day.
	RecordVisit(ctx context.Context, day time.Time, guestID string) (bool, error)
	// VisitorCounts reads unique visitors and visitor-days per period of g for
	// days in [start, end), and the unique visitors across the whole span.
	// Periods with no visits may be omitted.
	VisitorCounts(ctx context.Context, g reportperiod.Granularity, start, end time.Time) (periods []domain.VisitorPeriodCount, total int, err error)
}
