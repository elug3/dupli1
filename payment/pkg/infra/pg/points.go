package pg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/jackc/pgx/v4"
)

// Points ledger tables (docs/payment-points.md). A lot is unique per
// (source, ref) across customers: an order or review id names one thing.
var pointsTableDDL = []string{
	`CREATE TABLE IF NOT EXISTS payment_point_lots (
		id             TEXT PRIMARY KEY,
		customer_id    TEXT NOT NULL,
		source         TEXT NOT NULL,
		ref            TEXT NOT NULL,
		status         TEXT NOT NULL CHECK (status IN ('pending', 'available', 'expired', 'revoked')),
		points         BIGINT NOT NULL CHECK (points >= 0),
		remaining      BIGINT NOT NULL CHECK (remaining >= 0),
		base_won       BIGINT NOT NULL DEFAULT 0,
		multiplier     INT NOT NULL DEFAULT 0,
		expired_points BIGINT NOT NULL DEFAULT 0,
		note           TEXT NOT NULL DEFAULT '',
		created_at     TIMESTAMPTZ NOT NULL,
		credited_at    TIMESTAMPTZ,
		expires_at     TIMESTAMPTZ,
		UNIQUE (source, ref)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_payment_point_lots_customer ON payment_point_lots (customer_id)`,
	`CREATE INDEX IF NOT EXISTS idx_payment_point_lots_lapsing ON payment_point_lots (expires_at) WHERE status = 'available'`,
	`CREATE TABLE IF NOT EXISTS payment_point_holds (
		order_id    TEXT PRIMARY KEY,
		customer_id TEXT NOT NULL,
		points      BIGINT NOT NULL CHECK (points > 0),
		status      TEXT NOT NULL CHECK (status IN ('reserved', 'consumed', 'released')),
		allocations JSONB NOT NULL DEFAULT '[]',
		created_at  TIMESTAMPTZ NOT NULL,
		updated_at  TIMESTAMPTZ NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_payment_point_holds_customer ON payment_point_holds (customer_id)`,
	`CREATE TABLE IF NOT EXISTS payment_point_entries (
		id          TEXT PRIMARY KEY,
		customer_id TEXT NOT NULL,
		kind        TEXT NOT NULL,
		points      BIGINT NOT NULL,
		order_id    TEXT NOT NULL DEFAULT '',
		ref         TEXT NOT NULL DEFAULT '',
		note        TEXT NOT NULL DEFAULT '',
		created_at  TIMESTAMPTZ NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_payment_point_entries_customer ON payment_point_entries (customer_id, created_at DESC)`,
}

const lotSelect = `SELECT id, customer_id, source, ref, status, points, remaining, base_won, multiplier,
	expired_points, note, created_at, credited_at, expires_at FROM payment_point_lots`

const holdSelect = `SELECT order_id, customer_id, points, status, allocations, created_at, updated_at FROM payment_point_holds`

type rowQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func loadPointAccount(ctx context.Context, q rowQuerier, customerID string) (*domain.PointAccount, error) {
	rows, err := q.Query(ctx, lotSelect+` WHERE customer_id = $1 ORDER BY created_at, id`, customerID)
	if err != nil {
		return nil, fmt.Errorf("load point lots: %w", err)
	}
	var lots []*domain.PointLot
	for rows.Next() {
		var l domain.PointLot
		if err := rows.Scan(&l.ID, &l.CustomerID, &l.Source, &l.Ref, &l.Status, &l.Points, &l.Remaining,
			&l.BaseWon, &l.Multiplier, &l.ExpiredPoints, &l.Note, &l.CreatedAt, &l.CreditedAt, &l.ExpiresAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan point lot: %w", err)
		}
		l.CreatedAt = l.CreatedAt.UTC()
		lots = append(lots, &l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, holdSelect+` WHERE customer_id = $1 ORDER BY created_at, order_id`, customerID)
	if err != nil {
		return nil, fmt.Errorf("load point holds: %w", err)
	}
	defer rows.Close()
	var holds []*domain.PointHold
	for rows.Next() {
		h, err := scanHold(rows)
		if err != nil {
			return nil, err
		}
		holds = append(holds, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return domain.NewPointAccount(customerID, lots, holds, domain.NewPointID), nil
}

func scanHold(row pgx.Row) (*domain.PointHold, error) {
	var h domain.PointHold
	var alloc []byte
	if err := row.Scan(&h.OrderID, &h.CustomerID, &h.Points, &h.Status, &alloc, &h.CreatedAt, &h.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, fmt.Errorf("scan point hold: %w", err)
	}
	if err := json.Unmarshal(alloc, &h.Allocations); err != nil {
		return nil, fmt.Errorf("decode point hold allocations: %w", err)
	}
	return &h, nil
}

func (r *Repository) MutatePointAccount(ctx context.Context, customerID string, mutate func(*domain.PointAccount) error) (*domain.PointAccount, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// One customer's ledger changes one transaction at a time; different
	// customers never wait on each other.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('payment_points:' || $1))`, customerID); err != nil {
		return nil, fmt.Errorf("lock point account: %w", err)
	}
	acct, err := loadPointAccount(ctx, tx, customerID)
	if err != nil {
		return nil, err
	}
	if err := mutate(acct); err != nil {
		return nil, err
	}
	for _, l := range acct.Lots {
		if _, err := tx.Exec(ctx, `
			INSERT INTO payment_point_lots (id, customer_id, source, ref, status, points, remaining, base_won,
				multiplier, expired_points, note, created_at, credited_at, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (id) DO UPDATE SET
				status = EXCLUDED.status, points = EXCLUDED.points, remaining = EXCLUDED.remaining,
				base_won = EXCLUDED.base_won, multiplier = EXCLUDED.multiplier,
				expired_points = EXCLUDED.expired_points, credited_at = EXCLUDED.credited_at,
				expires_at = EXCLUDED.expires_at
		`, l.ID, l.CustomerID, l.Source, l.Ref, l.Status, l.Points, l.Remaining, l.BaseWon, l.Multiplier,
			l.ExpiredPoints, l.Note, l.CreatedAt, l.CreditedAt, l.ExpiresAt); err != nil {
			return nil, fmt.Errorf("save point lot: %w", err)
		}
	}
	for _, h := range acct.Holds {
		alloc, err := json.Marshal(h.Allocations)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO payment_point_holds (order_id, customer_id, points, status, allocations, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (order_id) DO UPDATE SET status = EXCLUDED.status, updated_at = EXCLUDED.updated_at
		`, h.OrderID, h.CustomerID, h.Points, h.Status, alloc, h.CreatedAt, h.UpdatedAt); err != nil {
			return nil, fmt.Errorf("save point hold: %w", err)
		}
	}
	for _, e := range acct.NewEntries {
		if _, err := tx.Exec(ctx, `
			INSERT INTO payment_point_entries (id, customer_id, kind, points, order_id, ref, note, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, e.ID, e.CustomerID, e.Kind, e.Points, e.OrderID, e.Ref, e.Note, e.CreatedAt); err != nil {
			return nil, fmt.Errorf("save point entry: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return acct, nil
}

func (r *Repository) GetPointAccount(ctx context.Context, customerID string) (*domain.PointAccount, error) {
	return loadPointAccount(ctx, r.pool, customerID)
}

func (r *Repository) FindPointHold(ctx context.Context, orderID string) (*domain.PointHold, error) {
	return scanHold(r.pool.QueryRow(ctx, holdSelect+` WHERE order_id = $1`, orderID))
}

func (r *Repository) ListPointEntries(ctx context.Context, customerID string, after ports.PointCursor, limit int) ([]domain.PointEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	var atArg any
	if !after.IsZero() {
		atArg = after.At
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, customer_id, kind, points, order_id, ref, note, created_at
		FROM payment_point_entries
		WHERE customer_id = $1 AND ($2::timestamptz IS NULL OR (created_at, id) < ($2, $3))
		ORDER BY created_at DESC, id DESC
		LIMIT $4
	`, customerID, atArg, after.ID, limit)
	if err != nil {
		return nil, fmt.Errorf("list point entries: %w", err)
	}
	defer rows.Close()
	var out []domain.PointEntry
	for rows.Next() {
		var e domain.PointEntry
		if err := rows.Scan(&e.ID, &e.CustomerID, &e.Kind, &e.Points, &e.OrderID, &e.Ref, &e.Note, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan point entry: %w", err)
		}
		e.CreatedAt = e.CreatedAt.UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) CustomersWithLapsedLots(ctx context.Context, at time.Time, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT customer_id FROM payment_point_lots
		WHERE status = 'available' AND expires_at <= $1
		LIMIT $2
	`, at, limit)
	if err != nil {
		return nil, fmt.Errorf("list lapsed point lots: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
