package pg

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(connString string) (*Repository, error) {
	connString = withPostgresSSLMode(connString)
	pool, err := pgxpool.Connect(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("connect order database: %w", err)
	}

	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		pool.Close()
		return nil, err
	}
	return repo, nil
}

func (r *Repository) Close() {
	if r.pool != nil {
		r.pool.Close()
	}
}

func (r *Repository) migrate() error {
	ctx := context.Background()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS id_sequences (
			name TEXT PRIMARY KEY,
			value BIGINT NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS orders (
			id TEXT PRIMARY KEY,
			customer_id TEXT NOT NULL,
			reservation_id TEXT NOT NULL,
			status TEXT NOT NULL,
			coupon_code TEXT NOT NULL DEFAULT '',
			subtotal_cents BIGINT NOT NULL,
			discount_cents BIGINT NOT NULL,
			total_cents BIGINT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_customer_id ON orders(customer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_pending_payment_due_at ON orders(payment_due_at) WHERE status = 'pending'`,
		`CREATE TABLE IF NOT EXISTS order_items (
			order_id TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
			sku TEXT NOT NULL,
			quantity INTEGER NOT NULL,
			unit_price_cents BIGINT NOT NULL,
			PRIMARY KEY (order_id, sku)
		)`,
		`CREATE TABLE IF NOT EXISTS checkout_sessions (
			id TEXT PRIMARY KEY,
			customer_id TEXT NOT NULL,
			status TEXT NOT NULL,
			coupon_code TEXT NOT NULL DEFAULT '',
			subtotal_cents BIGINT NOT NULL DEFAULT 0,
			discount_cents BIGINT NOT NULL DEFAULT 0,
			total_cents BIGINT NOT NULL DEFAULT 0,
			order_id TEXT NOT NULL DEFAULT '',
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS checkout_session_items (
			session_id TEXT NOT NULL REFERENCES checkout_sessions(id) ON DELETE CASCADE,
			sku TEXT NOT NULL,
			quantity INTEGER NOT NULL,
			unit_price_cents BIGINT NOT NULL,
			PRIMARY KEY (session_id, sku)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := r.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate order schema: %w", err)
		}
	}
	alterStmts := []string{
		`ALTER TABLE orders ADD COLUMN IF NOT EXISTS payment_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE orders ADD COLUMN IF NOT EXISTS paid_at TIMESTAMPTZ`,
		`ALTER TABLE orders ADD COLUMN IF NOT EXISTS payment_due_at TIMESTAMPTZ`,
		`ALTER TABLE orders ADD COLUMN IF NOT EXISTS shipped_by TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE orders ADD COLUMN IF NOT EXISTS shipped_at TIMESTAMPTZ`,
		`ALTER TABLE order_items ADD COLUMN IF NOT EXISTS sku_id TEXT`,
		`ALTER TABLE checkout_session_items ADD COLUMN IF NOT EXISTS sku_id TEXT`,
	}
	for _, stmt := range alterStmts {
		_, _ = r.pool.Exec(ctx, stmt)
	}
	_, _ = r.pool.Exec(ctx, `UPDATE orders SET payment_due_at = created_at + INTERVAL '5 minutes' WHERE payment_due_at IS NULL`)

	if err := r.promoteSkuIDPrimaryKey(ctx, "order_items", "order_id"); err != nil {
		return err
	}
	if err := r.promoteSkuIDPrimaryKey(ctx, "checkout_session_items", "session_id"); err != nil {
		return err
	}
	return nil
}

// promoteSkuIDPrimaryKey swaps table's primary key from (parentCol, sku) to
// (parentCol, sku_id), unlike cart's ephemeral data this table holds
// permanent historical/financial records, so rows can't simply be purged.
// Promotion is gated on every row already having a sku_id — cmd/backfill-sku-id
// resolves historical sku strings via product's API first. Until that's
// complete, this is a no-op on every startup (logged once, not an error) and
// the table stays on its legacy (parentCol, sku) key.
func (r *Repository) promoteSkuIDPrimaryKey(ctx context.Context, table, parentCol string) error {
	var pkColumns []string
	rows, err := r.pool.Query(ctx, `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = $1::regclass AND i.indisprimary
		ORDER BY array_position(i.indkey, a.attnum)
	`, table)
	if err != nil {
		return fmt.Errorf("check %s primary key: %w", table, err)
	}
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			rows.Close()
			return fmt.Errorf("check %s primary key: %w", table, err)
		}
		pkColumns = append(pkColumns, col)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("check %s primary key: %w", table, err)
	}
	if len(pkColumns) == 2 && pkColumns[1] == "sku_id" {
		return nil
	}

	var remaining int
	if err := r.pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE sku_id IS NULL`, table)).Scan(&remaining); err != nil {
		return fmt.Errorf("count unresolved %s rows: %w", table, err)
	}
	if remaining > 0 {
		log.Printf("order: %s has %d row(s) with no sku_id yet; run cmd/backfill-sku-id, then restart to promote the primary key (staying on legacy (%s, sku) key for now)",
			table, remaining, parentCol)
		return nil
	}

	if _, err := r.pool.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN sku_id SET NOT NULL`, table)); err != nil {
		return fmt.Errorf("set %s.sku_id not null: %w", table, err)
	}
	if _, err := r.pool.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s DROP CONSTRAINT %s_pkey`, table, table)); err != nil {
		return fmt.Errorf("drop legacy %s pkey: %w", table, err)
	}
	if _, err := r.pool.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ADD PRIMARY KEY (%s, sku_id)`, table, parentCol)); err != nil {
		return fmt.Errorf("promote %s primary key: %w", table, err)
	}
	log.Printf("order: promoted %s primary key to (%s, sku_id)", table, parentCol)
	return nil
}

