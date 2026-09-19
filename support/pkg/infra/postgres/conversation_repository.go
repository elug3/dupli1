package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/elug3/dupli1/support/pkg/domain"
)

// ConversationRepository stores one row per Telegram chat.
type ConversationRepository struct {
	db *sql.DB
}

func NewConversationRepository(db *sql.DB) *ConversationRepository {
	return &ConversationRepository{db: db}
}

// FindByID looks a conversation up by its own id, which is what an inquiry
// holds.
func (r *ConversationRepository) FindByID(ctx context.Context, id string) (*domain.Conversation, error) {
	return r.findBy(ctx, `id = $1`, id)
}

func (r *ConversationRepository) FindByChatID(ctx context.Context, chatID string) (*domain.Conversation, error) {
	return r.findBy(ctx, `chat_id = $1`, chatID)
}

func (r *ConversationRepository) findBy(ctx context.Context, where string, arg any) (*domain.Conversation, error) {
	query := `
		SELECT id, chat_id, telegram_user_id, username, language, node,
		       COALESCE(entry_payload, ''), last_seen_at, created_at
		  FROM support_conversations
		 WHERE ` + where

	var (
		conversation domain.Conversation
		userID       sql.NullInt64
		username     sql.NullString
	)
	err := r.db.QueryRowContext(ctx, query, arg).Scan(
		&conversation.ID,
		&conversation.ChatID,
		&userID,
		&username,
		&conversation.Language,
		&conversation.Node,
		&conversation.EntryPayload,
		&conversation.LastSeenAt,
		&conversation.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		// An unknown chat is not an error: it is a shopper who has not written
		// before, which is the common case on a storefront.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find support conversation: %w", err)
	}
	if userID.Valid {
		id := userID.Int64
		conversation.TelegramUserID = &id
	}
	conversation.Username = username.String
	return &conversation, nil
}

// Save inserts or updates the chat's row.
//
// Conflict is on chat_id rather than id: two updates racing for a chat that has
// never written before would otherwise insert two rows with different ULIDs for
// the same conversation, and Telegram can deliver a message and a button tap
// close enough together for that to happen.
func (r *ConversationRepository) Save(ctx context.Context, conversation *domain.Conversation) error {
	if conversation == nil {
		return nil
	}
	const query = `
		INSERT INTO support_conversations
			(id, chat_id, telegram_user_id, username, language, node, entry_payload, last_seen_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (chat_id) DO UPDATE SET
			telegram_user_id = COALESCE(EXCLUDED.telegram_user_id, support_conversations.telegram_user_id),
			username         = COALESCE(NULLIF(EXCLUDED.username, ''), support_conversations.username),
			language         = EXCLUDED.language,
			node             = EXCLUDED.node,
			last_seen_at     = EXCLUDED.last_seen_at`

	var userID any
	if conversation.TelegramUserID != nil {
		userID = *conversation.TelegramUserID
	}
	_, err := r.db.ExecContext(ctx, query,
		conversation.ID,
		conversation.ChatID,
		userID,
		conversation.Username,
		conversation.Language,
		conversation.Node,
		conversation.EntryPayload,
		conversation.LastSeenAt,
		conversation.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save support conversation: %w", err)
	}
	return nil
}
