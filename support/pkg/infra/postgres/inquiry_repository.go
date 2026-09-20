package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/elug3/dupli1/support/pkg/ports"

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

func (r *InquiryRepository) FindByID(ctx context.Context, id string) (*domain.Inquiry, error) {
	const query = `
		SELECT id, conversation_id, chat_id, topic, status, COALESCE(assigned_to, ''), opened_at, closed_at
		  FROM support_inquiries WHERE id = $1`

	var inquiry domain.Inquiry
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&inquiry.ID, &inquiry.ConversationID, &inquiry.ChatID, &inquiry.Topic,
		&inquiry.Status, &inquiry.AssignedTo, &inquiry.OpenedAt, &inquiry.ClosedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find inquiry: %w", err)
	}
	return &inquiry, nil
}

// List returns inquiries for the inbox, newest first.
func (r *InquiryRepository) List(ctx context.Context, filter ports.InquiryFilter) ([]domain.Inquiry, error) {
	query := `
		SELECT id, conversation_id, chat_id, topic, status, COALESCE(assigned_to, ''), opened_at, closed_at
		  FROM support_inquiries
		 WHERE ($1 = '' OR status = $1)
		   AND ($2 = '' OR assigned_to = $2)
		   AND (NOT $3::boolean OR (assigned_to IS NULL AND status <> 'closed'))
		 ORDER BY opened_at DESC
		 LIMIT $4`

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, query, filter.Status, filter.AssignedTo, filter.Unassigned, limit)
	if err != nil {
		return nil, fmt.Errorf("list inquiries: %w", err)
	}
	defer rows.Close()

	return scanInquiries(rows)
}

// ListStaleOpen returns unfinished inquiries with no message since the cutoff.
//
// Quiet is measured from the last message, not from when the inquiry opened: a
// consultation still being answered has not gone stale however long it runs.
func (r *InquiryRepository) ListStaleOpen(ctx context.Context, quietSince time.Time) ([]domain.Inquiry, error) {
	const query = `
		SELECT i.id, i.conversation_id, i.chat_id, i.topic, i.status,
		       COALESCE(i.assigned_to, ''), i.opened_at, i.closed_at
		  FROM support_inquiries i
		 WHERE i.status <> 'closed'
		   AND GREATEST(
		         i.opened_at,
		         COALESCE((SELECT max(m.created_at) FROM support_messages m WHERE m.inquiry_id = i.id), i.opened_at)
		       ) < $1
		 ORDER BY i.opened_at ASC`

	rows, err := r.db.QueryContext(ctx, query, quietSince)
	if err != nil {
		return nil, fmt.Errorf("list stale inquiries: %w", err)
	}
	defer rows.Close()

	return scanInquiries(rows)
}

func scanInquiries(rows *sql.Rows) ([]domain.Inquiry, error) {
	var out []domain.Inquiry
	for rows.Next() {
		var inquiry domain.Inquiry
		err := rows.Scan(
			&inquiry.ID, &inquiry.ConversationID, &inquiry.ChatID, &inquiry.Topic,
			&inquiry.Status, &inquiry.AssignedTo, &inquiry.OpenedAt, &inquiry.ClosedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan inquiry: %w", err)
		}
		out = append(out, inquiry)
	}
	return out, rows.Err()
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
			(id, conversation_id, inquiry_id, direction, author, body, delivery, delivery_error, created_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), $6, NULLIF($7, ''), NULLIF($8, ''), $9)`

	_, err := r.db.ExecContext(ctx, query,
		message.ID, message.ConversationID, message.InquiryID,
		message.Direction, message.Author, message.Body,
		message.Delivery, message.DeliveryError, message.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("append support message: %w", err)
	}
	return nil
}

// Transcript returns a conversation's messages, oldest first.
func (r *MessageRepository) Transcript(ctx context.Context, conversationID string) ([]domain.Message, error) {
	const query = `
		SELECT id, conversation_id, COALESCE(inquiry_id, ''), direction, COALESCE(author, ''),
		       body, COALESCE(delivery, ''), COALESCE(delivery_error, ''), created_at
		  FROM support_messages
		 WHERE conversation_id = $1
		 ORDER BY created_at ASC`

	rows, err := r.db.QueryContext(ctx, query, conversationID)
	if err != nil {
		return nil, fmt.Errorf("load transcript: %w", err)
	}
	defer rows.Close()

	var out []domain.Message
	for rows.Next() {
		var message domain.Message
		err := rows.Scan(
			&message.ID, &message.ConversationID, &message.InquiryID, &message.Direction,
			&message.Author, &message.Body, &message.Delivery, &message.DeliveryError, &message.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		out = append(out, message)
	}
	return out, rows.Err()
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

// PurgeBodies replaces the text of messages older than the cutoff.
//
// An UPDATE rather than a DELETE: the inquiry keeps its shape — how many
// messages, when, from whom — while the words, which are customer data, go.
// Rows already purged are skipped so a daily sweep does not rewrite history it
// has already handled.
func (r *MessageRepository) PurgeBodies(ctx context.Context, olderThan time.Time, placeholder string) (int, error) {
	const query = `
		UPDATE support_messages
		   SET body = $1, delivery_error = NULL
		 WHERE created_at < $2
		   AND body <> $1`

	result, err := r.db.ExecContext(ctx, query, placeholder, olderThan)
	if err != nil {
		return 0, fmt.Errorf("purge support message bodies: %w", err)
	}
	purged, err := result.RowsAffected()
	if err != nil {
		// The purge itself succeeded; only the count is unavailable. Reporting
		// a failure here would make the caller log an error for work that was
		// done, and retry nothing, since the rows are already rewritten.
		return 0, nil
	}
	return int(purged), nil
}
