package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/elug3/dupli1/inventory/pkg/domain"
	"github.com/elug3/dupli1/inventory/pkg/ports"
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
		return nil, fmt.Errorf("connect inventory database: %w", err)
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
		`CREATE TABLE IF NOT EXISTS stock_items (
			sku TEXT PRIMARY KEY,
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
		`CREATE TABLE IF NOT EXISTS reservation_items (
			reservation_id TEXT NOT NULL REFERENCES reservations(id) ON DELETE CASCADE,
			sku TEXT NOT NULL,
			quantity INTEGER NOT NULL,
			PRIMARY KEY (reservation_id, sku)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := r.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate inventory schema: %w", err)
		}
	}
	return nil
}

func (r *Repository) NextReservationID(ctx context.Context) (string, error) {
	return r.nextID(ctx, "reservation", "res")
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

func (r *Repository) GetItem(ctx context.Context, sku string) (*domain.StockItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var item domain.StockItem
	err := r.pool.QueryRow(ctx,
		`SELECT sku, quantity, reserved, updated_at FROM stock_items WHERE sku = $1`, sku,
	).Scan(&item.SKU, &item.Quantity, &item.Reserved, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) SaveItem(ctx context.Context, item *domain.StockItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO stock_items (sku, quantity, reserved, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (sku) DO UPDATE SET
			quantity = EXCLUDED.quantity,
			reserved = EXCLUDED.reserved,
			updated_at = EXCLUDED.updated_at
	`, item.SKU, item.Quantity, item.Reserved, item.UpdatedAt)
	return err
}

func (r *Repository) GetReservation(ctx context.Context, id string) (*domain.Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var reservation domain.Reservation
	err := r.pool.QueryRow(ctx, `
		SELECT id, order_id, status, created_at, updated_at
		FROM reservations WHERE id = $1
	`, id).Scan(&reservation.ID, &reservation.OrderID, &reservation.Status, &reservation.CreatedAt, &reservation.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT sku, quantity FROM reservation_items WHERE reservation_id = $1 ORDER BY sku
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item domain.ReservationItem
		if err := rows.Scan(&item.SKU, &item.Quantity); err != nil {
			return nil, err
		}
		reservation.Items = append(reservation.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &reservation, nil
}

func (r *Repository) SaveReservation(ctx context.Context, reservation *domain.Reservation) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
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
			INSERT INTO reservation_items (reservation_id, sku, quantity)
			VALUES ($1, $2, $3)
		`, reservation.ID, item.SKU, item.Quantity); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Ensure Repository implements ports.Repository at compile time.
var _ ports.Repository = (*Repository)(nil)
