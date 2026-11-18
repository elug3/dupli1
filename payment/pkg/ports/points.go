package ports

import (
	"context"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
)

// PointsStore keeps the points ledger (docs/payment-points.md).
//
// MutatePointAccount is the only write: it loads one customer's whole account
// under that customer's lock, runs mutate, and when mutate returns nil saves
// every lot and hold and appends the entries the mutation added. An error from
// mutate writes nothing. Accounts are small (a lot per order, review or grant),
// so loading one whole is cheaper than being clever.
type PointsStore interface {
	MutatePointAccount(ctx context.Context, customerID string, mutate func(*domain.PointAccount) error) (*domain.PointAccount, error)
	// GetPointAccount loads an account read-only. A customer with no points
	// yet has an empty account, not an error.
	GetPointAccount(ctx context.Context, customerID string) (*domain.PointAccount, error)
	// FindPointHold finds an order's hold, so consume and release can be
	// called with the order id alone. ErrNotFound when there is none.
	FindPointHold(ctx context.Context, orderID string) (*domain.PointHold, error)
	// ListPointEntries returns a customer's history, newest first (created_at,
	// then id, descending). after, when non-zero, pages: only entries that sort
	// after it in that order.
	ListPointEntries(ctx context.Context, customerID string, after PointCursor, limit int) ([]domain.PointEntry, error)
	// CustomersWithLapsedLots lists customers holding an available lot whose
	// expiry is at or before at, for the expiry sweep.
	CustomersWithLapsedLots(ctx context.Context, at time.Time, limit int) ([]string, error)
}

// PointCursor is the position of the last entry a page returned.
type PointCursor struct {
	At time.Time
	ID string
}

// IsZero reports whether the cursor is the start of the history.
func (c PointCursor) IsZero() bool { return c.At.IsZero() && c.ID == "" }
