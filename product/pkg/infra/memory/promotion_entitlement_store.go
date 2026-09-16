package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/oklog/ulid/v2"
)

// PromotionEntitlementStore holds who may use which single-user code.
// It enforces the same idempotency as Postgres rather than being a stub,
// because the tests run against it.
type PromotionEntitlementStore struct {
	mu   sync.Mutex
	rows map[string]*domain.CustomerPromotion // id -> row
}

func NewPromotionEntitlementStore() *PromotionEntitlementStore {
	return &PromotionEntitlementStore{rows: make(map[string]*domain.CustomerPromotion)}
}

func triggerKeyOf(code, customerID, triggerKey string) string {
	return code + "\x00" + customerID + "\x00" + triggerKey
}

func (s *PromotionEntitlementStore) Issue(ctx context.Context, in ports.IssueEntitlementInput) (*domain.CustomerPromotion, error) {
	_ = ctx
	code := domain.NormalizedCode(in.Code)
	key := triggerKeyOf(code, in.CustomerID, in.TriggerKey)

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range s.rows {
		if triggerKeyOf(row.Code, row.CustomerID, row.TriggerKey) == key {
			// A redelivered event mints nothing and returns what exists.
			return row, nil
		}
	}
	row := &domain.CustomerPromotion{
		ID:         ulid.Make().String(),
		CustomerID: in.CustomerID,
		Code:       code,
		Source:     in.Source,
		TriggerKey: in.TriggerKey,
		IssuedBy:   in.IssuedBy,
		ExpiresAt:  in.ExpiresAt,
		CreatedAt:  time.Now().UTC(),
	}
	s.rows[row.ID] = row
	return row, nil
}

func (s *PromotionEntitlementStore) Find(ctx context.Context, code, customerID string) (*domain.CustomerPromotion, error) {
	_ = ctx
	code = domain.NormalizedCode(code)
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	var best *domain.CustomerPromotion
	for _, row := range s.rows {
		if row.Code != code || row.CustomerID != customerID {
			continue
		}
		// Prefer a usable row: an account may hold a lapsed one plus a
		// manager re-issue.
		if best == nil ||
			(row.Usable(now) && !best.Usable(now)) ||
			(row.Usable(now) == best.Usable(now) && row.CreatedAt.After(best.CreatedAt)) {
			best = row
		}
	}
	if best == nil {
		return nil, fmt.Errorf("entitlement %s/%s: %w", code, customerID, ports.ErrNotFound)
	}
	return best, nil
}

func (s *PromotionEntitlementStore) ListForCustomer(ctx context.Context, customerID string) ([]domain.CustomerPromotion, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.CustomerPromotion
	for _, row := range s.rows {
		if row.CustomerID == customerID {
			out = append(out, *row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *PromotionEntitlementStore) Revoke(ctx context.Context, id string, at time.Time) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[id]
	if !ok || row.RevokedAt != nil {
		return fmt.Errorf("entitlement %s: %w", id, ports.ErrNotFound)
	}
	revoked := at.UTC()
	row.RevokedAt = &revoked
	return nil
}
