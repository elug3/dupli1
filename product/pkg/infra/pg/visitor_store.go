package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

// VisitorStore keeps one row per browser per KST day in site_visitors.
// Implements ports.VisitorStore.
type VisitorStore struct {
	pool *pgxpool.Pool
}

func NewVisitorStore(pool *pgxpool.Pool) (*VisitorStore, error) {
	s := &VisitorStore{pool: pool}
	// Startup schema migration; process lifetime only, like the other stores.
	if _, err := pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS site_visitors (
			visit_day     DATE NOT NULL,
			guest_id      TEXT NOT NULL,
			first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (visit_day, guest_id)
		)
	`); err != nil {
		return nil, fmt.Errorf("migrate site_visitors: %w", err)
	}
	return s, nil
}

func (s *VisitorStore) RecordVisit(ctx context.Context, day time.Time, guestID string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO site_visitors (visit_day, guest_id) VALUES ($1::date, $2) ON CONFLICT DO NOTHING`,
		day.Format(reportperiod.DateLayout), guestID,
	)
	if err != nil {
		return false, wrapDB("record visit", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *VisitorStore) VisitorCounts(ctx context.Context, g reportperiod.Granularity, start, end time.Time) ([]domain.VisitorPeriodCount, int, error) {
	if g != reportperiod.Week && g != reportperiod.Month {
		return nil, 0, fmt.Errorf("visitor counts: unsupported granularity %q", g)
	}
	from, to := start.Format(reportperiod.DateLayout), end.Format(reportperiod.DateLayout)

	// date_trunc('week') starts weeks on Monday, matching reportperiod.
	rows, err := s.pool.Query(ctx, `
		SELECT to_char(date_trunc($1, visit_day::timestamp), 'YYYY-MM-DD'),
		       COUNT(DISTINCT guest_id), COUNT(*)
		  FROM site_visitors
		 WHERE visit_day >= $2::date AND visit_day < $3::date
		 GROUP BY 1
		 ORDER BY 1`,
		string(g), from, to,
	)
	if err != nil {
		return nil, 0, wrapDB("visitor counts", err)
	}
	defer rows.Close()
	var out []domain.VisitorPeriodCount
	for rows.Next() {
		var periodStart string
		var c domain.VisitorPeriodCount
		if err := rows.Scan(&periodStart, &c.UniqueVisitors, &c.VisitorDays); err != nil {
			return nil, 0, wrapDB("visitor counts scan", err)
		}
		c.Start, err = time.ParseInLocation(reportperiod.DateLayout, periodStart, reportperiod.Location)
		if err != nil {
			return nil, 0, fmt.Errorf("visitor counts: period %q: %w", periodStart, err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, wrapDB("visitor counts rows", err)
	}

	var total int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT guest_id) FROM site_visitors
		 WHERE visit_day >= $1::date AND visit_day < $2::date`,
		from, to,
	).Scan(&total); err != nil {
		return nil, 0, wrapDB("visitor total", err)
	}
	return out, total, nil
}
