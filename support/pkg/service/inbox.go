package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/ports"
)

// ErrInquiryNotFound is returned when an id matches nothing.
var ErrInquiryNotFound = errors.New("inquiry not found")

// ErrUndeliverable reports that a reply was stored but never reached the
// shopper — they blocked the bot, or deleted the chat.
//
// A separate error because it is not a failure of the manager's action: the
// reply exists and is on the record. What must not happen is the console
// reporting success for a message nobody received.
var ErrUndeliverable = errors.New("reply could not be delivered")

// Inbox is the manager-facing side of a consultation.
type Inbox struct {
	conversations ports.ConversationRepository
	inquiries     ports.InquiryRepository
	messages      ports.MessageRepository
	bot           ports.Bot
	newID         IDGenerator
	now           Clock
}

func NewInbox(
	conversations ports.ConversationRepository,
	inquiries ports.InquiryRepository,
	messages ports.MessageRepository,
	bot ports.Bot,
	newID IDGenerator,
	now Clock,
) *Inbox {
	if newID == nil {
		newID = func() string { return "" }
	}
	if now == nil {
		now = time.Now
	}
	return &Inbox{
		conversations: conversations,
		inquiries:     inquiries,
		messages:      messages,
		bot:           bot,
		newID:         newID,
		now:           now,
	}
}

// InquiryView is an inquiry as the console shows it.
type InquiryView struct {
	domain.Inquiry
	Language     string
	Username     string
	EntryPayload string
	// LastMessage is the most recent line, for the list.
	LastMessage string
	Transcript  []domain.Message
}

// List returns one of the inbox's lists.
func (i *Inbox) List(ctx context.Context, filter ports.InquiryFilter) ([]InquiryView, error) {
	rows, err := i.inquiries.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list inquiries: %w", err)
	}

	views := make([]InquiryView, 0, len(rows))
	for _, row := range rows {
		view := InquiryView{Inquiry: row}
		if conversation, err := i.conversations.FindByID(ctx, row.ConversationID); err == nil && conversation != nil {
			view.Language = conversation.Language
			view.Username = conversation.Username
			view.EntryPayload = conversation.EntryPayload
		}
		if transcript, err := i.messages.Transcript(ctx, row.ConversationID); err == nil && len(transcript) > 0 {
			view.LastMessage = domain.Excerpt(transcript[len(transcript)-1].Body, 120)
		}
		views = append(views, view)
	}
	return views, nil
}

// Get returns one inquiry with its whole transcript.
func (i *Inbox) Get(ctx context.Context, id string) (*InquiryView, error) {
	inquiry, err := i.inquiries.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find inquiry: %w", err)
	}
	if inquiry == nil {
		return nil, ErrInquiryNotFound
	}

	view := &InquiryView{Inquiry: *inquiry}
	conversation, err := i.conversations.FindByID(ctx, inquiry.ConversationID)
	if err == nil && conversation != nil {
		view.Language = conversation.Language
		view.Username = conversation.Username
		view.EntryPayload = conversation.EntryPayload
	}
	transcript, err := i.messages.Transcript(ctx, inquiry.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("load transcript: %w", err)
	}
	view.Transcript = transcript
	return view, nil
}

// Claim assigns an inquiry to a manager.
//
// A claim is visible, not exclusive: anyone with support.reply may take over,
// and the takeover is recorded rather than blocked. A hard lock strands
// inquiries when a shift ends mid-conversation.
func (i *Inbox) Claim(ctx context.Context, id, managerID string) (*InquiryView, error) {
	inquiry, err := i.inquiries.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find inquiry: %w", err)
	}
	if inquiry == nil {
		return nil, ErrInquiryNotFound
	}
	if err := i.assign(ctx, inquiry, managerID); err != nil {
		return nil, err
	}
	return i.Get(ctx, id)
}

