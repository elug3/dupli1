package pg

import (
	"testing"
	"time"
)

// Databases that stored order money under *_cents must keep those snapshotted
// values after the columns are renamed to *_krw.
func TestMigrateRenamesMoneyCentsColumns(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_money_rename_test")
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Millisecond)

	if _, err := pool.Exec(ctx, `
		CREATE TABLE orders (
			id TEXT PRIMARY KEY,
			customer_id TEXT NOT NULL,
			reservation_id TEXT NOT NULL,
			status TEXT NOT NULL,
			coupon_code TEXT NOT NULL DEFAULT '',
			subtotal_cents BIGINT NOT NULL,
			discount_cents BIGINT NOT NULL,
			shipping_fee_cents BIGINT NOT NULL DEFAULT 0,
			total_cents BIGINT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`); err != nil {
		t.Fatalf("create cents-era orders table: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO orders (id, customer_id, reservation_id, status, subtotal_cents,
		                    discount_cents, shipping_fee_cents, total_cents, created_at, updated_at)
		VALUES ('ord-cents', 'cust-1', 'res-cents', 'pending', 250000, 10000, 3000, 243000, $1, $1)
	`, now); err != nil {
		t.Fatalf("seed cents-era order: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE TABLE order_items (
			order_id TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
			sku TEXT NOT NULL,
			quantity INTEGER NOT NULL,
			unit_price_cents BIGINT NOT NULL,
			PRIMARY KEY (order_id, sku)
		)`); err != nil {
		t.Fatalf("create cents-era order_items table: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO order_items (order_id, sku, quantity, unit_price_cents)
		VALUES ('ord-cents', 'BAG-001', 1, 250000)
	`); err != nil {
		t.Fatalf("seed cents-era order item: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE TABLE checkout_sessions (
			id TEXT PRIMARY KEY,
			customer_id TEXT NOT NULL,
			status TEXT NOT NULL,
			coupon_code TEXT NOT NULL DEFAULT '',
			subtotal_cents BIGINT NOT NULL DEFAULT 0,
			discount_cents BIGINT NOT NULL DEFAULT 0,
			shipping_fee_cents BIGINT NOT NULL DEFAULT 0,
			total_cents BIGINT NOT NULL DEFAULT 0,
			order_id TEXT NOT NULL DEFAULT '',
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`); err != nil {
		t.Fatalf("create cents-era checkout_sessions table: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO checkout_sessions (id, customer_id, status, subtotal_cents, discount_cents,
		                               shipping_fee_cents, total_cents, expires_at, created_at, updated_at)
		VALUES ('cs-cents', 'cust-1', 'open', 250000, 10000, 3000, 243000, $1, $1, $1)
	`, now.Add(time.Hour)); err != nil {
		t.Fatalf("seed cents-era session: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE TABLE checkout_session_items (
			session_id TEXT NOT NULL REFERENCES checkout_sessions(id) ON DELETE CASCADE,
			sku TEXT NOT NULL,
			quantity INTEGER NOT NULL,
			unit_price_cents BIGINT NOT NULL,
			PRIMARY KEY (session_id, sku)
		)`); err != nil {
		t.Fatalf("create cents-era checkout_session_items table: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO checkout_session_items (session_id, sku, quantity, unit_price_cents)
		VALUES ('cs-cents', 'BAG-001', 1, 250000)
	`); err != nil {
		t.Fatalf("seed cents-era session item: %v", err)
	}

	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate rename: %v", err)
	}

	pairs := []struct{ table, krw, legacy string }{
		{"orders", "subtotal_krw", "subtotal_cents"},
		{"orders", "discount_krw", "discount_cents"},
		{"orders", "shipping_fee_krw", "shipping_fee_cents"},
		{"orders", "total_krw", "total_cents"},
		{"checkout_sessions", "subtotal_krw", "subtotal_cents"},
		{"checkout_sessions", "discount_krw", "discount_cents"},
		{"checkout_sessions", "shipping_fee_krw", "shipping_fee_cents"},
		{"checkout_sessions", "total_krw", "total_cents"},
		{"order_items", "unit_price_krw", "unit_price_cents"},
		{"checkout_session_items", "unit_price_krw", "unit_price_cents"},
	}
	for _, pair := range pairs {
		var krw, legacy int
		if err := repo.pool.QueryRow(ctx, `
			SELECT
				count(*) FILTER (WHERE column_name = $2),
				count(*) FILTER (WHERE column_name = $3)
			FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1
		`, pair.table, pair.krw, pair.legacy).Scan(&krw, &legacy); err != nil {
			t.Fatalf("inspect %s.%s: %v", pair.table, pair.krw, err)
		}
		if krw != 1 || legacy != 0 {
			t.Fatalf("%s columns: %s=%d %s=%d, want 1 / 0", pair.table, pair.krw, krw, pair.legacy, legacy)
		}
	}

	got, err := repo.Get(ctx, "ord-cents")
	if err != nil {
		t.Fatalf("Get after rename: %v", err)
	}
	if got.SubtotalKRW != 250000 || got.DiscountKRW != 10000 || got.ShippingFeeKRW != 3000 || got.TotalKRW != 243000 {
		t.Fatalf("renamed order money = subtotal %d discount %d shipping %d total %d",
			got.SubtotalKRW, got.DiscountKRW, got.ShippingFeeKRW, got.TotalKRW)
	}
	if len(got.Items) != 1 || got.Items[0].UnitPriceKRW != 250000 {
		t.Fatalf("renamed order item = %+v, want unit_price_krw 250000", got.Items)
	}

	session, err := repo.GetCheckoutSession(ctx, "cs-cents")
	if err != nil {
		t.Fatalf("GetCheckoutSession after rename: %v", err)
	}
	if session.SubtotalKRW != 250000 || session.DiscountKRW != 10000 || session.ShippingFeeKRW != 3000 || session.TotalKRW != 243000 {
		t.Fatalf("renamed session money = subtotal %d discount %d shipping %d total %d",
			session.SubtotalKRW, session.DiscountKRW, session.ShippingFeeKRW, session.TotalKRW)
	}
	if len(session.Items) != 1 || session.Items[0].UnitPriceKRW != 250000 {
		t.Fatalf("renamed session item = %+v, want unit_price_krw 250000", session.Items)
	}
}
