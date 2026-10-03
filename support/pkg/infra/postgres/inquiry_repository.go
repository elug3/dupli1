package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

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
	query := `
		SELECT ` + inquiryColumns + `
		  FROM support_inquiries
		 WHERE chat_id = $1 AND status <> $2
		 ORDER BY opened_at DESC
		 LIMIT 1`

	inquiry, err := scanInquiry(r.db.QueryRowContext(ctx, query, chatID, domain.InquiryClosed))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find open inquiry: %w", err)
	}
	return inquiry, nil
}

func (r *InquiryRepository) FindByID(ctx context.Context, id string) (*domain.Inquiry, error) {
	query := `SELECT ` + inquiryColumns + ` FROM support_inquiries WHERE id = $1`

	inquiry, err := scanInquiry(r.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find inquiry: %w", err)
	}
	return inquiry, nil
}

// List returns inquiries for the inbox, newest first.
func (r *InquiryRepository) List(ctx context.Context, filter ports.InquiryFilter) ([]domain.Inquiry, error) {
	query := `
		SELECT ` + inquiryColumns + `
		  FROM support_inquiries
		 WHERE ($1 = '' OR status = $1)
		   AND ($2 = '' OR assigned_to = $2)
		   AND (NOT $3::boolean OR (assigned_to IS NULL AND status <> 'closed'))
		   AND ($5 = '' OR channel = $5)
		 ORDER BY opened_at DESC
		 LIMIT $4`

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, query, filter.Status, filter.AssignedTo, filter.Unassigned, limit, filter.Channel)
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
	query := `
		SELECT ` + inquiryColumnsAs("i") + `
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

// inquiryColumns is the select list scanInquiry reads, in its order.
var inquiryColumns = inquiryColumnsAs("")

func inquiryColumnsAs(alias string) string {
	p := ""
	if alias != "" {
		p = alias + "."
	}
	return p + "id, " + p + "conversation_id, " + p + "chat_id, " + p + "topic, " + p + "status, " +
		"COALESCE(" + p + "assigned_to, ''), " + p + "opened_at, " + p + "closed_at, " +
		p + "channel, COALESCE(" + p + "customer_id, ''), COALESCE(" + p + "product_id, ''), " +
		"COALESCE(" + p + "sku_id, ''), COALESCE(" + p + "order_id, '')"
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanInquiry(row rowScanner) (*domain.Inquiry, error) {
	var inquiry domain.Inquiry
	err := row.Scan(
		&inquiry.ID, &inquiry.ConversationID, &inquiry.ChatID, &inquiry.Topic,
		&inquiry.Status, &inquiry.AssignedTo, &inquiry.OpenedAt, &inquiry.ClosedAt,
		&inquiry.Channel, &inquiry.CustomerID, &inquiry.ProductID, &inquiry.SkuID, &inquiry.OrderID,
	)
	if err != nil {
		return nil, err
	}
	return &inquiry, nil
}

func scanInquiries(rows *sql.Rows) ([]domain.Inquiry, error) {
	var out []domain.Inquiry
	for rows.Next() {
		inquiry, err := scanInquiry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan inquiry: %w", err)
		}
		out = append(out, *inquiry)
	}
	return out, rows.Err()
}

func (r *InquiryRepository) Save(ctx context.Context, inquiry *domain.Inquiry) error {
	if inquiry == nil {
		return nil
	}
	const query = `
		INSERT INTO support_inquiries
			(id, conversation_id, chat_id, topic, status, assigned_to, opened_at, closed_at,
			 channel, customer_id, product_id, sku_id, order_id)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8,
			COALESCE(NULLIF($9, ''), 'telegram'), NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''))
		ON CONFLICT (id) DO UPDATE SET
			status      = EXCLUDED.status,
			assigned_to = EXCLUDED.assigned_to,
			closed_at   = EXCLUDED.closed_at`

	_, err := r.db.ExecContext(ctx, query,
		inquiry.ID, inquiry.ConversationID, inquiry.ChatID, inquiry.Topic,
		inquiry.Status, inquiry.AssignedTo, inquiry.OpenedAt, inquiry.ClosedAt,
		inquiry.Channel, inquiry.CustomerID, inquiry.ProductID, inquiry.SkuID, inquiry.OrderID,
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
			(id, conversation_id, inquiry_id, direction, author, body, delivery, delivery_error, created_at,
			 kind, ref_id, ref_snapshot, notice_status)
		VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), $6, NULLIF($7, ''), NULLIF($8, ''), $9,
			COALESCE(NULLIF($10, ''), 'text'), NULLIF($11, ''), $12, NULLIF($13, ''))`

	var snapshot any
	if len(message.RefSnapshot) > 0 {
		snapshot = string(message.RefSnapshot)
	}
	_, err := r.db.ExecContext(ctx, query,
		message.ID, message.ConversationID, message.InquiryID,
		message.Direction, message.Author, message.Body,
		message.Delivery, message.DeliveryError, message.CreatedAt,
		message.Kind, message.RefID, snapshot, message.NoticeStatus,
	)
	if err != nil {
		return fmt.Errorf("append support message: %w", err)
	}
	return nil
}