func (i *Inbox) assign(ctx context.Context, inquiry *domain.Inquiry, managerID string) error {
	managerID = strings.TrimSpace(managerID)
	if managerID == "" {
		return fmt.Errorf("manager id is required")
	}
	if inquiry.AssignedTo == managerID && inquiry.Status == domain.InquiryAssigned {
		return nil
	}
	inquiry.AssignedTo = managerID
	if inquiry.Status == domain.InquiryOpen {
		inquiry.Status = domain.InquiryAssigned
	}
	if err := i.inquiries.Save(ctx, inquiry); err != nil {
		return fmt.Errorf("save inquiry: %w", err)
	}
	return nil
}

// Reply sends a manager's words to the shopper and records them.
//
// Replying claims the inquiry: a manager who opens an unclaimed one and simply
// answers should not have to press a button first.
//
// The message is stored whatever Telegram says. If the shopper has blocked the
// bot, the row is marked failed and ErrUndeliverable comes back, so the console
// can show 미전송 instead of a success it cannot justify.
func (i *Inbox) Reply(ctx context.Context, id, managerID, body string) (*InquiryView, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("reply body is required")
	}

	inquiry, err := i.inquiries.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find inquiry: %w", err)
	}
	if inquiry == nil {
		return nil, ErrInquiryNotFound
	}
	if err := i.assign(ctx, inquiry, managerID); err != nil {
		return nil, err
	}

	message := &domain.Message{
		ID:             i.newID(),
		ConversationID: inquiry.ConversationID,
		InquiryID:      inquiry.ID,
		Direction:      domain.DirectionOutbound,
		Author:         managerID,
		Body:           body,
		Delivery:       domain.DeliverySent,
		CreatedAt:      i.now(),
	}

	sendErr := i.bot.Reply(ctx, inquiry.ChatID, body)
	if sendErr != nil {
		message.Delivery = domain.DeliveryFailed
		message.DeliveryError = sendErr.Error()
	}
	if err := i.messages.Append(ctx, message); err != nil {
		return nil, fmt.Errorf("record reply: %w", err)
	}

	if inquiry.Status == domain.InquiryAssigned && sendErr == nil {
		inquiry.Status = domain.InquiryAnswered
		if err := i.inquiries.Save(ctx, inquiry); err != nil {
			return nil, fmt.Errorf("save inquiry: %w", err)
		}
	}

	view, err := i.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if sendErr != nil {
		return view, fmt.Errorf("%w: %v", ErrUndeliverable, sendErr)
	}
	return view, nil
}

// Close finishes an inquiry, freeing the chat for a later consultation.
func (i *Inbox) Close(ctx context.Context, id, managerID string) (*InquiryView, error) {
	inquiry, err := i.inquiries.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find inquiry: %w", err)
	}
	if inquiry == nil {
		return nil, ErrInquiryNotFound
	}
	if inquiry.Status == domain.InquiryClosed {
		return i.Get(ctx, id)
	}

	closed := i.now()
	inquiry.Status = domain.InquiryClosed
	inquiry.ClosedAt = &closed
	if manager := strings.TrimSpace(managerID); manager != "" && inquiry.AssignedTo == "" {
		inquiry.AssignedTo = manager
	}
	if err := i.inquiries.Save(ctx, inquiry); err != nil {
		return nil, fmt.Errorf("save inquiry: %w", err)
	}
	return i.Get(ctx, id)
}

// CloseStale finishes inquiries nobody has touched for quietFor, so the queue
// reflects live work rather than history. Returns how many were closed.
func (i *Inbox) CloseStale(ctx context.Context, quietFor time.Duration) (int, error) {
	cutoff := i.now().Add(-quietFor)
	stale, err := i.inquiries.ListStaleOpen(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("list stale inquiries: %w", err)
	}

	closed := 0
	var errs []error
	for idx := range stale {
		inquiry := stale[idx]
		at := i.now()
		inquiry.Status = domain.InquiryClosed
		inquiry.ClosedAt = &at
		if err := i.inquiries.Save(ctx, &inquiry); err != nil {
			errs = append(errs, err)
			continue
		}
		closed++
	}
	return closed, errors.Join(errs...)
}