func (r *Repository) NextOrderID(ctx context.Context) (string, error) {
	return r.nextID(ctx, "order", "ord")
}

func (r *Repository) NextCheckoutSessionID(ctx context.Context) (string, error) {
	return r.nextID(ctx, "checkout_session", "cs")
}

func (r *Repository) nextID(ctx context.Context, name, prefix string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var seq int64
	err := r.pool.QueryRow(ctx, `
		INSERT INTO id_sequences (name, value) VALUES ($1, 1)
		ON CONFLICT (name) DO UPDATE SET value = id_sequences.value + 1
		RETURNING value
	`, name).Scan(&seq)
	if err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return fmt.Sprintf("%s_%06d", prefix, seq), nil
}

func (r *Repository) Save(ctx context.Context, order *domain.Order) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO orders (
			id, customer_id, reservation_id, status, coupon_code,
			subtotal_cents, discount_cents, total_cents,
			payment_id, paid_at, payment_due_at, shipped_by, shipped_at,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (id) DO UPDATE SET
			customer_id = EXCLUDED.customer_id,
			reservation_id = EXCLUDED.reservation_id,
			status = EXCLUDED.status,
			coupon_code = EXCLUDED.coupon_code,
			subtotal_cents = EXCLUDED.subtotal_cents,
			discount_cents = EXCLUDED.discount_cents,
			total_cents = EXCLUDED.total_cents,
			payment_id = EXCLUDED.payment_id,
			paid_at = EXCLUDED.paid_at,
			payment_due_at = EXCLUDED.payment_due_at,
			shipped_by = EXCLUDED.shipped_by,
			shipped_at = EXCLUDED.shipped_at,
			updated_at = EXCLUDED.updated_at
	`, order.ID, order.CustomerID, order.ReservationID, order.Status, order.CouponCode,
		order.SubtotalCents, order.DiscountCents, order.TotalCents,
		order.PaymentID, order.PaidAt, order.PaymentDueAt, order.ShippedBy, order.ShippedAt,
		order.CreatedAt, order.UpdatedAt)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM order_items WHERE order_id = $1`, order.ID); err != nil {
		return err
	}
	for _, item := range order.Items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO order_items (order_id, sku, sku_id, quantity, unit_price_cents)
			VALUES ($1, $2, $3, $4, $5)
		`, order.ID, item.SKU, nullIfEmpty(item.SkuID), item.Quantity, item.UnitPriceCents); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (r *Repository) Get(ctx context.Context, id string) (*domain.Order, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var order domain.Order
	var paidAt, shippedAt *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT id, customer_id, reservation_id, status, coupon_code,
			subtotal_cents, discount_cents, total_cents,
			payment_id, paid_at, payment_due_at, shipped_by, shipped_at,
			created_at, updated_at
		FROM orders WHERE id = $1
	`, id).Scan(
		&order.ID, &order.CustomerID, &order.ReservationID, &order.Status, &order.CouponCode,
		&order.SubtotalCents, &order.DiscountCents, &order.TotalCents,
		&order.PaymentID, &paidAt, &order.PaymentDueAt, &order.ShippedBy, &shippedAt,
		&order.CreatedAt, &order.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	order.PaidAt = paidAt
	order.ShippedAt = shippedAt

	items, err := r.loadOrderItems(ctx, id)
	if err != nil {
		return nil, err
	}
	order.Items = items
	return &order, nil
}

