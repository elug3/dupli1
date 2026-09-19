package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/elug3/dupli1/support/pkg/domain"
)

// InquiryRepository stores escalations.
type InquiryRepository struct {
	db *sql.DB
}

func NewInquiryRepository(db *sql.DB) *InquiryRepository {
	return &InquiryRepository{db: db}
}

func (r *InquiryRepository) FindOpenByChatID(ctx context.Context, chatID string) (*domain.Inquiry, error) {
	const query = `
		SELECT id, conversation_id, chat_id, topic, status, COALESCE(assigned_to, ''), opened_at, closed_at
		  FROM support_inquiries
		 WHERE chat_id = $1 AND status <> $2
		 ORDER BY opened_at DESC
		 LIMIT 1`

	var inquiry domain.Inquiry
	err := r.db.QueryRowContext(ctx, query, chatID, domain.InquiryClosed).Scan(
		&inquiry.ID,
		&inquiry.ConversationID,
		&inquiry.ChatID,
		&inquiry.Topic,
		&inquiry.Status,
		&inquiry.AssignedTo,
		&inquiry.OpenedAt,
		&inquiry.ClosedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find open inquiry: %w", err)
	}
	return &inquiry, nil
}

func (r *InquiryRepository) Save(ctx context.Context, inquiry *domain.Inquiry) error {
	if inquiry == nil {
		return nil
	}
	const query = `
		INSERT INTO support_inquiries
			(id, conversation_id, chat_id, topic, status, assigned_to, opened_at, closed_at)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			status      = EXCLUDED.status,
			assigned_to = EXCLUDED.assigned_to,
			closed_at   = EXCLUDED.closed_at`

	_, err := r.db.ExecContext(ctx, query,
		inquiry.ID, inquiry.ConversationID, inquiry.ChatID, inquiry.Topic,
		inquiry.Status, inquiry.AssignedTo, inquiry.OpenedAt, inquiry.ClosedAt,
	)
	if err != nil {
		return fmt.Errorf("save inquiry: %w", err)
	}
	return nil
}

// MessageRepository stores the transcript.
type MessageRepository struct {
	db *sql.DB
}

func NewMessageRepository(db *sql.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

func (r *MessageRepository) Append(ctx context.Context, message *domain.Message) error {
	if message == nil {
		return nil
	}
	const query = `
		INSERT INTO support_messages
			(id, conversation_id, inquiry_id, direction, author, body, created_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), $6, $7)`

	_, err := r.db.ExecContext(ctx, query,
		message.ID, message.ConversationID, message.InquiryID,
		message.Direction, message.Author, message.Body, message.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("append support message: %w", err)
	}
	return nil
}

// LastInbound returns the most recent thing the shopper typed.
func (r *MessageRepository) LastInbound(ctx context.Context, conversationID string) (string, error) {
	const query = `
		SELECT body FROM support_messages
		 WHERE conversation_id = $1 AND direction = $2
		 ORDER BY created_at DESC
		 LIMIT 1`

	var body string
	err := r.db.QueryRowContext(ctx, query, conversationID, domain.DirectionInbound).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("last inbound message: %w", err)
	}
	return body, nil
}
