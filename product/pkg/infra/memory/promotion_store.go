package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/oklog/ulid/v2"
)

type PromotionStore struct {
	mu         sync.RWMutex
	promotions map[string]domain.Promotion
}

func NewPromotionStore() *PromotionStore {
	s := &PromotionStore{promotions: make(map[string]domain.Promotion)}
	s.promotions["SUMMER30"] = domain.Promotion{
		Code:        "SUMMER30",
		Scope:       domain.ScopeGlobal,
		Discount:    0.30,
		Description: "Summer sale — all items",
		Expires:     "Aug 31, 2026",
		Active:      true,
		Benefit: domain.Benefit{
			Target:           domain.BenefitTargetGoods,
			DiscountType:     domain.DiscountTypePercent,
			DiscountFraction: 0.30,
			ApplyTo:          domain.ApplyToEntireSubtotal,
		},
		MaxPerCustomer: 1,
	}
	// The sign-up campaign, mirroring the Postgres seed: 50,000원 off orders
	// of 100,000원 or more, one per account, 30-day window. Seeded inactive —
	// a manager enables it when marketing is ready.
	s.promotions[domain.WelcomeCode] = domain.Promotion{
		Code:        domain.WelcomeCode,
		Scope:       domain.ScopeSingleUser,
		Description: "First-purchase discount",
		Terms:       "100,000원 이상 구매 시 50,000원 할인",
		Active:      false,
		Benefit: domain.Benefit{
			Target:           domain.BenefitTargetGoods,
			DiscountType:     domain.DiscountTypeFixed,
			DiscountFixedWon: 50000,
			ApplyTo:          domain.ApplyToEntireSubtotal,
		},
		Conditions: domain.Conditions{
			Version: domain.ConditionsVersion,
			All: []domain.Predicate{
				{Attr: domain.AttrSubtotalWon, Op: domain.OpGte, Value: 100000.0},
			},
		},
		MaxPerCustomer:     1,
		EntitlementTTLDays: 30,
	}
	return s
}

func (s *PromotionStore) List(ctx context.Context) ([]domain.Promotion, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Promotion, 0, len(s.promotions))
	for _, p := range s.promotions {
		out = append(out, p)
	}
	return out, nil
}

func (s *PromotionStore) Create(ctx context.Context, p domain.Promotion) error {
	_ = ctx
	code := domain.NormalizedCode(p.Code)
	if code == "" {
		return ports.Invalid("code is required")
	}
	p.Code = code
	p.Scope = p.EffectiveScope()
	p.MaxPerCustomer = p.EffectiveMaxPerCustomer()
	p.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.promotions[code]; exists {
		return ports.Conflict("promotion already exists")
	}
	s.promotions[code] = p
	return nil
}

func (s *PromotionStore) Update(ctx context.Context, code string, patch ports.PromotionPatch) (*domain.Promotion, error) {
	_ = ctx
	code = domain.NormalizedCode(code)
	s.mu.Lock()
	defer s.mu.Unlock()
	p, exists := s.promotions[code]
	if !exists {
		return nil, fmt.Errorf("promotion %s: %w", code, ports.ErrNotFound)
	}
	applyPromotionPatch(&p, patch)
	p.UpdatedAt = time.Now().UTC()
	s.promotions[code] = p
	return &p, nil
}

func (s *PromotionStore) Delete(ctx context.Context, code string) error {
	_ = ctx
	code = domain.NormalizedCode(code)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.promotions[code]; !exists {
		return fmt.Errorf("promotion %s: %w", code, ports.ErrNotFound)
	}
	delete(s.promotions, code)
	return nil
}

func (s *PromotionStore) Get(ctx context.Context, code string) (*domain.Promotion, error) {
	_ = ctx
	code = domain.NormalizedCode(code)
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.promotions[code]
	if !ok {
		return nil, fmt.Errorf("promotion %s: %w", code, ports.ErrNotFound)
	}
	return &p, nil
}

