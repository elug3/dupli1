package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/ports"
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
		return nil, fmt.Errorf("connect payment database: %w", err)
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
		`CREATE SEQUENCE IF NOT EXISTS payment_id_seq`,
		`CREATE TABLE IF NOT EXISTS payments (
			id              TEXT PRIMARY KEY,
			order_id        TEXT NOT NULL,
			customer_id     TEXT NOT NULL,
			amount_cents    BIGINT NOT NULL CHECK (amount_cents > 0),
			currency        TEXT NOT NULL,
			status          TEXT NOT NULL,
			provider        TEXT NOT NULL,
			provider_ref    TEXT NOT NULL,
			checkout_url    TEXT,
			idempotency_key TEXT UNIQUE,
			expires_at      TIMESTAMPTZ NOT NULL,
			created_at      TIMESTAMPTZ NOT NULL,
			updated_at      TIMESTAMPTZ NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_payments_provider_ref ON payments(provider_ref)`,
		`CREATE INDEX IF NOT EXISTS idx_payments_order_id ON payments(order_id)`,
	}
	for _, stmt := range stmts {
		if _, err := r.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate payment schema: %w", err)
		}
	}
	return nil
}

func (r *Repository) NextPaymentID(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var id string
	err := r.pool.QueryRow(ctx,
		`SELECT 'pay_' || LPAD(nextval('payment_id_seq')::text, 6, '0')`,
	).Scan(&id)
	return id, err
}

func (r *Repository) Save(ctx context.Context, payment *domain.Payment) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var idempotencyKey any
	if payment.IdempotencyKey != "" {
		idempotencyKey = payment.IdempotencyKey
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO payments (
			id, order_id, customer_id, amount_cents, currency, status,
			provider, provider_ref, checkout_url, idempotency_key,
			expires_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			provider_ref = EXCLUDED.provider_ref,
			checkout_url = EXCLUDED.checkout_url,
			updated_at = EXCLUDED.updated_at
	`, payment.ID, payment.OrderID, payment.CustomerID, payment.AmountCents, payment.Currency,
		string(payment.Status), payment.Provider, payment.ProviderRef, payment.CheckoutURL,
		idempotencyKey, payment.ExpiresAt, payment.CreatedAt, payment.UpdatedAt)
	return err
}

func (r *Repository) Get(ctx context.Context, id string) (*domain.Payment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	row := r.pool.QueryRow(ctx, paymentSelect+` WHERE id = $1`, id)
	return scanPayment(row)
}

func (r *Repository) GetByProviderRef(ctx context.Context, providerRef string) (*domain.Payment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	row := r.pool.QueryRow(ctx, paymentSelect+` WHERE provider_ref = $1`, providerRef)
	return scanPayment(row)
}

func (r *Repository) FindByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	row := r.pool.QueryRow(ctx, paymentSelect+` WHERE idempotency_key = $1`, key)
	return scanPayment(row)
}

const paymentSelect = `
	SELECT id, order_id, customer_id, amount_cents, currency, status,
	       provider, provider_ref, checkout_url, COALESCE(idempotency_key, ''),
	       expires_at, created_at, updated_at
	FROM payments`

func scanPayment(row pgx.Row) (*domain.Payment, error) {
	var p domain.Payment
	var status string
	var idempotencyKey string
	err := row.Scan(
		&p.ID, &p.OrderID, &p.CustomerID, &p.AmountCents, &p.Currency, &status,
		&p.Provider, &p.ProviderRef, &p.CheckoutURL, &idempotencyKey,
		&p.ExpiresAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Status = domain.PaymentStatus(status)
	p.IdempotencyKey = idempotencyKey
	return &p, nil
}

var _ ports.Repository = (*Repository)(nil)
