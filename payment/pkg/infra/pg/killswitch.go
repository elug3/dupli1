package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
)

// payment_kill_switches holds one row per switch an operator has flipped.
// Kind and target are not stored: they come from domain.KillSwitchDefs by key.
const killSwitchTableDDL = `CREATE TABLE IF NOT EXISTS payment_kill_switches (
	key        TEXT PRIMARY KEY,
	engaged    BOOLEAN NOT NULL DEFAULT FALSE,
	message    TEXT NOT NULL DEFAULT '',
	reason     TEXT NOT NULL DEFAULT '',
	updated_by TEXT NOT NULL DEFAULT '',
	updated_at TIMESTAMPTZ NOT NULL
)`

func (r *Repository) ListKillSwitches(ctx context.Context) ([]domain.KillSwitch, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT key, engaged, message, reason, updated_by, updated_at
		FROM payment_kill_switches
	`)
	if err != nil {
		return nil, fmt.Errorf("list kill switches: %w", err)
	}
	defer rows.Close()
	var out []domain.KillSwitch
	for rows.Next() {
		var sw domain.KillSwitch
		var at time.Time
		if err := rows.Scan(&sw.Key, &sw.Engaged, &sw.Message, &sw.Reason, &sw.UpdatedBy, &at); err != nil {
			return nil, fmt.Errorf("scan kill switch: %w", err)
		}
		at = at.UTC()
		sw.UpdatedAt = &at
		out = append(out, sw)
	}
	return out, rows.Err()
}

func (r *Repository) SaveKillSwitch(ctx context.Context, sw domain.KillSwitch) error {
	at := time.Now().UTC()
	if sw.UpdatedAt != nil {
		at = *sw.UpdatedAt
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO payment_kill_switches (key, engaged, message, reason, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (key) DO UPDATE SET
			engaged = EXCLUDED.engaged,
			message = EXCLUDED.message,
			reason = EXCLUDED.reason,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
	`, sw.Key, sw.Engaged, sw.Message, sw.Reason, sw.UpdatedBy, at)
	if err != nil {
		return fmt.Errorf("save kill switch: %w", err)
	}
	return nil
}