func (s *PromotionStore) GetActive(ctx context.Context, code string) (*domain.Promotion, bool) {
	p, err := s.Get(ctx, code)
	if err != nil || !p.Active {
		return nil, false
	}
	return p, true
}

// bumpRedemptionCount keeps the denormalised campaign counter in step with the
// ledger, the way the Postgres store does inside its transaction.
func (s *PromotionStore) bumpRedemptionCount(code string, delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.promotions[code]
	if !ok {
		return
	}
	p.RedemptionCount += delta
	if p.RedemptionCount < 0 {
		p.RedemptionCount = 0
	}
	s.promotions[code] = p
}

// tryReserveCampaignSlot increments redemption_count when the campaign cap
// allows. The ledger calls this under its own lock so a reserve and its cap
// check cannot interleave with another checkout completing at the same instant.
func (s *PromotionStore) tryReserveCampaignSlot(code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.promotions[code]
	if !ok {
		return false
	}
	if p.IsExhausted() {
		return false
	}
	p.RedemptionCount++
	s.promotions[code] = p
	return true
}

// applyPromotionPatch mirrors the Postgres store's patch folding.
func applyPromotionPatch(p *domain.Promotion, patch ports.PromotionPatch) {
	if patch.Description != nil {
		p.Description = *patch.Description
	}
	if patch.Active != nil {
		p.Active = *patch.Active
	}
	if patch.Terms != nil {
		p.Terms = *patch.Terms
	}
	if patch.Scope != nil {
		p.Scope = *patch.Scope
	}
	if patch.Conditions != nil {
		p.Conditions = *patch.Conditions
	}
	if patch.Benefit != nil {
		p.Benefit = *patch.Benefit
	}
	if patch.MaxPerCustomer != nil {
		p.MaxPerCustomer = *patch.MaxPerCustomer
	}
	if patch.EntitlementTTLDays != nil {
		p.EntitlementTTLDays = *patch.EntitlementTTLDays
	}
	if patch.ClearExpiresAt {
		p.ExpiresAt = nil
	} else if patch.ExpiresAt != nil {
		p.ExpiresAt = patch.ExpiresAt
	}
	if patch.ClearMaxRedemptions {
		p.MaxRedemptions = nil
	} else if patch.MaxRedemptions != nil {
		p.MaxRedemptions = patch.MaxRedemptions
	}
	if patch.Discount != nil {
		p.Discount = *patch.Discount
	}
	if patch.Expires != nil {
		p.Expires = *patch.Expires
	}
}

// PromotionRedemptionStore is the in-memory usage ledger. Order, cart and
// payment fall back to in-memory repositories when no DB URL is set and the
// tests rely on that, so this has to enforce the same once-per-customer rule
// as Postgres rather than being a stub.
type PromotionRedemptionStore struct {
	mu          sync.Mutex
	byOrder     map[string]*domain.Redemption
	definitions *PromotionStore
}

func NewPromotionRedemptionStore(definitions *PromotionStore) *PromotionRedemptionStore {
	return &PromotionRedemptionStore{
		byOrder:     make(map[string]*domain.Redemption),
		definitions: definitions,
	}
}

