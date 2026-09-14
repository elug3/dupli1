package pg

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

// A database created before confirmed/delivered/disputed existed carries the
// original five-value orders_status_check, and manager acceptance was a
// confirmed_at timestamp left on a still-`paid` row. Migrating must widen the
// constraint (it is replaced, not skipped as already-present) and promote
// those rows to the confirmed status, or every such order would be stuck:
// Ship now requires confirmed, and writing the new statuses would fail the
// stale CHECK.
func TestMigratePromotesLegacyConfirmedPaidOrders(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_status_stage_migration_test")
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Millisecond)

	if _, err := pool.Exec(ctx, `
		CREATE TABLE orders (
			id TEXT PRIMARY KEY,
			customer_id TEXT NOT NULL,
			reservation_id TEXT NOT NULL,
			status TEXT NOT NULL,
			coupon_code TEXT NOT NULL DEFAULT '',
			subtotal_krw BIGINT NOT NULL,
			discount_krw BIGINT NOT NULL,
			shipping_fee_krw BIGINT NOT NULL DEFAULT 0,
			total_krw BIGINT NOT NULL,
			confirmed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			CONSTRAINT orders_status_check
				CHECK (status IN ('pending', 'paid', 'in_transit', 'fulfilled', 'canceled'))
		)`); err != nil {
		t.Fatalf("create pre-stage orders table: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO orders (id, customer_id, reservation_id, status, subtotal_krw,
		                    discount_krw, total_krw, confirmed_at, created_at, updated_at)
		VALUES
			('ord-confirmed', 'cust-1', 'res-1', 'paid', 50000, 0, 50000, $1, $1, $1),
			('ord-unconfirmed', 'cust-2', 'res-2', 'paid', 50000, 0, 50000, NULL, $1, $1),
			('ord-shipped', 'cust-3', 'res-3', 'in_transit', 50000, 0, 50000, $1, $1, $1)
	`, now); err != nil {
		t.Fatalf("seed pre-stage orders: %v", err)
	}

	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for _, want := range []struct{ id, status string }{
		// Manager had already accepted this one under the old semantics.
		{"ord-confirmed", string(domain.StatusConfirmed)},
		// Never confirmed, so it still waits on the manager.
		{"ord-unconfirmed", string(domain.StatusPaid)},
		// Already past confirmation; ship set confirmed_at as a side effect.
		{"ord-shipped", string(domain.StatusInTransit)},
	} {
		var got string
		if err := pool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, want.id).Scan(&got); err != nil {
			t.Fatalf("read %s: %v", want.id, err)
		}
		if got != want.status {
			t.Fatalf("%s status = %q, want %q", want.id, got, want.status)
		}
	}

	// The widened constraint must accept every new status.
	for _, status := range []domain.OrderStatus{
		domain.StatusConfirmed, domain.StatusDelivered, domain.StatusDisputed,
	} {
		if _, err := pool.Exec(ctx, `UPDATE orders SET status = $1 WHERE id = 'ord-unconfirmed'`, status); err != nil {
			t.Fatalf("status %q rejected after migrate: %v", status, err)
		}
	}

	// Re-running migrate is a no-op, not a re-promotion.
	if err := repo.migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var stillPaid string
	if err := pool.QueryRow(ctx, `SELECT status FROM orders WHERE id = 'ord-shipped'`).Scan(&stillPaid); err != nil {
		t.Fatalf("read ord-shipped: %v", err)
	}
	if stillPaid != string(domain.StatusInTransit) {
		t.Fatalf("ord-shipped status = %q after second migrate, want in_transit", stillPaid)
	}
}
