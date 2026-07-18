package pg

import (
	"context"
	"fmt"
	"strings"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/jackc/pgx/v4/pgxpool"
)

type CouponStore struct {
	pool *pgxpool.Pool
}

func NewCouponStore(pool *pgxpool.Pool) (*CouponStore, error) {
	store := &CouponStore{pool: pool}
	if err := store.migrate(); err != nil {
		return nil, err
	}
	if err := store.seedDefaults(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *CouponStore) migrate() error {
	_, err := s.pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS coupons (
			code        TEXT PRIMARY KEY,
			discount    DOUBLE PRECISION NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			expires     TEXT NOT NULL DEFAULT '',
			active      BOOLEAN NOT NULL DEFAULT TRUE
		)
	`)
	if err != nil {
		return fmt.Errorf("migrate coupons: %w", err)
	}
	return nil
}

func (s *CouponStore) seedDefaults() error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO coupons (code, discount, description, expires, active)
		VALUES ('SUMMER30', 0.30, 'Summer sale — all items', 'Aug 31, 2026', TRUE)
		ON CONFLICT (code) DO NOTHING
	`)
	return err
}

func (s *CouponStore) List() ([]domain.Coupon, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT code, discount, description, expires, active FROM coupons ORDER BY code
	`)
	if err != nil {
		return nil, wrapDB("list coupons", err)
	}
	defer rows.Close()

	var coupons []domain.Coupon
	for rows.Next() {
		var c domain.Coupon
		if err := rows.Scan(&c.Code, &c.Discount, &c.Description, &c.Expires, &c.Active); err != nil {
			return nil, wrapDB("list coupons", err)
		}
		coupons = append(coupons, c)
	}
	return coupons, wrapDB("list coupons", rows.Err())
}

func (s *CouponStore) Create(c domain.Coupon) error {
	code := strings.ToUpper(strings.TrimSpace(c.Code))
	if code == "" {
		return ports.Invalid("code is required")
	}
	c.Code = code
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO coupons (code, discount, description, expires, active)
		VALUES ($1, $2, $3, $4, $5)
	`, c.Code, c.Discount, c.Description, c.Expires, c.Active)
	if err != nil {
		if isUniqueViolation(err) {
			return ports.Conflict("coupon already exists")
		}
		return wrapDB("create coupon", err)
	}
	return nil
}

func (s *CouponStore) Update(code string, discount *float64, description, expires *string, active *bool) (*domain.Coupon, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	current, err := s.getCoupon(code)
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
	_, err = s.pool.Exec(context.Background(), `
		UPDATE coupons SET discount = $2, description = $3, expires = $4, active = $5 WHERE code = $1
	`, current.Code, current.Discount, current.Description, current.Expires, current.Active)
	if err != nil {
		return nil, wrapDB("update coupon", err)
	}
	return current, nil
}

func (s *CouponStore) Delete(code string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM coupons WHERE code = $1`, code)
	if err != nil {
		return wrapDB("delete coupon", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("coupon %s: %w", code, ports.ErrNotFound)
	}
	return nil
}

func (s *CouponStore) GetActive(code string) (*domain.Coupon, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	coupon, err := s.getCoupon(code)
	if err != nil || !coupon.Active {
		return nil, false
	}
	return coupon, true
}

func (s *CouponStore) getCoupon(code string) (*domain.Coupon, error) {
	var c domain.Coupon
	err := s.pool.QueryRow(context.Background(), `
		SELECT code, discount, description, expires, active FROM coupons WHERE code = $1
	`, code).Scan(&c.Code, &c.Discount, &c.Description, &c.Expires, &c.Active)
	if err != nil {
		return nil, wrapDB("get coupon", err)
	}
	return &c, nil
}