func (s *PromotionRedemptionStore) Reserve(ctx context.Context, in ports.ReserveRedemptionInput, maxPerCustomer int) (*domain.Redemption, error) {
	_ = ctx
	if maxPerCustomer <= 0 {
		maxPerCustomer = 1
	}
	code := domain.NormalizedCode(in.Code)

	s.mu.Lock()
	defer s.mu.Unlock()

	// A retried complete for the same order must not double-count — unless a
	// pre-payment cancel released the row and complete is being retried.
	if existing, ok := s.byOrder[in.OrderID]; ok {
		if existing.Status != domain.RedemptionReleased {
			return existing, nil
		}
		if s.definitions != nil && !s.definitions.tryReserveCampaignSlot(code) {
			return nil, ports.Conflict("promotion campaign exhausted")
		}
		used := 0
		for _, r := range s.byOrder {
			if r.Code == code && r.CustomerID == in.CustomerID && r.CountsAgainstCustomer() {
				used++
			}
		}
		if used >= maxPerCustomer {
			return nil, ports.Conflict("promotion already used by this customer")
		}
		existing.Status = domain.RedemptionReserved
		existing.DiscountWon = in.DiscountWon
		existing.ShippingDiscountWon = in.ShippingDiscountWon
		existing.OrderSubtotalWon = in.OrderSubtotalWon
		existing.EligibleSubtotalWon = in.EligibleSubtotalWon
		existing.AppliedBenefit = in.AppliedBenefit
		existing.ReleasedAt = nil
		if s.definitions != nil {
			s.definitions.bumpRedemptionCount(code, 1)
		}
		return existing, nil
	}

	used := 0
	for _, r := range s.byOrder {
		if r.Code == code && r.CustomerID == in.CustomerID && r.CountsAgainstCustomer() {
			used++
		}
	}
	if used >= maxPerCustomer {
		return nil, ports.Conflict("promotion already used by this customer")
	}

	if s.definitions != nil && !s.definitions.tryReserveCampaignSlot(code) {
		return nil, ports.Conflict("promotion campaign exhausted")
	}

	row := &domain.Redemption{
		ID:                  ulid.Make().String(),
		Code:                code,
		OrderID:             in.OrderID,
		CustomerID:          in.CustomerID,
		Status:              domain.RedemptionReserved,
		DiscountWon:         in.DiscountWon,
		ShippingDiscountWon: in.ShippingDiscountWon,
		OrderSubtotalWon:    in.OrderSubtotalWon,
		EligibleSubtotalWon: in.EligibleSubtotalWon,
		AppliedBenefit:      in.AppliedBenefit,
		CreatedAt:           time.Now().UTC(),
	}
	s.byOrder[in.OrderID] = row
	return row, nil
}

func (s *PromotionRedemptionStore) Consume(ctx context.Context, orderID string, at time.Time) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.byOrder[orderID]
	if !ok || row.Status == domain.RedemptionConsumed {
		return nil // idempotent: payment events can be redelivered
	}
	paid := at.UTC()
	switch row.Status {
	case domain.RedemptionReserved:
		row.Status = domain.RedemptionConsumed
		row.PaidAt = &paid
	case domain.RedemptionReleased:
		row.Status = domain.RedemptionConsumed
		row.PaidAt = &paid
		row.ReleasedAt = nil
		if s.definitions != nil {
			s.definitions.bumpRedemptionCount(row.Code, 1)
		}
	}
	return nil
}

func (s *PromotionRedemptionStore) Release(ctx context.Context, orderID string, at time.Time) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.byOrder[orderID]
	if !ok || !row.CountsAgainstCustomer() {
		return nil
	}
	released := at.UTC()
	row.Status = domain.RedemptionReleased
	row.ReleasedAt = &released
	// bumpRedemptionCount takes the definition store's own mutex, not this
	// one, so it is safe to call while holding the ledger lock.
	if s.definitions != nil {
		s.definitions.bumpRedemptionCount(row.Code, -1)
	}
	return nil
}

func (s *PromotionRedemptionStore) ActiveCountForCustomer(ctx context.Context, code, customerID string) (int, error) {
	_ = ctx
	code = domain.NormalizedCode(code)
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.byOrder {
		if r.Code == code && r.CustomerID == customerID && r.CountsAgainstCustomer() {
			n++
		}
	}
	return n, nil
}

func (s *PromotionRedemptionStore) CountsByCode(ctx context.Context, code string) (int, int, error) {
	_ = ctx
	code = domain.NormalizedCode(code)
	s.mu.Lock()
	defer s.mu.Unlock()
	active, consumed := 0, 0
	for _, r := range s.byOrder {
		if r.Code != code {
			continue
		}
		if r.CountsAgainstCustomer() {
			active++
		}
		if r.Status == domain.RedemptionConsumed {
			consumed++
		}
	}
	return active, consumed, nil
}
