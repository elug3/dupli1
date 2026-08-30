package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elug3/dupli1/notification/pkg/domain"
	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/oklog/ulid/v2"
)

type TelegramRepository struct {
	pool *pgxpool.Pool
}

func NewTelegramRepository(connString string) (*TelegramRepository, error) {
	connString = withPostgresSSLMode(connString)
	// Pool connect at process start; no request context available.
	pool, err := pgxpool.Connect(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("connect notification database: %w", err)
	}
	repo := &TelegramRepository{pool: pool}
	if err := repo.migrate(); err != nil {
		pool.Close()
		return nil, err
	}
	return repo, nil
}

func (r *TelegramRepository) Close() {
	if r.pool != nil {
		r.pool.Close()
	}
}

func (r *TelegramRepository) migrate() error {
	// Startup schema migration; no request-scoped context to propagate.
	ctx := context.Background()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS telegram_subscriptions (
			id                TEXT PRIMARY KEY,
			telegram_user_id  BIGINT,
			chat_id           TEXT NOT NULL,
			chat_type         TEXT NOT NULL DEFAULT '',
			chat_label        TEXT NOT NULL DEFAULT '',
			username          TEXT NOT NULL DEFAULT '',
			status            TEXT NOT NULL DEFAULT 'pending',
			alert_order       BOOLEAN NOT NULL DEFAULT FALSE,
			alert_product     BOOLEAN NOT NULL DEFAULT FALSE,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			accepted_at       TIMESTAMPTZ,
			accepted_by       TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS telegram_subscriptions_chat_id_idx
		 ON telegram_subscriptions (chat_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS telegram_subscriptions_user_id_idx
		 ON telegram_subscriptions (telegram_user_id) WHERE telegram_user_id IS NOT NULL`,
	}
	for _, stmt := range stmts {
		if _, err := r.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate notification schema: %w", err)
		}
	}
	return nil
}

func (r *TelegramRepository) UpsertPending(ctx context.Context, in ports.TelegramSubscriptionInput) (*domain.TelegramSubscription, error) {
	chatID := strings.TrimSpace(in.ChatID)
	if chatID == "" {
		return nil, fmt.Errorf("chat id is required")
	}

	if existing, err := r.FindByChatID(ctx, chatID); err == nil && existing != nil {
		return existing, nil
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	if in.TelegramUserID != nil {
		if existing, err := r.FindByUserID(ctx, *in.TelegramUserID); err == nil && existing != nil {
			return existing, nil
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}

	now := time.Now().UTC()
	sub := domain.TelegramSubscription{
		ID:             ulid.Make().String(),
		TelegramUserID: in.TelegramUserID,
		ChatID:         chatID,
		ChatType:       strings.TrimSpace(in.ChatType),
		ChatLabel:      strings.TrimSpace(in.ChatLabel),
		Username:       strings.TrimSpace(in.Username),
		Status:         domain.SubscriptionStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO telegram_subscriptions (
			id, telegram_user_id, chat_id, chat_type, chat_label, username, status, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		sub.ID, sub.TelegramUserID, sub.ChatID, sub.ChatType, sub.ChatLabel, sub.Username, sub.Status, sub.CreatedAt, sub.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert telegram subscription: %w", err)
	}
	return &sub, nil
}

func (r *TelegramRepository) List(ctx context.Context, status string) ([]domain.TelegramSubscription, error) {
	status = strings.TrimSpace(status)
	query := `SELECT id, telegram_user_id, chat_id, chat_type, chat_label, username, status,
		alert_order, alert_product, created_at, updated_at, accepted_at, accepted_by
		FROM telegram_subscriptions`
	args := []any{}
	if status != "" {
		query += ` WHERE status = $1`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptions(rows)
}

func (r *TelegramRepository) GetByID(ctx context.Context, id string) (*domain.TelegramSubscription, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, telegram_user_id, chat_id, chat_type, chat_label, username, status,
			alert_order, alert_product, created_at, updated_at, accepted_at, accepted_by
		FROM telegram_subscriptions WHERE id = $1`, id)
	return scanSubscription(row)
}

func (r *TelegramRepository) FindByChatID(ctx context.Context, chatID string) (*domain.TelegramSubscription, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, telegram_user_id, chat_id, chat_type, chat_label, username, status,
			alert_order, alert_product, created_at, updated_at, accepted_at, accepted_by
		FROM telegram_subscriptions WHERE chat_id = $1`, strings.TrimSpace(chatID))
	return scanSubscription(row)
}

func (r *TelegramRepository) FindByUserID(ctx context.Context, userID int64) (*domain.TelegramSubscription, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, telegram_user_id, chat_id, chat_type, chat_label, username, status,
			alert_order, alert_product, created_at, updated_at, accepted_at, accepted_by
		FROM telegram_subscriptions WHERE telegram_user_id = $1`, userID)
	return scanSubscription(row)
}

