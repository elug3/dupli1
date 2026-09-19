// Package postgres stores conversations and canned answers in PostgreSQL.
//
// Schema is migrated inline on startup, as every service in this repo does:
// CREATE TABLE IF NOT EXISTS plus additive ADD COLUMN IF NOT EXISTS. Breaking
// changes are not supported this way, which is why support_answers keys on
// (node, language) from the start even though only Korean rows ship.
package postgres

import (
	"database/sql"
	"fmt"

	"github.com/elug3/dupli1/shared/pkg/pgsslmode"

	_ "github.com/lib/pq"
)

// Open connects and applies the support schema.
func Open(connString string) (*sql.DB, error) {
	db, err := sql.Open("postgres", pgsslmode.WithSSLMode(connString))
	if err != nil {
		return nil, fmt.Errorf("open support database: %w", err)
	}
	if err := Migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// Migrate creates the support schema if it is absent.
func Migrate(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS support_conversations (
			id                TEXT PRIMARY KEY,
			chat_id           TEXT NOT NULL UNIQUE,
			telegram_user_id  BIGINT,
			username          TEXT,
			language          TEXT NOT NULL DEFAULT 'ko',
			node              TEXT NOT NULL DEFAULT 'root',
			entry_payload     TEXT,
			last_seen_at      TIMESTAMPTZ NOT NULL,
			created_at        TIMESTAMPTZ NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS support_conversations_last_seen_idx
		 ON support_conversations (last_seen_at DESC)`,
		`CREATE TABLE IF NOT EXISTS support_inquiries (
			id               TEXT PRIMARY KEY,
			conversation_id  TEXT NOT NULL REFERENCES support_conversations(id),
			chat_id          TEXT NOT NULL,
			topic            TEXT NOT NULL,
			status           TEXT NOT NULL,
			assigned_to      TEXT,
			opened_at        TIMESTAMPTZ NOT NULL,
			closed_at        TIMESTAMPTZ
		)`,
		// One open inquiry per chat, enforced by the database rather than by
		// the read-then-write in the router: two taps of "상담원 연결" landing
		// together would otherwise queue the same shopper twice.
		`CREATE UNIQUE INDEX IF NOT EXISTS support_inquiries_one_open_per_chat
		 ON support_inquiries (chat_id) WHERE status <> 'closed'`,
		`CREATE INDEX IF NOT EXISTS support_inquiries_status_idx
		 ON support_inquiries (status, opened_at DESC)`,
		`CREATE TABLE IF NOT EXISTS support_messages (
			id               TEXT PRIMARY KEY,
			conversation_id  TEXT NOT NULL REFERENCES support_conversations(id),
			inquiry_id       TEXT REFERENCES support_inquiries(id),
			direction        TEXT NOT NULL,
			author           TEXT,
			body             TEXT NOT NULL,
			created_at       TIMESTAMPTZ NOT NULL
		)`,
		// Additive, per this repo's inline-migration convention: delivery
		// outcomes arrived with the manager inbox.
		`ALTER TABLE support_messages ADD COLUMN IF NOT EXISTS delivery TEXT`,
		`ALTER TABLE support_messages ADD COLUMN IF NOT EXISTS delivery_error TEXT`,
		`CREATE INDEX IF NOT EXISTS support_messages_conversation_idx
		 ON support_messages (conversation_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS support_answers (
			node        TEXT NOT NULL,
			language    TEXT NOT NULL DEFAULT 'ko',
			body        TEXT NOT NULL,
			updated_by  TEXT,
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (node, language)
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("migrate support schema: %w", err)
		}
	}
	return nil
}
