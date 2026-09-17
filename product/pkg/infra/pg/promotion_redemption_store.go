package pg

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/oklog/ulid/v2"
)

// PromotionRedemptionStore is the usage ledger that makes a promotional code
// finite. It shares the pool with PromotionStore; the tables are created by
// that store's migration.
type PromotionRedemptionStore struct {
	pool *pgxpool.Pool
}

func NewPromotionRedemptionStore(pool *pgxpool.Pool) *PromotionRedemptionStore {
	return &PromotionRedemptionStore{pool: pool}
}

// Reserve records a pending use, refusing one the customer is not entitled to.
//
// The limit check and the insert run in one transaction with the customer's
// existing rows locked, because a read-then-write in application code loses
// the race between two checkouts completing at the same instant — which is
// exactly the race a determined customer would try.
func (s *PromotionRedemptionStore) Reserve(ctx context.Context, in ports.ReserveRedemptionInput, maxPerCustomer int) (*domain.Redemption, error) {
	if maxPerCustomer <= 0 {
		maxPerCustomer = 1
	}
	code := domain.NormalizedCode(in.Code)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, wrapDB("reserve redemption", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// An order may hold only one redemption. If this order already has one,
	// hand it back rather than double-counting a retried complete — unless a
	// pre-payment cancel released it and complete is being retried.
	existing, err := scanRedemption(tx.QueryRow(ctx, `
		SELECT `+redemptionColumns+` FROM promotion_redemptions WHERE order_id = $1
	`, in.OrderID))
	if err == nil {
		if existing.Status != domain.RedemptionReleased {
			return existing, tx.Commit(ctx)
		}
		if err := s.reactivateReleased(ctx, tx, existing, in, code, maxPerCustomer); err != nil {
			return nil, err
		}
		return existing, tx.Commit(ctx)
	}
	if err != pgx.ErrNoRows {
		return nil, wrapDB("reserve redemption", err)
	}

	var redemptionCount int
	var maxRedemptions *int
	if err := tx.QueryRow(ctx, `
		SELECT redemption_count, max_redemptions FROM promotions WHERE code = $1 FOR UPDATE
	`, code).Scan(&redemptionCount, &maxRedemptions); err != nil {
		return nil, wrapDB("reserve redemption", err)
	}
	if maxRedemptions != nil && redemptionCount >= *maxRedemptions {
		return nil, ports.Conflict("promotion campaign exhausted")
	}

	var used int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM promotion_redemptions
		WHERE code = $1 AND customer_id = $2 AND status IN ('reserved', 'consumed')
		FOR UPDATE
	`, code, in.CustomerID).Scan(&used); err != nil {
		return nil, wrapDB("reserve redemption", err)
	}
	if used >= maxPerCustomer {
		return nil, ports.Conflict("promotion already used by this customer")
	}

	benefit, err := json.Marshal(in.AppliedBenefit)
	if err != nil {
		return nil, fmt.Errorf("encode applied benefit: %w", err)
	}

	row := domain.Redemption{
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
	if _, err := tx.Exec(ctx, `
		INSERT INTO promotion_redemptions (id, code, order_id, customer_id, status,
			discount_won, shipping_discount_won, order_subtotal_won, eligible_subtotal_won,
			applied_benefit, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, row.ID, row.Code, row.OrderID, row.CustomerID, string(row.Status),
		row.DiscountWon, row.ShippingDiscountWon, row.OrderSubtotalWon, row.EligibleSubtotalWon,
		benefit, row.CreatedAt); err != nil {
		if isUniqueViolation(err) {
			return nil, ports.Conflict("promotion already reserved for this order")
		}
		return nil, wrapDB("reserve redemption", err)
	}

	// The denormalised count on the definition drives the campaign cap and the
	// list view; it is kept in the same transaction so it cannot drift.
	if _, err := tx.Exec(ctx, `
		UPDATE promotions SET redemption_count = redemption_count + 1 WHERE code = $1
	`, code); err != nil {
		return nil, wrapDB("reserve redemption", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, wrapDB("reserve redemption", err)
	}
	return &row, nil
}

// Consume marks an order's reservation paid. It is idempotent: payment events
// can be redelivered, and a second call on an already-consumed row is a no-op.
//
// A row in released state is also consumable: cancel-before-pay hands the slot
// back, but a late payment.succeeded still lands on the same order_id and must
// spend the use again (mirroring stock reinstate on MarkOrderPaid).
func (s *PromotionRedemptionStore) Consume(ctx context.Context, orderID string, at time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return wrapDB("consume redemption", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var code, status string
	err = tx.QueryRow(ctx, `
		SELECT code, status FROM promotion_redemptions WHERE order_id = $1 FOR UPDATE
	`, orderID).Scan(&code, &status)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return wrapDB("consume redemption", err)
	}
	if status == string(domain.RedemptionConsumed) {
		return wrapDB("consume redemption", tx.Commit(ctx))
	}
	if status == string(domain.RedemptionReserved) {
		if _, err := tx.Exec(ctx, `
			UPDATE promotion_redemptions
			SET status = 'consumed', paid_at = $2
			WHERE order_id = $1 AND status = 'reserved'
		`, orderID, at.UTC()); err != nil {
			return wrapDB("consume redemption", err)
		}
		return wrapDB("consume redemption", tx.Commit(ctx))
	}
	if status != string(domain.RedemptionReleased) {
		return wrapDB("consume redemption", tx.Commit(ctx))
	}

	if _, err := tx.Exec(ctx, `
		UPDATE promotion_redemptions
		SET status = 'consumed', paid_at = $2, released_at = NULL
		WHERE order_id = $1 AND status = 'released'
	`, orderID, at.UTC()); err != nil {
		return wrapDB("consume redemption", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE promotions SET redemption_count = redemption_count + 1 WHERE code = $1
	`, code); err != nil {
		return wrapDB("consume redemption", err)
	}
	return wrapDB("consume redemption", tx.Commit(ctx))
}

// Release hands the use back to the customer, for a cancel before shipment.
//
// It deliberately does not match consumed rows that belong to a shipped order:
// the caller decides whether a cancel is releasable, and passes only those
// here. Releasing also decrements the campaign count so a cancelled order does
// not permanently eat a slot.
func (s *PromotionRedemptionStore) Release(ctx context.Context, orderID string, at time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return wrapDB("release redemption", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var code string
	err = tx.QueryRow(ctx, `
		UPDATE promotion_redemptions
		SET status = 'released', released_at = $2
		WHERE order_id = $1 AND status IN ('reserved', 'consumed')
		RETURNING code
	`, orderID, at.UTC()).Scan(&code)
	if err == pgx.ErrNoRows {
		return nil // nothing held for this order; releasing twice is harmless
	}
	if err != nil {
		return wrapDB("release redemption", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE promotions SET redemption_count = GREATEST(redemption_count - 1, 0) WHERE code = $1
	`, code); err != nil {
		return wrapDB("release redemption", err)
	}
	return wrapDB("release redemption", tx.Commit(ctx))
}

func (s *PromotionRedemptionStore) ActiveCountForCustomer(ctx context.Context, code, customerID string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM promotion_redemptions
		WHERE code = $1 AND customer_id = $2 AND status IN ('reserved', 'consumed')
	`, domain.NormalizedCode(code), customerID).Scan(&n)
	return n, wrapDB("count customer redemptions", err)
}

func (s *PromotionRedemptionStore) CountsByCode(ctx context.Context, code string) (int, int, error) {
	var active, consumed int
	err := s.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status IN ('reserved', 'consumed')),
			count(*) FILTER (WHERE status = 'consumed')
		FROM promotion_redemptions WHERE code = $1
	`, domain.NormalizedCode(code)).Scan(&active, &consumed)
	return active, consumed, wrapDB("count redemptions", err)
}

func (s *PromotionRedemptionStore) reactivateReleased(
	ctx context.Context,
	tx pgx.Tx,
	row *domain.Redemption,
	in ports.ReserveRedemptionInput,
	code string,
	maxPerCustomer int,
) error {
	var redemptionCount int
	var maxRedemptions *int
	if err := tx.QueryRow(ctx, `
		SELECT redemption_count, max_redemptions FROM promotions WHERE code = $1 FOR UPDATE
	`, code).Scan(&redemptionCount, &maxRedemptions); err != nil {
		return wrapDB("reactivate released redemption", err)
	}
	if maxRedemptions != nil && redemptionCount >= *maxRedemptions {
		return ports.Conflict("promotion campaign exhausted")
	}

	var used int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM promotion_redemptions
		WHERE code = $1 AND customer_id = $2 AND status IN ('reserved', 'consumed')
		FOR UPDATE
	`, code, in.CustomerID).Scan(&used); err != nil {
		return wrapDB("reactivate released redemption", err)
	}
	if used >= maxPerCustomer {
		return ports.Conflict("promotion already used by this customer")
	}

	benefit, err := json.Marshal(in.AppliedBenefit)
	if err != nil {
		return fmt.Errorf("encode applied benefit: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE promotion_redemptions
		SET status = 'reserved', discount_won = $2, shipping_discount_won = $3,
			order_subtotal_won = $4, eligible_subtotal_won = $5, applied_benefit = $6,
			released_at = NULL
		WHERE order_id = $1 AND status = 'released'
	`, in.OrderID, in.DiscountWon, in.ShippingDiscountWon, in.OrderSubtotalWon,
		in.EligibleSubtotalWon, benefit); err != nil {
		return wrapDB("reactivate released redemption", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE promotions SET redemption_count = redemption_count + 1 WHERE code = $1
	`, code); err != nil {
		return wrapDB("reactivate released redemption", err)
	}

	row.Status = domain.RedemptionReserved
	row.DiscountWon = in.DiscountWon
	row.ShippingDiscountWon = in.ShippingDiscountWon
	row.OrderSubtotalWon = in.OrderSubtotalWon
	row.EligibleSubtotalWon = in.EligibleSubtotalWon
	row.AppliedBenefit = in.AppliedBenefit
	row.ReleasedAt = nil
	return nil
}

const redemptionColumns = `id, code, order_id, customer_id, status, discount_won, ` +
	`shipping_discount_won, order_subtotal_won, eligible_subtotal_won, applied_benefit, ` +
	`created_at, paid_at, released_at`

func scanRedemption(row scanner) (*domain.Redemption, error) {
	var (
		r       domain.Redemption
		status  string
		benefit []byte
	)
	if err := row.Scan(
		&r.ID, &r.Code, &r.OrderID, &r.CustomerID, &status, &r.DiscountWon,
		&r.ShippingDiscountWon, &r.OrderSubtotalWon, &r.EligibleSubtotalWon, &benefit,
		&r.CreatedAt, &r.PaidAt, &r.ReleasedAt,
	); err != nil {
		return nil, err
	}
	r.Status = domain.RedemptionStatus(status)
	if len(benefit) > 0 && string(benefit) != "null" {
		var b domain.Benefit
		if err := json.Unmarshal(benefit, &b); err != nil {
			return nil, fmt.Errorf("redemption %s: decode applied benefit: %w", r.ID, err)
		}
		r.AppliedBenefit = &b
	}
	return &r, nil
}