func (r *TelegramRepository) CreateAccepted(ctx context.Context, in ports.TelegramManualInput) (*domain.TelegramSubscription, error) {
	chatID := strings.TrimSpace(in.ChatID)
	if chatID == "" && in.TelegramUserID != nil {
		chatID = fmt.Sprintf("%d", *in.TelegramUserID)
	}
	if chatID == "" {
		return nil, fmt.Errorf("telegram_user_id or chat_id is required")
	}

	now := time.Now().UTC()
	sub := domain.TelegramSubscription{
		ID:             ulid.Make().String(),
		TelegramUserID: in.TelegramUserID,
		ChatID:         chatID,
		ChatLabel:      strings.TrimSpace(in.ChatLabel),
		Status:         domain.SubscriptionStatusAccepted,
		AlertOrder:     in.AlertOrder,
		AlertProduct:   in.AlertProduct,
		CreatedAt:      now,
		UpdatedAt:      now,
		AcceptedAt:     &now,
		AcceptedBy:     strings.TrimSpace(in.AcceptedBy),
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO telegram_subscriptions (
			id, telegram_user_id, chat_id, chat_type, chat_label, username, status,
			alert_order, alert_product, created_at, updated_at, accepted_at, accepted_by
		) VALUES ($1,$2,$3,'','',$4,'accepted',$5,$6,$7,$8,$9,$10)
		ON CONFLICT (chat_id) DO UPDATE SET
			telegram_user_id = COALESCE(EXCLUDED.telegram_user_id, telegram_subscriptions.telegram_user_id),
			chat_label = CASE WHEN EXCLUDED.chat_label <> '' THEN EXCLUDED.chat_label ELSE telegram_subscriptions.chat_label END,
			status = 'accepted',
			alert_order = EXCLUDED.alert_order,
			alert_product = EXCLUDED.alert_product,
			updated_at = EXCLUDED.updated_at,
			accepted_at = EXCLUDED.accepted_at,
			accepted_by = EXCLUDED.accepted_by`,
		sub.ID, sub.TelegramUserID, sub.ChatID, sub.ChatLabel, sub.AlertOrder, sub.AlertProduct,
		sub.CreatedAt, sub.UpdatedAt, sub.AcceptedAt, sub.AcceptedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("create accepted telegram subscription: %w", err)
	}
	return r.FindByChatID(ctx, chatID)
}

func (r *TelegramRepository) Accept(ctx context.Context, id string, in ports.TelegramAcceptInput) (*domain.TelegramSubscription, error) {
	now := time.Now().UTC()
	tag, err := r.pool.Exec(ctx, `
		UPDATE telegram_subscriptions
		SET status = 'accepted', alert_order = $2, alert_product = $3,
		    accepted_at = $4, accepted_by = $5, updated_at = $4
		WHERE id = $1 AND status = 'pending'`,
		id, in.AlertOrder, in.AlertProduct, now, strings.TrimSpace(in.AcceptedBy),
	)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, pgx.ErrNoRows
	}
	return r.GetByID(ctx, id)
}

func (r *TelegramRepository) Reject(ctx context.Context, id, rejectedBy string) (*domain.TelegramSubscription, error) {
	now := time.Now().UTC()
	tag, err := r.pool.Exec(ctx, `
		UPDATE telegram_subscriptions
		SET status = 'rejected', updated_at = $2, accepted_by = $3
		WHERE id = $1 AND status = 'pending'`,
		id, now, strings.TrimSpace(rejectedBy),
	)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, pgx.ErrNoRows
	}
	return r.GetByID(ctx, id)
}

func (r *TelegramRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM telegram_subscriptions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *TelegramRepository) ListAccepted(ctx context.Context) ([]domain.TelegramSubscription, error) {
	return r.List(ctx, domain.SubscriptionStatusAccepted)
}

type scannable interface {
	Scan(dest ...any) error
}

func scanSubscription(row scannable) (*domain.TelegramSubscription, error) {
	var sub domain.TelegramSubscription
	var userID sql.NullInt64
	var acceptedAt sql.NullTime
	err := row.Scan(
		&sub.ID, &userID, &sub.ChatID, &sub.ChatType, &sub.ChatLabel, &sub.Username, &sub.Status,
		&sub.AlertOrder, &sub.AlertProduct, &sub.CreatedAt, &sub.UpdatedAt, &acceptedAt, &sub.AcceptedBy,
	)
	if err != nil {
		return nil, err
	}
	if userID.Valid {
		v := userID.Int64
		sub.TelegramUserID = &v
	}
	if acceptedAt.Valid {
		t := acceptedAt.Time
		sub.AcceptedAt = &t
	}
	return &sub, nil
}

func scanSubscriptions(rows pgx.Rows) ([]domain.TelegramSubscription, error) {
	var out []domain.TelegramSubscription
	for rows.Next() {
		var sub domain.TelegramSubscription
		var userID sql.NullInt64
		var acceptedAt sql.NullTime
		if err := rows.Scan(
			&sub.ID, &userID, &sub.ChatID, &sub.ChatType, &sub.ChatLabel, &sub.Username, &sub.Status,
			&sub.AlertOrder, &sub.AlertProduct, &sub.CreatedAt, &sub.UpdatedAt, &acceptedAt, &sub.AcceptedBy,
		); err != nil {
			return nil, err
		}
		if userID.Valid {
			v := userID.Int64
			sub.TelegramUserID = &v
		}
		if acceptedAt.Valid {
			t := acceptedAt.Time
			sub.AcceptedAt = &t
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func withPostgresSSLMode(connString string) string {
	if strings.Contains(connString, "sslmode=") {
		return connString
	}
	if strings.Contains(connString, "?") {
		return connString + "&sslmode=disable"
	}
	return connString + "?sslmode=disable"
}
