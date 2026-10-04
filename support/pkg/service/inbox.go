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

	// Web chat collaborators, set with WithWebChat. A Telegram-only inbox
	// runs without them.
	products ports.ProductReader
	orders   ports.OrderReader
	live     ports.LivePublisher
}

// WithWebChat gives the inbox what web consultations need: catalog and order
// lookups for the reference cards a manager attaches, and the live feed that
// pushes a reply to the shopper's open panel and the inbox's own stream.
func (i *Inbox) WithWebChat(products ports.ProductReader, orders ports.OrderReader, live ports.LivePublisher) *Inbox {
	i.products, i.orders, i.live = products, orders, live
	return i
}

// ErrReferenceOnTelegram refuses a reference card on a Telegram inquiry: the
// bot can only send text, and a card it cannot render must not be recorded
// as sent.
var ErrReferenceOnTelegram = errors.New("reference cards can only be sent in web consultations")

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
	// CustomerEmail and CustomerLastReadAt are set for web inquiries: who
	// the shopper is, and how far they have read (the console's 읽음).
	CustomerEmail      string
	CustomerLastReadAt *time.Time
}

func (v *InquiryView) fill(conversation *domain.Conversation) {
	if conversation == nil {
		return
	}
	v.Language = conversation.Language
	v.Username = conversation.Username
	v.EntryPayload = conversation.EntryPayload
	v.CustomerEmail = conversation.CustomerEmail
	v.CustomerLastReadAt = conversation.CustomerLastReadAt
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
		if conversation, err := i.conversations.FindByID(ctx, row.ConversationID); err == nil {
			view.fill(conversation)
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
	if conversation, err := i.conversations.FindByID(ctx, inquiry.ConversationID); err == nil {
		view.fill(conversation)
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
	i.publishInquiry(ctx, inquiry)
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

// ReplyInput is one manager reply: text and, in a web consultation, an
// optional product (by SKU) or order reference card.
type ReplyInput struct {
	InquiryID string
	ManagerID string
	Body      string
	// Bearer is the manager's own token, forwarded to order to read an order
	// reference under the manager's order.read.all.
	Bearer  string
	SkuID   string
	OrderID string
}

// Reply sends a manager's words to the shopper and records them.
func (i *Inbox) Reply(ctx context.Context, id, managerID, body string) (*InquiryView, error) {
	return i.ReplyWith(ctx, ReplyInput{InquiryID: id, ManagerID: managerID, Body: body})
}

// ReplyWith sends a reply, with any reference cards.
//
// Replying claims the inquiry: a manager who opens an unclaimed one and simply
// answers should not have to press a button first.
//
// On Telegram the message is stored whatever the Bot API says. If the shopper
// has blocked the bot, the row is marked failed and ErrUndeliverable comes
// back, so the console can show 미전송 instead of a success it cannot justify.
// On the web there is no third party to refuse: the reply is stored, pushed
// to the shopper's panel if it is open, and left for the reply-notice job.
func (i *Inbox) ReplyWith(ctx context.Context, in ReplyInput) (*InquiryView, error) {
	body := strings.TrimSpace(in.Body)
	hasRef := strings.TrimSpace(in.SkuID) != "" || strings.TrimSpace(in.OrderID) != ""
	if body == "" && !hasRef {
		return nil, fmt.Errorf("%w: reply body is required", ErrInvalidMessage)
	}

	inquiry, err := i.inquiries.FindByID(ctx, in.InquiryID)
	if err != nil {
		return nil, fmt.Errorf("find inquiry: %w", err)
	}
	if inquiry == nil {
		return nil, ErrInquiryNotFound
	}
	if !inquiry.IsWeb() {
		if hasRef {
			return nil, ErrReferenceOnTelegram
		}
		return i.replyTelegram(ctx, inquiry, in.ManagerID, body)
	}
	if body != "" && !domain.ValidWebBody(body) {
		return nil, ErrInvalidMessage
	}

	refs, err := resolveRefs(ctx, i.products, i.orders, in.Bearer, "", in.SkuID, in.OrderID)
	if err != nil {
		return nil, err
	}
	if err := i.assign(ctx, inquiry, in.ManagerID); err != nil {
		return nil, err
	}

	now := i.now()
	outgoing := refs.messages(i.newID, inquiry.ConversationID, inquiry.ID, domain.DirectionOutbound, in.ManagerID, now)
	if body != "" {
		outgoing = append(outgoing, domain.Message{
			ID: i.newID(), ConversationID: inquiry.ConversationID, InquiryID: inquiry.ID,
			Direction: domain.DirectionOutbound, Author: in.ManagerID, Kind: domain.MessageText,
			Body: body, CreatedAt: now,
		})
	}
	for idx := range outgoing {
		outgoing[idx].Delivery = domain.DeliverySent
		outgoing[idx].NoticeStatus = domain.NoticePending
		if err := i.messages.Append(ctx, &outgoing[idx]); err != nil {
			return nil, fmt.Errorf("record reply: %w", err)
		}
	}

	if inquiry.Status == domain.InquiryAssigned {
		inquiry.Status = domain.InquiryAnswered
		if err := i.inquiries.Save(ctx, inquiry); err != nil {
			return nil, fmt.Errorf("save inquiry: %w", err)
		}
	}
	if len(outgoing) > 0 {
		i.publish(ctx, ports.LiveEvent{
			Type: ports.LiveMessage, InquiryID: inquiry.ID, ConversationID: inquiry.ConversationID,
			CustomerID: inquiry.CustomerID, Channel: domain.ChannelWeb,
			MessageID: outgoing[len(outgoing)-1].ID,
		})
	}
	return i.Get(ctx, inquiry.ID)
}

func (i *Inbox) replyTelegram(ctx context.Context, inquiry *domain.Inquiry, managerID, body string) (*InquiryView, error) {
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
	i.publish(ctx, ports.LiveEvent{
		Type: ports.LiveMessage, InquiryID: inquiry.ID, ConversationID: inquiry.ConversationID,
		Channel: domain.ChannelTelegram, MessageID: message.ID,
	})

	view, err := i.Get(ctx, inquiry.ID)
	if err != nil {
		return nil, err
	}
	if sendErr != nil {
		return view, fmt.Errorf("%w: %v", ErrUndeliverable, sendErr)
	}
	return view, nil
}

func (i *Inbox) publish(ctx context.Context, event ports.LiveEvent) {
	if i.live != nil {
		i.live.PublishLive(ctx, event)
	}
}

func (i *Inbox) publishInquiry(ctx context.Context, inquiry *domain.Inquiry) {
	i.publish(ctx, ports.LiveEvent{
		Type: ports.LiveInquiry, InquiryID: inquiry.ID, ConversationID: inquiry.ConversationID,
		CustomerID: inquiry.CustomerID, Channel: domain.ChannelOf(inquiry.Channel), Status: inquiry.Status,
	})
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

	if manager := strings.TrimSpace(managerID); manager != "" && inquiry.AssignedTo == "" {
		inquiry.AssignedTo = manager
	}
	closed := i.now()
	if inquiry.IsWeb() {
		// The shopper's panel shows why the consultation ended.
		if err := closeWebInquiry(ctx, i.inquiries, i.messages, i.newID, inquiry, closed,
			"상담이 종료되었습니다. 더 궁금한 점이 있으면 언제든 다시 문의해 주세요."); err != nil {
			return nil, err
		}
	} else {
		inquiry.Status = domain.InquiryClosed
		inquiry.ClosedAt = &closed
		if err := i.inquiries.Save(ctx, inquiry); err != nil {
			return nil, fmt.Errorf("save inquiry: %w", err)
		}
	}
	i.publishInquiry(ctx, inquiry)
	return i.Get(ctx, id)
}

// PurgeExpiredBodies drops the text of messages past their retention window.
//
// Retention is a promise to shoppers, not a cleanup nicety: a transcript holds
// whatever they typed — names, phone numbers, addresses — so the words go on
// schedule while the inquiry's shape stays for the record.
func (i *Inbox) PurgeExpiredBodies(ctx context.Context, retention time.Duration) (int, error) {
	if retention <= 0 {
		return 0, nil
	}
	cutoff := i.now().Add(-retention)
	purged, err := i.messages.PurgeBodies(ctx, cutoff, domain.PurgedBody)
	if err != nil {
		return 0, fmt.Errorf("purge expired message bodies: %w", err)
	}
	return purged, nil
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
