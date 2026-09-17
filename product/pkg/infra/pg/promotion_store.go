package pg

import (
	"context"
	"fmt"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/jackc/pgx/v4/pgxpool"
)

type PromotionStore struct {
	pool *pgxpool.Pool
}

func NewPromotionStore(pool *pgxpool.Pool) (*PromotionStore, error) {
	store := &PromotionStore{pool: pool}
	if err := store.migrate(); err != nil {
		return nil, err
	}
	if err := store.seedDefaults(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *PromotionStore) migrate() error {
	// Startup schema migration runs outside any HTTP request; there is no
	// request-scoped context to propagate (process lifetime only).
	if err := s.renameCouponsTableIfNeeded(); err != nil {
		return err
	}
	return s.migrateFreshSchema()
}

func (s *PromotionStore) migrateFreshSchema() error {
	_, err := s.pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS promotions (
			code        TEXT PRIMARY KEY,
			discount    DOUBLE PRECISION NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			expires     TEXT NOT NULL DEFAULT '',
			active      BOOLEAN NOT NULL DEFAULT TRUE
		)
	`)
	if err != nil {
		return fmt.Errorf("migrate promotions: %w", err)
	}

	// Phase 2 columns are added one at a time with IF NOT EXISTS, the additive
	// pattern this repo uses in place of a migration tool. Existing rows keep
	// their discount/expires values; the backfills below give them a scope and
	// a benefit document.
	for _, stmt := range []string{
		`ALTER TABLE promotions ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'global'`,
		`ALTER TABLE promotions ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ`,
		`ALTER TABLE promotions ADD COLUMN IF NOT EXISTS max_redemptions INTEGER`,
		`ALTER TABLE promotions ADD COLUMN IF NOT EXISTS max_per_customer INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE promotions ADD COLUMN IF NOT EXISTS conditions JSONB NOT NULL DEFAULT '{}'::jsonb`,
		`ALTER TABLE promotions ADD COLUMN IF NOT EXISTS benefit JSONB NOT NULL DEFAULT '{}'::jsonb`,
		`ALTER TABLE promotions ADD COLUMN IF NOT EXISTS terms TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE promotions ADD COLUMN IF NOT EXISTS redemption_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE promotions ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`,
		`ALTER TABLE promotions ADD COLUMN IF NOT EXISTS entitlement_ttl_days INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := s.pool.Exec(context.Background(), stmt); err != nil {
			return fmt.Errorf("migrate promotions columns: %w", err)
		}
	}

	if err := s.migrateRedemptions(); err != nil {
		return err
	}
	if err := s.migrateEntitlements(); err != nil {
		return err
	}
	return s.backfillLegacyBenefit()
}

// migrateRedemptions creates the usage ledger.
//
// The partial unique index is the once-per-customer guarantee, and it lives in
// the database rather than in a read-then-write in Go: two checkouts
// completing at the same moment would both pass an application-level check.
// Released rows are excluded so a cancelled order hands the use back.
func (s *PromotionStore) migrateRedemptions() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS promotion_redemptions (
			id                    TEXT PRIMARY KEY,
			code                  TEXT NOT NULL,
			order_id              TEXT NOT NULL,
			customer_id           TEXT NOT NULL,
			status                TEXT NOT NULL,
			discount_won          BIGINT NOT NULL DEFAULT 0,
			shipping_discount_won BIGINT NOT NULL DEFAULT 0,
			order_subtotal_won    BIGINT NOT NULL DEFAULT 0,
			eligible_subtotal_won BIGINT NOT NULL DEFAULT 0,
			applied_benefit       JSONB,
			created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
			paid_at               TIMESTAMPTZ,
			released_at           TIMESTAMPTZ
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS promotion_redemptions_order_uniq
			ON promotion_redemptions (order_id)`,
		`CREATE INDEX IF NOT EXISTS promotion_redemptions_code_idx
			ON promotion_redemptions (code, status)`,
		`CREATE INDEX IF NOT EXISTS promotion_redemptions_customer_idx
			ON promotion_redemptions (code, customer_id, status)`,
	} {
		if _, err := s.pool.Exec(context.Background(), stmt); err != nil {
			return fmt.Errorf("migrate promotion_redemptions: %w", err)
		}
	}
	return nil
}

// migrateEntitlements creates the single-user access table.
//
// The unique index on (code, customer_id, trigger_key) is the idempotency
// guarantee for auto-issue: a redelivered registration event conflicts instead
// of minting a second entitlement.
func (s *PromotionStore) migrateEntitlements() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS customer_promotions (
			id          TEXT PRIMARY KEY,
			customer_id TEXT NOT NULL,
			code        TEXT NOT NULL,
			source      TEXT NOT NULL DEFAULT '',
			trigger_key TEXT NOT NULL DEFAULT '',
			issued_by   TEXT NOT NULL DEFAULT '',
			expires_at  TIMESTAMPTZ,
			revoked_at  TIMESTAMPTZ,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS customer_promotions_trigger_uniq
			ON customer_promotions (code, customer_id, trigger_key)`,
		`CREATE INDEX IF NOT EXISTS customer_promotions_customer_idx
			ON customer_promotions (customer_id)`,
	} {
		if _, err := s.pool.Exec(context.Background(), stmt); err != nil {
			return fmt.Errorf("migrate customer_promotions: %w", err)
		}
	}
	return nil
}