func (r *Repository) loadOrderItems(ctx context.Context, orderID string) ([]domain.OrderItem, error) {
	byOrder, err := r.loadOrderItemsBatch(ctx, []string{orderID})
	if err != nil {
		return nil, err
	}
	return byOrder[orderID], nil
}

func (r *Repository) loadOrderItemsBatch(ctx context.Context, orderIDs []string) (map[string][]domain.OrderItem, error) {
	out := make(map[string][]domain.OrderItem, len(orderIDs))
	if len(orderIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT order_id, sku, COALESCE(sku_id, ''), quantity, unit_price_cents
		FROM order_items
		WHERE order_id = ANY($1)
		ORDER BY order_id, sku
	`, toTextArray(orderIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var orderID string
		var item domain.OrderItem
		if err := rows.Scan(&orderID, &item.SKU, &item.SkuID, &item.Quantity, &item.UnitPriceCents); err != nil {
			return nil, err
		}
		out[orderID] = append(out[orderID], item)
	}
	return out, rows.Err()
}

func toTextArray(ss []string) pgtype.TextArray {
	if len(ss) == 0 {
		return pgtype.TextArray{Status: pgtype.Present}
	}
	elements := make([]pgtype.Text, len(ss))
	for i, s := range ss {
		elements[i] = pgtype.Text{String: s, Status: pgtype.Present}
	}
	return pgtype.TextArray{
		Elements:   elements,
		Dimensions: []pgtype.ArrayDimension{{Length: int32(len(ss)), LowerBound: 1}},
		Status:     pgtype.Present,
	}
}

func (r *Repository) ListByCustomer(ctx context.Context, customerID string) ([]domain.Order, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, customer_id, reservation_id, status, coupon_code,
			subtotal_cents, discount_cents, total_cents,
			payment_id, paid_at, payment_due_at, shipped_by, shipped_at,
			created_at, updated_at
		FROM orders WHERE customer_id = $1 ORDER BY created_at DESC
	`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []domain.Order
	var ids []string
	for rows.Next() {
		var order domain.Order
		var paidAt, shippedAt *time.Time
		if err := rows.Scan(
			&order.ID, &order.CustomerID, &order.ReservationID, &order.Status, &order.CouponCode,
			&order.SubtotalCents, &order.DiscountCents, &order.TotalCents,
			&order.PaymentID, &paidAt, &order.PaymentDueAt, &order.ShippedBy, &shippedAt,
			&order.CreatedAt, &order.UpdatedAt,
		); err != nil {
			return nil, err
		}
		order.PaidAt = paidAt
		order.ShippedAt = shippedAt
		orders = append(orders, order)
		ids = append(ids, order.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	itemsByOrder, err := r.loadOrderItemsBatch(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		orders[i].Items = itemsByOrder[orders[i].ID]
	}
	return orders, nil
}

func (r *Repository) ListPendingPaymentExpired(ctx context.Context, now time.Time) ([]domain.Order, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, customer_id, reservation_id, status, coupon_code,
			subtotal_cents, discount_cents, total_cents,
			payment_id, paid_at, payment_due_at, shipped_by, shipped_at,
			created_at, updated_at
		FROM orders
		WHERE status = $1 AND payment_due_at < $2
	`, domain.StatusPending, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []domain.Order
	var ids []string
	for rows.Next() {
		var order domain.Order
		var paidAt, shippedAt *time.Time
		if err := rows.Scan(
			&order.ID, &order.CustomerID, &order.ReservationID, &order.Status, &order.CouponCode,
			&order.SubtotalCents, &order.DiscountCents, &order.TotalCents,
			&order.PaymentID, &paidAt, &order.PaymentDueAt, &order.ShippedBy, &shippedAt,
			&order.CreatedAt, &order.UpdatedAt,
		); err != nil {
			return nil, err
		}
		order.PaidAt = paidAt
		order.ShippedAt = shippedAt
		orders = append(orders, order)
		ids = append(ids, order.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	itemsByOrder, err := r.loadOrderItemsBatch(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		orders[i].Items = itemsByOrder[orders[i].ID]
	}
	return orders, nil
}

func (r *Repository) SaveCheckoutSession(ctx context.Context, session *domain.CheckoutSession) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO checkout_sessions (
			id, customer_id, status, coupon_code, subtotal_cents, discount_cents, total_cents,
			order_id, expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			customer_id = EXCLUDED.customer_id,
			status = EXCLUDED.status,
			coupon_code = EXCLUDED.coupon_code,
			subtotal_cents = EXCLUDED.subtotal_cents,
			discount_cents = EXCLUDED.discount_cents,
			total_cents = EXCLUDED.total_cents,
			order_id = EXCLUDED.order_id,
			expires_at = EXCLUDED.expires_at,
			updated_at = EXCLUDED.updated_at
	`, session.ID, session.CustomerID, session.Status, session.CouponCode,
		session.SubtotalCents, session.DiscountCents, session.TotalCents,
		session.OrderID, session.ExpiresAt, session.CreatedAt, session.UpdatedAt)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM checkout_session_items WHERE session_id = $1`, session.ID); err != nil {
		return err
	}
	for _, item := range session.Items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO checkout_session_items (session_id, sku, sku_id, quantity, unit_price_cents)
			VALUES ($1, $2, $3, $4, $5)
		`, session.ID, item.SKU, nullIfEmpty(item.SkuID), item.Quantity, item.UnitPriceCents); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) GetCheckoutSession(ctx context.Context, id string) (*domain.CheckoutSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var session domain.CheckoutSession
	err := r.pool.QueryRow(ctx, `
		SELECT id, customer_id, status, coupon_code, subtotal_cents, discount_cents, total_cents,
			order_id, expires_at, created_at, updated_at
		FROM checkout_sessions WHERE id = $1
	`, id).Scan(
		&session.ID, &session.CustomerID, &session.Status, &session.CouponCode,
		&session.SubtotalCents, &session.DiscountCents, &session.TotalCents,
		&session.OrderID, &session.ExpiresAt, &session.CreatedAt, &session.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT sku, COALESCE(sku_id, ''), quantity, unit_price_cents FROM checkout_session_items WHERE session_id = $1 ORDER BY sku
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item domain.OrderItem
		if err := rows.Scan(&item.SKU, &item.SkuID, &item.Quantity, &item.UnitPriceCents); err != nil {
			return nil, err
		}
		session.Items = append(session.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &session, nil
}

var _ ports.Repository = (*Repository)(nil)
