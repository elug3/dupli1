package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/ports"
)

// Points is the in-memory points ledger, used when no database is configured
// and by tests.
type Points struct {
	mu      sync.Mutex
	lots    map[string][]*domain.PointLot
	holds   map[string][]*domain.PointHold
	entries []domain.PointEntry
}

func NewPoints() *Points {
	return &Points{lots: map[string][]*domain.PointLot{}, holds: map[string][]*domain.PointHold{}}
}

func (p *Points) load(customerID string) *domain.PointAccount {
	var lots []*domain.PointLot
	for _, l := range p.lots[customerID] {
		c := *l
		lots = append(lots, &c)
	}
	var holds []*domain.PointHold
	for _, h := range p.holds[customerID] {
		c := *h
		c.Allocations = append([]domain.PointAllocation(nil), h.Allocations...)
		holds = append(holds, &c)
	}
	return domain.NewPointAccount(customerID, lots, holds, domain.NewPointID)
}

func (p *Points) MutatePointAccount(ctx context.Context, customerID string, mutate func(*domain.PointAccount) error) (*domain.PointAccount, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	acct := p.load(customerID)
	if err := mutate(acct); err != nil {
		return nil, err
	}
	p.lots[customerID] = acct.Lots
	p.holds[customerID] = acct.Holds
	p.entries = append(p.entries, acct.NewEntries...)
	// Hand back a copy so the caller cannot reach into the store.
	out := p.load(customerID)
	out.NewEntries = acct.NewEntries
	acct.NewEntries = nil
	return out, nil
}

func (p *Points) GetPointAccount(ctx context.Context, customerID string) (*domain.PointAccount, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.load(customerID), nil
}

func (p *Points) FindPointHold(ctx context.Context, orderID string) (*domain.PointHold, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, hs := range p.holds {
		for _, h := range hs {
			if h.OrderID == orderID {
				c := *h
				return &c, nil
			}
		}
	}
	return nil, ports.ErrNotFound
}

func (p *Points) ListPointEntries(ctx context.Context, customerID string, after ports.PointCursor, limit int) ([]domain.PointEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []domain.PointEntry
	for _, e := range p.entries {
		if e.CustomerID != customerID {
			continue
		}
		if !after.IsZero() && !entryBefore(e, after) {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return entryBefore(out[j], ports.PointCursor{At: out[i].CreatedAt, ID: out[i].ID})
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (p *Points) CustomersWithLapsedLots(ctx context.Context, at time.Time, limit int) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for customer, lots := range p.lots {
		for _, l := range lots {
			if l.Status == domain.LotAvailable && l.ExpiresAt != nil && !l.ExpiresAt.After(at) {
				out = append(out, customer)
				break
			}
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// entryBefore reports whether e sorts after the cursor in newest-first order.
func entryBefore(e domain.PointEntry, c ports.PointCursor) bool {
	if !e.CreatedAt.Equal(c.At) {
		return e.CreatedAt.Before(c.At)
	}
	return e.ID < c.ID
}
