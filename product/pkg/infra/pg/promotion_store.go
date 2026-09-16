package pg

import (
	"context"
	"fmt"
	"strings"

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
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO promotions (code, discount, description, expires, active)
		VALUES ('SUMMER30', 0.30, 'Summer sale — all items', 'Aug 31, 2026', TRUE)
		ON CONFLICT (code) DO NOTHING
	`)
	return err
}

func (s *PromotionStore) List(ctx context.Context) ([]domain.Promotion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT code, discount, description, expires, active FROM promotions ORDER BY code
	`)
	if err != nil {
		return nil, wrapDB("list promotions", err)
	}
	defer rows.Close()

	var promotions []domain.Promotion
	for rows.Next() {
		var c domain.Promotion
		if err := rows.Scan(&c.Code, &c.Discount, &c.Description, &c.Expires, &c.Active); err != nil {
			return nil, wrapDB("list promotions", err)
		}
		promotions = append(promotions, c)
	}
	return promotions, wrapDB("list promotions", rows.Err())
}

func (s *PromotionStore) Create(ctx context.Context, c domain.Promotion) error {
	code := strings.ToUpper(strings.TrimSpace(c.Code))
	if code == "" {
		return ports.Invalid("code is required")
	}
	c.Code = code
	_, err := s.pool.Exec(ctx, `
		INSERT INTO promotions (code, discount, description, expires, active)
		VALUES ($1, $2, $3, $4, $5)
	`, c.Code, c.Discount, c.Description, c.Expires, c.Active)
	if err != nil {
		if isUniqueViolation(err) {
			return ports.Conflict("promotion already exists")
		}
		return wrapDB("create promotion", err)
	}
	return nil
}

func (s *PromotionStore) Update(ctx context.Context, code string, discount *float64, description, expires *string, active *bool) (*domain.Promotion, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	current, err := s.getPromotion(ctx, code)
	if err != nil {
		return nil, err
	}
	if discount != nil {
		current.Discount = *discount
	}
	if description != nil {
		current.Description = *description
	}
	if expires != nil {
		current.Expires = *expires
	}
	if active != nil {
		current.Active = *active
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE promotions SET discount = $2, description = $3, expires = $4, active = $5 WHERE code = $1
	`, current.Code, current.Discount, current.Description, current.Expires, current.Active)
	if err != nil {
		return nil, wrapDB("update promotion", err)
	}
	return current, nil
}

func (s *PromotionStore) Delete(ctx context.Context, code string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
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
	code = strings.ToUpper(strings.TrimSpace(code))
	promotion, err := s.getPromotion(ctx, code)
	if err != nil || !promotion.Active {
		return nil, false
	}
	return promotion, true
}

func (s *PromotionStore) getPromotion(ctx context.Context, code string) (*domain.Promotion, error) {
	var c domain.Promotion
	err := s.pool.QueryRow(ctx, `
		SELECT code, discount, description, expires, active FROM promotions WHERE code = $1
	`, code).Scan(&c.Code, &c.Discount, &c.Description, &c.Expires, &c.Active)
	if err != nil {
		return nil, wrapDB("get promotion", err)
	}
	return &c, nil
}
