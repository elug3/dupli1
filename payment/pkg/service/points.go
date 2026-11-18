package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/events"
)

// Points runs the points ledger (docs/payment-points.md): earning from order
// events, holds for checkout, manager adjustments and expiry.
type Points struct {
	store ports.PointsStore
	now   func() time.Time
}

func NewPoints(store ports.PointsStore) *Points {
	return &Points{store: store, now: func() time.Time { return time.Now().UTC() }}
}

// WithClock replaces the clock (tests).
func (p *Points) WithClock(now func() time.Time) *Points {
	p.now = now
	return p
}

// ExpiringWindow is how far ahead the summary warns about expiring points.
const ExpiringWindow = 30 * 24 * time.Hour

// PointsSummary is what a shopper (or a manager looking at one) sees.
type PointsSummary struct {
	CustomerID string `json:"customer_id"`
	// Balance is spendable now; points held by an unfinished checkout are
	// already out of it.
	Balance int64 `json:"balance"`
	// Pending is earned on paid orders not yet fulfilled.
	Pending int64 `json:"pending"`
	// Expiring is spendable points that expire within ExpiringWindow.
	Expiring   int64               `json:"expiring"`
	ExpiringBy time.Time           `json:"expiring_by"`
	Tier       domain.TierStatus   `json:"tier"`
	Entries    []domain.PointEntry `json:"entries"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

// Summary is a customer's balance, tier and latest history.
func (p *Points) Summary(ctx context.Context, customerID string, limit int) (*PointsSummary, error) {
	if strings.TrimSpace(customerID) == "" {
		return nil, domain.ErrInvalidPoints
	}
	acct, err := p.store.GetPointAccount(ctx, customerID)
	if err != nil {
		return nil, err
	}
	now := p.now()
	entries, cursor, err := p.Entries(ctx, customerID, "", limit)
	if err != nil {
		return nil, err
	}
	by := now.Add(ExpiringWindow)
	return &PointsSummary{
		CustomerID: customerID,
		Balance:    acct.Balance(now),
		Pending:    acct.Pending(),
		Expiring:   acct.ExpiringBy(now, by),
		ExpiringBy: by,
		Tier:       acct.TierAt(now),
		Entries:    entries,
		NextCursor: cursor,
	}, nil
}

// Entries pages a customer's history, newest first. The cursor names the
// last entry returned: its created_at (RFC 3339, nanoseconds), "~", its id.
func (p *Points) Entries(ctx context.Context, customerID, cursor string, limit int) ([]domain.PointEntry, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var after ports.PointCursor
	if cursor != "" {
		at, id, ok := strings.Cut(cursor, "~")
		t, err := time.Parse(time.RFC3339Nano, at)
		if !ok || err != nil || id == "" {
			return nil, "", fmt.Errorf("%w: bad cursor", domain.ErrInvalidPoints)
		}
		after = ports.PointCursor{At: t, ID: id}
	}
	entries, err := p.store.ListPointEntries(ctx, customerID, after, limit)
	if err != nil {
		return nil, "", err
	}
	if entries == nil {
		entries = []domain.PointEntry{}
	}
	next := ""
	if len(entries) == limit {
		last := entries[len(entries)-1]
		next = last.CreatedAt.Format(time.RFC3339Nano) + "~" + last.ID
	}
	return entries, next, nil
}

// HandleOrderEvent applies order.paid / order.status_updated to the ledger.
// Every step is idempotent per order, so a replay is harmless.
func (p *Points) HandleOrderEvent(ctx context.Context, _ string, data []byte) error {
	var e events.Order
	if err := json.Unmarshal(data, &e); err != nil {
		return fmt.Errorf("decode order event: %w", err)
	}
	if e.OrderID == "" || e.CustomerID == "" {
		return nil
	}
	at := e.Occurred.UTC()
	if at.IsZero() {
		at = p.now()
	}
	goods := e.SubtotalWon - e.DiscountWon
	switch e.Status {
	case "paid":
		_, err := p.store.MutatePointAccount(ctx, e.CustomerID, func(a *domain.PointAccount) error {
			a.OrderPaid(e.OrderID, goods, at)
			return nil
		})
		return err
	case "fulfilled":
		_, err := p.store.MutatePointAccount(ctx, e.CustomerID, func(a *domain.PointAccount) error {
			a.OrderFulfilled(e.OrderID, goods, at)
			return nil
		})
		return err
	case "canceled":
		var shortfall int64
		_, err := p.store.MutatePointAccount(ctx, e.CustomerID, func(a *domain.PointAccount) error {
			shortfall = a.OrderCanceled(e.OrderID, at)
			return nil
		})
		if err == nil && shortfall > 0 {
			log.Printf("payment points: order %s canceled after its %d points were spent; clawback short by %d", e.OrderID, shortfall, shortfall)
		}
		return err
	}
	return nil
}

// Reserve sets points aside for an order at checkout (internal, order only).
func (p *Points) Reserve(ctx context.Context, orderID, customerID string, points int64) (*domain.PointHold, error) {
	if strings.TrimSpace(orderID) == "" || strings.TrimSpace(customerID) == "" || points <= 0 {
		return nil, domain.ErrInvalidPoints
	}
	// An order's hold belongs to one customer: a second customer asking for
	// the same order id is a conflict, not a second hold.
	if h, err := p.store.FindPointHold(ctx, orderID); err == nil && h.CustomerID != customerID {
		return nil, domain.ErrPointsHoldConflict
	} else if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return nil, err
	}
	var hold *domain.PointHold
	_, err := p.store.MutatePointAccount(ctx, customerID, func(a *domain.PointAccount) error {
		h, err := a.Reserve(orderID, points, p.now())
		hold = h
		return err
	})
	return hold, err
}

// Consume marks an order's held points spent once it is paid.
func (p *Points) Consume(ctx context.Context, orderID string) (*domain.PointHold, error) {
	h, err := p.store.FindPointHold(ctx, orderID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, domain.ErrPointsHoldNotFound
		}
		return nil, err
	}
	var hold *domain.PointHold
	_, err = p.store.MutatePointAccount(ctx, h.CustomerID, func(a *domain.PointAccount) error {
		out, err := a.Consume(orderID, p.now())
		hold = out
		return err
	})
	return hold, err
}

// Release returns an order's held points to the customer.
func (p *Points) Release(ctx context.Context, orderID string) (*domain.PointHold, error) {
	h, err := p.store.FindPointHold(ctx, orderID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, domain.ErrPointsHoldNotFound
		}
		return nil, err
	}
	var hold *domain.PointHold
	_, err = p.store.MutatePointAccount(ctx, h.CustomerID, func(a *domain.PointAccount) error {
		if err := a.Release(orderID, p.now()); err != nil {
			return err
		}
		hold = a.Hold(orderID)
		return nil
	})
	return hold, err
}

// AdjustInput is a manager's correction. Points > 0 grants, < 0 takes away.
// Ref makes it idempotent (the request's Idempotency-Key, else a fresh id).
type AdjustInput struct {
	CustomerID string
	Points     int64
	Reason     string
	Ref        string
	By         string
}

// Adjust applies a manager correction and returns the new summary.
func (p *Points) Adjust(ctx context.Context, in AdjustInput) (*PointsSummary, error) {
	reason := strings.TrimSpace(in.Reason)
	if strings.TrimSpace(in.CustomerID) == "" || in.Points == 0 || reason == "" || len(reason) > 200 {
		return nil, domain.ErrInvalidPoints
	}
	ref := strings.TrimSpace(in.Ref)
	if ref == "" {
		ref = domain.NewPointID()
	}
	note := reason
	if in.By != "" {
		note = reason + " (" + in.By + ")"
	}
	_, err := p.store.MutatePointAccount(ctx, in.CustomerID, func(a *domain.PointAccount) error {
		if in.Points > 0 {
			_, err := a.Grant(domain.PointSourceAdjust, ref, in.Points, note, p.now())
			return err
		}
		return a.Deduct(ref, -in.Points, note, p.now())
	})
	if err != nil {
		return nil, err
	}
	return p.Summary(ctx, in.CustomerID, 20)
}

// ExpireDue ends every lot past its date and returns how many customers it
// touched.
func (p *Points) ExpireDue(ctx context.Context) (int, error) {
	now := p.now()
	customers, err := p.store.CustomersWithLapsedLots(ctx, now, 200)
	if err != nil {
		return 0, err
	}
	for _, c := range customers {
		if _, err := p.store.MutatePointAccount(ctx, c, func(a *domain.PointAccount) error {
			a.Expire(now)
			return nil
		}); err != nil {
			return 0, err
		}
	}
	return len(customers), nil
}

// StartExpiryWorker runs ExpireDue every interval until ctx ends.
func (p *Points) StartExpiryWorker(ctx context.Context, interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			if _, err := p.ExpireDue(ctx); err != nil && ctx.Err() == nil {
				log.Printf("payment points: expiry sweep: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}