// Transcript returns a conversation's messages, oldest first.
func (r *MessageRepository) Transcript(ctx context.Context, conversationID string) ([]domain.Message, error) {
	query := `
		SELECT ` + messageColumns + `
		  FROM support_messages
		 WHERE conversation_id = $1
		 ORDER BY created_at ASC, id ASC`

	rows, err := r.db.QueryContext(ctx, query, conversationID)
	if err != nil {
		return nil, fmt.Errorf("load transcript: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// messageColumns is the select list scanMessages reads, in its order.
const messageColumns = `id, conversation_id, COALESCE(inquiry_id, ''), direction, COALESCE(author, ''),
	body, COALESCE(delivery, ''), COALESCE(delivery_error, ''), created_at,
	kind, COALESCE(ref_id, ''), ref_snapshot, COALESCE(notice_status, '')`

func scanMessages(rows *sql.Rows) ([]domain.Message, error) {
	var out []domain.Message
	for rows.Next() {
		var (
			message  domain.Message
			snapshot []byte
		)
		err := rows.Scan(
			&message.ID, &message.ConversationID, &message.InquiryID, &message.Direction,
			&message.Author, &message.Body, &message.Delivery, &message.DeliveryError, &message.CreatedAt,
			&message.Kind, &message.RefID, &snapshot, &message.NoticeStatus,
		)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		if len(snapshot) > 0 {
			message.RefSnapshot = snapshot
		}
		out = append(out, message)
	}
	return out, rows.Err()
}

// PurgeConversation replaces the text of every message in one conversation,
// for an account that was deleted.
func (r *MessageRepository) PurgeConversation(ctx context.Context, conversationID, placeholder string) (int, error) {
	const query = `
		UPDATE support_messages
		   SET body = $1, delivery_error = NULL, ref_snapshot = NULL
		 WHERE conversation_id = $2
		   AND body <> $1`

	result, err := r.db.ExecContext(ctx, query, placeholder, conversationID)
	if err != nil {
		return 0, fmt.Errorf("purge support conversation: %w", err)
	}
	purged, err := result.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return int(purged), nil
}

// DueNotices returns messages whose reply notice is still pending.
func (r *MessageRepository) DueNotices(ctx context.Context, createdBefore time.Time, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
		SELECT ` + messageColumns + `
		  FROM support_messages
		 WHERE notice_status = $1 AND created_at < $2
		 ORDER BY created_at ASC
		 LIMIT $3`

	rows, err := r.db.QueryContext(ctx, query, domain.NoticePending, createdBefore, limit)
	if err != nil {
		return nil, fmt.Errorf("list due notices: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// SetNoticeStatus records the notice outcome on the given messages.
func (r *MessageRepository) SetNoticeStatus(ctx context.Context, messageIDs []string, status string) error {
	if len(messageIDs) == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE support_messages SET notice_status = $1 WHERE id = ANY($2)`,
		status, pq.Array(messageIDs))
	if err != nil {
		return fmt.Errorf("set notice status: %w", err)
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

// PurgeBodies replaces the text of messages older than the cutoff.
//
// An UPDATE rather than a DELETE: the inquiry keeps its shape — how many
// messages, when, from whom — while the words, which are customer data, go.
// Rows already purged are skipped so a daily sweep does not rewrite history it
// has already handled.
func (r *MessageRepository) PurgeBodies(ctx context.Context, olderThan time.Time, placeholder string) (int, error) {
	const query = `
		UPDATE support_messages
		   SET body = $1, delivery_error = NULL, ref_snapshot = NULL
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
