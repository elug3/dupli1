package pg

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// InventoryStore is the product service's own stock/reservation store
// (merged in from the standalone inventory service). It shares the same
// connection pool as ProductSearchStore, the same way CouponStore does.
type InventoryStore struct {
	pool *pgxpool.Pool
}

func NewInventoryStore(pool *pgxpool.Pool) (*InventoryStore, error) {
	store := &InventoryStore{pool: pool}
	if err := store.migrate(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *InventoryStore) migrate() error {
	ctx := context.Background()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS id_sequences (
			name TEXT PRIMARY KEY,
			value BIGINT NOT NULL DEFAULT 0
		)`,
		// sku_id is a live FK to product_variants: stock is always attached to
		// an existing variant, so its sku string is joined at read time rather
		// than denormalized. ON DELETE RESTRICT blocks deleting a variant that
		// still has a stock row.
		`CREATE TABLE IF NOT EXISTS stock_items (
			sku_id TEXT PRIMARY KEY REFERENCES product_variants(sku_id) ON DELETE RESTRICT,
			quantity INTEGER NOT NULL DEFAULT 0,
			reserved INTEGER NOT NULL DEFAULT 0,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS reservations (
			id TEXT PRIMARY KEY,
			order_id TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
		// reservation_items intentionally has no FK to product_variants: it's a
		// historical/audit record that must survive a variant being deleted
		// later, so the sku string is denormalized (snapshot at reservation time).
		`CREATE TABLE IF NOT EXISTS reservation_items (
			reservation_id TEXT NOT NULL REFERENCES reservations(id) ON DELETE CASCADE,
			sku_id TEXT NOT NULL,
			sku TEXT NOT NULL DEFAULT '',
			quantity INTEGER NOT NULL,
			PRIMARY KEY (reservation_id, sku_id)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate inventory schema: %w", err)
		}
	}
	return nil
}

func (s *InventoryStore) nextReservationID(ctx context.Context, tx pgx.Tx) (string, error) {
	var seq int64
	err := tx.QueryRow(ctx, `
		INSERT INTO id_sequences (name, value) VALUES ('reservation', 1)
		ON CONFLICT (name) DO UPDATE SET value = id_sequences.value + 1
		RETURNING value
	`).Scan(&seq)
	if err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return fmt.Sprintf("res_%06d", seq), nil
}

func (s *InventoryStore) GetItem(ctx context.Context, skuID string) (*domain.StockItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var item domain.StockItem
	err := s.pool.QueryRow(ctx, `
		SELECT s.sku_id, pv.sku, s.quantity, s.reserved, s.updated_at
		FROM stock_items s JOIN product_variants pv ON pv.sku_id = s.sku_id
		WHERE s.sku_id = $1
	`, skuID).Scan(&item.SkuID, &item.SKU, &item.Quantity, &item.Reserved, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ports.ErrInventoryItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *InventoryStore) SaveItem(ctx context.Context, item *domain.StockItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO stock_items (sku_id, quantity, reserved, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (sku_id) DO UPDATE SET
			quantity = EXCLUDED.quantity,
			reserved = EXCLUDED.reserved,
			updated_at = EXCLUDED.updated_at
	`, item.SkuID, item.Quantity, item.Reserved, item.UpdatedAt)
	return err
}

func (s *InventoryStore) GetReservation(ctx context.Context, id string) (*domain.Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var reservation domain.Reservation
	err := s.pool.QueryRow(ctx, `
		SELECT id, order_id, status, created_at, updated_at
		FROM reservations WHERE id = $1
	`, id).Scan(&reservation.ID, &reservation.OrderID, &reservation.Status, &reservation.CreatedAt, &reservation.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ports.ErrInventoryItemNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT sku_id, sku, quantity FROM reservation_items WHERE reservation_id = $1 ORDER BY sku_id
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item domain.ReservationItem
		if err := rows.Scan(&item.SkuID, &item.SKU, &item.Quantity); err != nil {
			return nil, err
		}
		reservation.Items = append(reservation.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &reservation, nil
}

func (s *InventoryStore) SaveReservation(ctx context.Context, reservation *domain.Reservation) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO reservations (id, order_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET
			order_id = EXCLUDED.order_id,
			status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at
	`, reservation.ID, reservation.OrderID, reservation.Status, reservation.CreatedAt, reservation.UpdatedAt)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM reservation_items WHERE reservation_id = $1`, reservation.ID); err != nil {
		return err
	}
	for _, item := range reservation.Items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO reservation_items (reservation_id, sku_id, sku, quantity)
			VALUES ($1, $2, $3, $4)
		`, reservation.ID, item.SkuID, item.SKU, item.Quantity); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// CreateReservation expects items already resolved to a canonical SkuID by
// the service layer. It sorts and locks strictly by SkuID (never by the
// caller-supplied sku string) so two requests referencing the same
// underlying variant by different identifiers always take locks in the same
// order and can't deadlock.
func (s *InventoryStore) CreateReservation(ctx context.Context, orderID string, items []domain.ReservationItem, now time.Time) (*domain.Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sorted := append([]domain.ReservationItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].SkuID < sorted[j].SkuID })

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	for _, item := range sorted {
		var quantity, reserved int
		err := tx.QueryRow(ctx, `
			SELECT quantity, reserved FROM stock_items WHERE sku_id = $1 FOR UPDATE
		`, item.SkuID).Scan(&quantity, &reserved)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrInventoryItemNotFound
		}
		if err != nil {
			return nil, err
		}
		if quantity-reserved < item.Quantity {
			return nil, ports.ErrInsufficientStock
		}
		if _, err := tx.Exec(ctx, `
			UPDATE stock_items SET reserved = reserved + $2, updated_at = $3 WHERE sku_id = $1
		`, item.SkuID, item.Quantity, now); err != nil {
			return nil, err
		}
	}

	reservationID, err := s.nextReservationID(ctx, tx)
	if err != nil {
		return nil, err
	}
	reservation := domain.NewReservation(reservationID, orderID, items, now)

	if _, err := tx.Exec(ctx, `
		INSERT INTO reservations (id, order_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, reservation.ID, reservation.OrderID, reservation.Status, reservation.CreatedAt, reservation.UpdatedAt); err != nil {
		return nil, err
	}
	for _, item := range reservation.Items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO reservation_items (reservation_id, sku_id, sku, quantity) VALUES ($1, $2, $3, $4)
		`, reservation.ID, item.SkuID, item.SKU, item.Quantity); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return reservation, nil
}

func (s *InventoryStore) FinalizeReservation(ctx context.Context, id string, status domain.ReservationStatus, now time.Time) (*domain.Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var reservation domain.Reservation
	err = tx.QueryRow(ctx, `
		SELECT id, order_id, status, created_at, updated_at
		FROM reservations WHERE id = $1 FOR UPDATE
	`, id).Scan(&reservation.ID, &reservation.OrderID, &reservation.Status, &reservation.CreatedAt, &reservation.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ports.ErrInventoryItemNotFound
	}
	if err != nil {
		return nil, err
	}
	if !reservation.IsActive() {
		return nil, ports.ErrReservationClosed
	}

	rows, err := tx.Query(ctx, `
		SELECT sku_id, sku, quantity FROM reservation_items WHERE reservation_id = $1 ORDER BY sku_id
	`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item domain.ReservationItem
		if err := rows.Scan(&item.SkuID, &item.SKU, &item.Quantity); err != nil {
			rows.Close()
			return nil, err
		}
		reservation.Items = append(reservation.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	for _, item := range reservation.Items {
		var quantity, reserved int
		err := tx.QueryRow(ctx, `
			SELECT quantity, reserved FROM stock_items WHERE sku_id = $1 FOR UPDATE
		`, item.SkuID).Scan(&quantity, &reserved)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrInventoryItemNotFound
		}
		if err != nil {
			return nil, err
		}
		if reserved < item.Quantity {
			return nil, ports.ErrInsufficientStock
		}
		nextReserved := reserved - item.Quantity
		nextQuantity := quantity
		if status == domain.ReservationCommitted {
			if quantity < item.Quantity {
				return nil, ports.ErrInsufficientStock
			}
			nextQuantity = quantity - item.Quantity
		}
		if _, err := tx.Exec(ctx, `
			UPDATE stock_items SET quantity = $2, reserved = $3, updated_at = $4 WHERE sku_id = $1
		`, item.SkuID, nextQuantity, nextReserved, now); err != nil {
			return nil, err
		}
	}

	reservation.Status = status
	reservation.UpdatedAt = now
	if _, err := tx.Exec(ctx, `
		UPDATE reservations SET status = $2, updated_at = $3 WHERE id = $1
	`, id, status, now); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &reservation, nil
}

// Ensure InventoryStore implements ports.InventoryStore at compile time.
var _ ports.InventoryStore = (*InventoryStore)(nil)