// backfillLegacyBenefit gives pre-Phase-2 rows a benefit document built from
// the discount column they already carry, so they price identically through
// the new engine instead of relying on the read-time fallback forever.
func (s *PromotionStore) backfillLegacyBenefit() error {
	_, err := s.pool.Exec(context.Background(), `
		UPDATE promotions
		SET benefit = jsonb_build_object(
			'target', 'goods',
			'discount_type', 'percent',
			'discount_fraction', discount,
			'apply_to', 'entire_subtotal'
		)
		WHERE benefit = '{}'::jsonb AND discount > 0 AND discount < 1
	`)
	if err != nil {
		return fmt.Errorf("backfill promotion benefit: %w", err)
	}
	return nil
}

// renameCouponsTableIfNeeded moves the pre-2026-09 coupons table to its new
// name, keeping the rows rather than stranding them behind an empty
// promotions table (docs/product-promotion-rename.md).
//
// Guarded the same way order's renameColumnIfNeeded is: it acts only when the
// old table is present and the new one is not, so a fresh database skips it
// and a redeploy — or a rollback that recreated promotions — leaves the live
// table untouched.
func (s *PromotionStore) renameCouponsTableIfNeeded() error {
	var hasCoupons, hasPromotions bool
	err := s.pool.QueryRow(context.Background(), `
		SELECT to_regclass('public.coupons') IS NOT NULL,
		       to_regclass('public.promotions') IS NOT NULL
	`).Scan(&hasCoupons, &hasPromotions)
	if err != nil {
		return fmt.Errorf("inspect promotions table: %w", err)
	}
	if !hasCoupons || hasPromotions {
		return nil
	}
	if _, err := s.pool.Exec(context.Background(), `ALTER TABLE coupons RENAME TO promotions`); err != nil {
		return fmt.Errorf("rename coupons to promotions: %w", err)
	}
	return nil
}

