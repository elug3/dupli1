package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/oklog/ulid/v2"
)

// PromotionEntitlementStore holds who may use which single-user code. Its
// table is created by PromotionStore's migration, which owns the schema.
type PromotionEntitlementStore struct {
	pool *pgxpool.Pool
}

func NewPromotionEntitlementStore(pool *pgxpool.Pool) *PromotionEntitlementStore {
	return &PromotionEntitlementStore{pool: pool}
}

const entitlementColumns = `id, customer_id, code, source, trigger_key, issued_by, ` +
	`expires_at, revoked_at, created_at`

// Issue grants an entitlement, idempotently.
//
// The unique index on (code, customer_id, trigger_key) is what makes a
// redelivered registration event harmless: the insert is a no-op and the
// existing row comes back. Doing this with a read-then-write in Go would let
// two deliveries of the same event both see nothing and both mint a row.
func (s *PromotionEntitlementStore) Issue(ctx context.Context, in ports.IssueEntitlementInput) (*domain.CustomerPromotion, error) {
	code := domain.NormalizedCode(in.Code)
	row := domain.CustomerPromotion{
		ID:         ulid.Make().String(),
		CustomerID: in.CustomerID,
		Code:       code,
		Source:     in.Source,
		TriggerKey: in.TriggerKey,
		IssuedBy:   in.IssuedBy,
		ExpiresAt:  in.ExpiresAt,
		CreatedAt:  time.Now().UTC(),
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO customer_promotions (id, customer_id, code, source, trigger_key, issued_by, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (code, customer_id, trigger_key) DO NOTHING
	`, row.ID, row.CustomerID, row.Code, row.Source, row.TriggerKey, row.IssuedBy, row.ExpiresAt, row.CreatedAt)
	if err != nil {
		return nil, wrapDB("issue entitlement", err)
	}
	// Read back rather than trusting the insert: on conflict the stored row is
	// the earlier one, with its own id and expiry.
	existing, err := s.findByTrigger(ctx, code, in.CustomerID, in.TriggerKey)
	if err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *PromotionEntitlementStore) findByTrigger(ctx context.Context, code, customerID, triggerKey string) (*domain.CustomerPromotion, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+entitlementColumns+` FROM customer_promotions
		WHERE code = $1 AND customer_id = $2 AND trigger_key = $3
	`, code, customerID, triggerKey)
	out, err := scanEntitlement(row)
	if err != nil {
		return nil, wrapDB("issue entitlement", err)
	}
	return out, nil
}

// Find returns the entitlement this customer may actually use, preferring a
// live one. An account can hold more than one row for a code — a manager may
// re-issue after the auto-issued one lapsed — so picking the usable one
// matters.
func (s *PromotionEntitlementStore) Find(ctx context.Context, code, customerID string) (*domain.CustomerPromotion, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+entitlementColumns+` FROM customer_promotions
		WHERE code = $1 AND customer_id = $2
		ORDER BY
			(revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())) DESC,
			created_at DESC
		LIMIT 1
	`, domain.NormalizedCode(code), customerID)
	out, err := scanEntitlement(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("entitlement %s/%s: %w", code, customerID, ports.ErrNotFound)
	}
	if err != nil {
		return nil, wrapDB("find entitlement", err)
	}
	return out, nil
}

func (s *PromotionEntitlementStore) ListForCustomer(ctx context.Context, customerID string) ([]domain.CustomerPromotion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+entitlementColumns+` FROM customer_promotions
		WHERE customer_id = $1 ORDER BY created_at DESC
	`, customerID)
	if err != nil {
		return nil, wrapDB("list entitlements", err)
	}
	defer rows.Close()

	var out []domain.CustomerPromotion
	for rows.Next() {
		e, err := scanEntitlement(rows)
		if err != nil {
			return nil, wrapDB("list entitlements", err)
		}
		out = append(out, *e)
	}
	return out, wrapDB("list entitlements", rows.Err())
}

func (s *PromotionEntitlementStore) Revoke(ctx context.Context, id string, at time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE customer_promotions SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL
	`, id, at.UTC())
	if err != nil {
		return wrapDB("revoke entitlement", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("entitlement %s: %w", id, ports.ErrNotFound)
	}
	return nil
}

func scanEntitlement(row scanner) (*domain.CustomerPromotion, error) {
	var e domain.CustomerPromotion
	if err := row.Scan(
		&e.ID, &e.CustomerID, &e.Code, &e.Source, &e.TriggerKey, &e.IssuedBy,
		&e.ExpiresAt, &e.RevokedAt, &e.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &e, nil
}