func (s *PromotionStore) seedDefaults() error {
	// Bootstrap seed data at process start; no request context available.
	if _, err := s.pool.Exec(context.Background(), `
		INSERT INTO promotions (code, discount, description, expires, active)
		VALUES ('SUMMER30', 0.30, 'Summer sale — all items', 'Aug 31, 2026', TRUE)
		ON CONFLICT (code) DO NOTHING
	`); err != nil {
		return err
	}

	// The sign-up campaign's definition, seeded so the registration issuer and
	// the backfill have something to issue against in every environment.
	//
	// It is seeded INACTIVE on purpose. Creating a live 50,000원 discount on
	// every environment the moment this deploys is not a decision a migration
	// should make — a manager enables it when marketing is ready. Entitlements
	// issued while it is inactive are not wasted: flipping active makes every
	// one of them work, and each keeps the expiry it was issued with.
	//
	// ON CONFLICT DO NOTHING, so enabling it (or editing it) is never undone
	// by the next deploy.
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO promotions (
			code, scope, discount, description, expires, active,
			conditions, benefit, max_per_customer, entitlement_ttl_days, terms
		)
		VALUES (
			'WELCOME50', 'single_user', 0, 'First-purchase discount', '', FALSE,
			$1::jsonb, $2::jsonb, 1, 30, '100,000원 이상 구매 시 50,000원 할인'
		)
		ON CONFLICT (code) DO NOTHING
	`, welcome50Conditions, welcome50Benefit)
	return err
}

// The sign-up campaign's rules, written once here so the seed and the docs
// cannot disagree: 50,000원 off goods, on orders of 100,000원 or more.
const (
	welcome50Conditions = `{"version":1,"all":[{"attr":"subtotal_won","op":"gte","value":100000}]}`
	welcome50Benefit    = `{"target":"goods","discount_type":"fixed","discount_fixed_won":50000,"apply_to":"entire_subtotal"}`
)

func (s *PromotionStore) List(ctx context.Context) ([]domain.Promotion, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+promotionColumns+` FROM promotions ORDER BY code`)
	if err != nil {
		return nil, wrapDB("list promotions", err)
	}
	defer rows.Close()

	var promotions []domain.Promotion
	for rows.Next() {
		p, err := scanPromotion(rows)
		if err != nil {
			return nil, wrapDB("list promotions", err)
		}
		promotions = append(promotions, *p)
	}
	return promotions, wrapDB("list promotions", rows.Err())
}

func (s *PromotionStore) Create(ctx context.Context, c domain.Promotion) error {
	code := domain.NormalizedCode(c.Code)
	if code == "" {
		return ports.Invalid("code is required")
	}
	c.Code = code
	conditions, benefit, err := marshalPromotionDocs(c)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO promotions (code, scope, discount, description, expires, active,
			conditions, benefit, expires_at, max_redemptions, max_per_customer, terms,
			entitlement_ttl_days, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now())
	`, c.Code, string(c.EffectiveScope()), c.Discount, c.Description, c.Expires, c.Active,
		conditions, benefit, c.ExpiresAt, c.MaxRedemptions, c.EffectiveMaxPerCustomer(), c.Terms,
		c.EntitlementTTLDays)
	if err != nil {
		if isUniqueViolation(err) {
			return ports.Conflict("promotion already exists")
		}
		return wrapDB("create promotion", err)
	}
	return nil
}

func (s *PromotionStore) Update(ctx context.Context, code string, patch ports.PromotionPatch) (*domain.Promotion, error) {
	code = domain.NormalizedCode(code)
	current, err := s.getPromotion(ctx, code)
	if err != nil {
		return nil, err
	}
	applyPromotionPatch(current, patch)

	conditions, benefit, err := marshalPromotionDocs(*current)
	if err != nil {
		return nil, err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE promotions SET scope = $2, discount = $3, description = $4, expires = $5,
			active = $6, conditions = $7, benefit = $8, expires_at = $9, max_redemptions = $10,
			max_per_customer = $11, terms = $12, entitlement_ttl_days = $13, updated_at = now()
		WHERE code = $1
	`, current.Code, string(current.EffectiveScope()), current.Discount, current.Description,
		current.Expires, current.Active, conditions, benefit, current.ExpiresAt,
		current.MaxRedemptions, current.EffectiveMaxPerCustomer(), current.Terms,
		current.EntitlementTTLDays)
	if err != nil {
		return nil, wrapDB("update promotion", err)
	}
	return current, nil
}

// Get returns a definition whatever its state, so a caller can tell an expired
// or exhausted code from one that does not exist.
func (s *PromotionStore) Get(ctx context.Context, code string) (*domain.Promotion, error) {
	return s.getPromotion(ctx, domain.NormalizedCode(code))
}

func (s *PromotionStore) Delete(ctx context.Context, code string) error {
	code = domain.NormalizedCode(code)
	tag, err := s.pool.Exec(ctx, `DELETE FROM promotions WHERE code = $1`, code)
	if err != nil {
		return wrapDB("delete promotion", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("promotion %s: %w", code, ports.ErrNotFound)
	}
	return nil
}

func (s *PromotionStore) GetActive(ctx context.Context, code string) (*domain.Promotion, bool) {
	promotion, err := s.getPromotion(ctx, domain.NormalizedCode(code))
	if err != nil || !promotion.Active {
		return nil, false
	}
	return promotion, true
}

func (s *PromotionStore) getPromotion(ctx context.Context, code string) (*domain.Promotion, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+promotionColumns+` FROM promotions WHERE code = $1`, code)
	p, err := scanPromotion(row)
	if err != nil {
		return nil, wrapDB("get promotion", err)
	}
	return p, nil
}
