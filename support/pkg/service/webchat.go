package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/ports"
)

// Web chat errors. The handler maps each to a status; none is a server fault.
var (
	// ErrInvalidMessage: nothing to send, or a body over the length limit.
	ErrInvalidMessage = errors.New("message must have text or a reference, and at most 2000 characters")
	// ErrInvalidReference: the product or order does not exist or is not the
	// caller's. One error for both, so an order id cannot be probed.
	ErrInvalidReference = errors.New("referenced product or order not found")
	// ErrReferencesUnavailable: the catalog or order service could not be
	// asked. Nothing was written; the shopper may retry.
	ErrReferencesUnavailable = errors.New("could not check the referenced product or order")
	// ErrRateLimited: the shopper is sending faster than any person types.
	ErrRateLimited = errors.New("too many messages, try again shortly")
	// ErrCustomerRequired: no account id on the token.
	ErrCustomerRequired = errors.New("a signed-in customer is required")
)

// transcriptWindow bounds how much history one load returns. A consultation
// is a few dozen lines; older ones fall off the top.
const transcriptWindow = 200

// Customer is the signed-in shopper, as their access token names them.
type Customer struct {
	ID    string
	Email string
	// Bearer is the shopper's own access token, forwarded to order so that
	// order's ABAC decides whether an order reference is theirs.
	Bearer string
}

// SendInput is one send from the chat panel: text, and optionally the
// product (by SKU) or order it is about.
type SendInput struct {
	Body      string
	ProductID string
	SkuID     string
	OrderID   string
}

// WebConversationView is the shopper's chat as the panel shows it.
type WebConversationView struct {
	ConversationID string
	// Inquiry is the open consultation, or nil when there is none.
	Inquiry  *domain.Inquiry
	Messages []domain.Message
	// Unread counts manager replies after the shopper's read mark.
	Unread        int
	ServiceOpen   bool
	ServiceWindow string
}

// WebChat is the shopper-facing side of a web consultation.
type WebChat struct {
	conversations ports.ConversationRepository
	inquiries     ports.InquiryRepository
	messages      ports.MessageRepository
	publisher     ports.InquiryPublisher
	live          ports.LivePublisher
	products      ports.ProductReader
	orders        ports.OrderReader
	notifier      ports.ShopperNotifier
	hours         domain.BusinessHours
	chatURL       string
	newID         IDGenerator
	now           Clock
	limiter       *sendLimiter
}

// WebChatDeps are WebChat's collaborators. Publisher, Live, Products, Orders
// and Notifier are optional: without them the matching feature is off, and
// the chat itself still works.
type WebChatDeps struct {
	Conversations ports.ConversationRepository
	Inquiries     ports.InquiryRepository
	Messages      ports.MessageRepository
	Publisher     ports.InquiryPublisher
	Live          ports.LivePublisher
	Products      ports.ProductReader
	Orders        ports.OrderReader
	Notifier      ports.ShopperNotifier
	Hours         domain.BusinessHours
	// ChatURL is where a reply notice links: the storefront's chat page.
	ChatURL string
	NewID   IDGenerator
	Now     Clock
}

func NewWebChat(deps WebChatDeps) *WebChat {
	newID := deps.NewID
	if newID == nil {
		newID = func() string { return "" }
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	hours := deps.Hours
	if hours.Location == nil {
		hours = domain.DefaultBusinessHours()
	}
	return &WebChat{
		conversations: deps.Conversations,
		inquiries:     deps.Inquiries,
		messages:      deps.Messages,
		publisher:     deps.Publisher,
		live:          deps.Live,
		products:      deps.Products,
		orders:        deps.Orders,
		notifier:      deps.Notifier,
		hours:         hours,
		chatURL:       deps.ChatURL,
		newID:         newID,
		now:           now,
		limiter:       newSendLimiter(),
	}
}

// Conversation returns the shopper's chat without creating one: a shopper
// who only opened the panel has not started a consultation.
func (w *WebChat) Conversation(ctx context.Context, customer Customer) (*WebConversationView, error) {
	if strings.TrimSpace(customer.ID) == "" {
		return nil, ErrCustomerRequired
	}
	conversation, err := w.conversations.FindByChatID(ctx, domain.WebChatID(customer.ID))
	if err != nil {
		return nil, fmt.Errorf("find conversation: %w", err)
	}
	return w.view(ctx, conversation)
}

func (w *WebChat) view(ctx context.Context, conversation *domain.Conversation) (*WebConversationView, error) {
	now := w.now()
	view := &WebConversationView{
		ServiceOpen:   w.hours.IsOpen(now),
		ServiceWindow: w.hours.Window(),
		Messages:      []domain.Message{},
	}
	if conversation == nil {
		return view, nil
	}
	view.ConversationID = conversation.ID

	open, err := w.inquiries.FindOpenByChatID(ctx, conversation.ChatID)
	if err != nil {
		return nil, fmt.Errorf("find open inquiry: %w", err)
	}
	view.Inquiry = open

	transcript, err := w.messages.Transcript(ctx, conversation.ID)
	if err != nil {
		return nil, fmt.Errorf("load transcript: %w", err)
	}
	for _, message := range transcript {
		if message.UnreadBy(conversation.CustomerLastReadAt) {
			view.Unread++
		}
	}
	if len(transcript) > transcriptWindow {
		transcript = transcript[len(transcript)-transcriptWindow:]
	}
	view.Messages = transcript
	return view, nil
}

// Send records what the shopper wrote, opening a consultation when none is
// open. References are checked before anything is written, so a refused
// reference leaves no half-sent message behind.
func (w *WebChat) Send(ctx context.Context, customer Customer, in SendInput) (*WebConversationView, error) {
	if strings.TrimSpace(customer.ID) == "" {
		return nil, ErrCustomerRequired
	}
	body := strings.TrimSpace(in.Body)
	hasRef := strings.TrimSpace(in.SkuID) != "" || strings.TrimSpace(in.OrderID) != ""
	if (body == "" && !hasRef) || (body != "" && !domain.ValidWebBody(body)) {
		return nil, ErrInvalidMessage
	}
	if !w.limiter.allow(customer.ID, w.now()) {
		return nil, ErrRateLimited
	}

	refs, err := resolveRefs(ctx, w.products, w.orders, customer.Bearer, in.ProductID, in.SkuID, in.OrderID)
	if err != nil {
		return nil, err
	}

	now := w.now()
	conversation, err := w.ensureConversation(ctx, customer, now)
	if err != nil {
		return nil, err
	}

	inquiry, err := w.inquiries.FindOpenByChatID(ctx, conversation.ChatID)
	if err != nil {
		return nil, fmt.Errorf("find open inquiry: %w", err)
	}
	opened := false
	if inquiry == nil {
		inquiry = domain.NewInquiry(w.newID(), conversation.ID, conversation.ChatID,
			domain.WebTopic(in.OrderID, refs.skuID()), now)
		inquiry.Channel = domain.ChannelWeb
		inquiry.CustomerID = customer.ID
		inquiry.ProductID = refs.productID()
		inquiry.SkuID = refs.skuID()
		inquiry.OrderID = refs.orderID()
		if err := w.inquiries.Save(ctx, inquiry); err != nil {
			return nil, fmt.Errorf("save inquiry: %w", err)
		}
		opened = true
	}

	var last *domain.Message
	for _, message := range refs.messages(w.newID, conversation.ID, inquiry.ID, domain.DirectionInbound, "", now) {
		if err := w.messages.Append(ctx, &message); err != nil {
			return nil, fmt.Errorf("record reference: %w", err)
		}
		last = &message
	}
	if body != "" {
		message := domain.Message{
			ID:             w.newID(),
			ConversationID: conversation.ID,
			InquiryID:      inquiry.ID,
			Direction:      domain.DirectionInbound,
			Kind:           domain.MessageText,
			Body:           body,
			CreatedAt:      now,
		}
		if err := w.messages.Append(ctx, &message); err != nil {
			return nil, fmt.Errorf("record message: %w", err)
		}
		last = &message
	}

	if opened {
		w.announce(ctx, conversation, inquiry, last, now)
	}
	if last != nil {
		w.publish(ctx, ports.LiveEvent{
			Type: ports.LiveMessage, InquiryID: inquiry.ID, ConversationID: conversation.ID,
			CustomerID: customer.ID, Channel: domain.ChannelWeb, MessageID: last.ID,
		})
	}
	return w.view(ctx, conversation)
}

// announce rings the ops doorbell for a new web consultation and, outside
// service hours, tells the shopper when someone will answer — naming the
// window, never a day, for the reason docs/support-telegram-bot.md gives.
func (w *WebChat) announce(ctx context.Context, conversation *domain.Conversation, inquiry *domain.Inquiry, first *domain.Message, now time.Time) {
	afterHours := !w.hours.IsOpen(now)
	if afterHours {
		note := domain.Message{
			ID:             w.newID(),
			ConversationID: conversation.ID,
			InquiryID:      inquiry.ID,
			Direction:      domain.DirectionOutbound,
			Kind:           domain.MessageSystem,
			Body: "지금은 상담 시간이 아닙니다. 상담 시간(" + w.hours.Window() +
				", 공휴일 휴무)에 순서대로 답변드리겠습니다.",
			CreatedAt: now,
		}
		if err := w.messages.Append(ctx, &note); err != nil {
			log.Printf("support web after-hours note for inquiry %s: %v", inquiry.ID, err)
		}
	}

	w.publish(ctx, ports.LiveEvent{
		Type: ports.LiveInquiry, InquiryID: inquiry.ID, ConversationID: conversation.ID,
		CustomerID: conversation.CustomerID, Channel: domain.ChannelWeb, Status: inquiry.Status,
	})
	if w.publisher == nil {
		return
	}
	excerpt := ""
	if first != nil {
		excerpt = domain.Excerpt(first.Body, excerptRunes)
	}
	err := w.publisher.InquiryOpened(ctx, ports.InquiryOpened{
		InquiryID:  inquiry.ID,
		ChatID:     conversation.ChatID,
		Topic:      inquiry.Topic,
		Language:   conversation.Language,
		Excerpt:    excerpt,
		AfterHours: afterHours,
		Channel:    domain.ChannelWeb,
	})
	if err != nil {
		// The message is stored and staff will see it in the inbox; only the
		// doorbell is lost. Failing the shopper's send for it would be worse.
		log.Printf("support web publish inquiry opened %s: %v", inquiry.ID, err)
	}
}

func (w *WebChat) ensureConversation(ctx context.Context, customer Customer, now time.Time) (*domain.Conversation, error) {
	chatID := domain.WebChatID(customer.ID)
	conversation, err := w.conversations.FindByChatID(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("find conversation: %w", err)
	}
	if conversation == nil {
		conversation = domain.NewConversation(w.newID(), chatID, now)
		conversation.Channel = domain.ChannelWeb
		conversation.CustomerID = customer.ID
	}
	if email := strings.TrimSpace(customer.Email); email != "" {
		conversation.CustomerEmail = email
	}
	conversation.Touch(now)
	if err := w.conversations.Save(ctx, conversation); err != nil {
		return nil, fmt.Errorf("save conversation: %w", err)
	}
	return conversation, nil
}

// MarkRead moves the shopper's read mark to now.
func (w *WebChat) MarkRead(ctx context.Context, customer Customer) error {
	if strings.TrimSpace(customer.ID) == "" {
		return ErrCustomerRequired
	}
	conversation, err := w.conversations.FindByChatID(ctx, domain.WebChatID(customer.ID))
	if err != nil {
		return fmt.Errorf("find conversation: %w", err)
	}
	if conversation == nil {
		return nil
	}
	now := w.now()
	conversation.CustomerLastReadAt = &now
	if err := w.conversations.Save(ctx, conversation); err != nil {
		return fmt.Errorf("save conversation: %w", err)
	}
	// Staff see 읽음 against their replies; tell the inbox stream.
	if open, err := w.inquiries.FindOpenByChatID(ctx, conversation.ChatID); err == nil && open != nil {
		w.publish(ctx, ports.LiveEvent{
			Type: ports.LiveInquiry, InquiryID: open.ID, ConversationID: conversation.ID,
			CustomerID: customer.ID, Channel: domain.ChannelWeb, Status: open.Status,
		})
	}
	return nil
}

// Close lets the shopper end their consultation.
func (w *WebChat) Close(ctx context.Context, customer Customer) (*WebConversationView, error) {
	if strings.TrimSpace(customer.ID) == "" {
		return nil, ErrCustomerRequired
	}
	conversation, err := w.conversations.FindByChatID(ctx, domain.WebChatID(customer.ID))
	if err != nil {
		return nil, fmt.Errorf("find conversation: %w", err)
	}
	if conversation == nil {
		return w.view(ctx, nil)
	}
	open, err := w.inquiries.FindOpenByChatID(ctx, conversation.ChatID)
	if err != nil {
		return nil, fmt.Errorf("find open inquiry: %w", err)
	}
	if open != nil {
		if err := closeWebInquiry(ctx, w.inquiries, w.messages, w.newID, open, w.now(), "상담을 종료했습니다."); err != nil {
			return nil, err
		}
		w.publish(ctx, ports.LiveEvent{
			Type: ports.LiveInquiry, InquiryID: open.ID, ConversationID: conversation.ID,
			CustomerID: customer.ID, Channel: domain.ChannelWeb, Status: domain.InquiryClosed,
		})
	}
	return w.view(ctx, conversation)
}

// closeWebInquiry closes an inquiry and leaves a system line in the
// transcript, so the shopper's panel shows why the consultation ended.
func closeWebInquiry(ctx context.Context, inquiries ports.InquiryRepository, messages ports.MessageRepository,
	newID IDGenerator, inquiry *domain.Inquiry, now time.Time, line string) error {
	inquiry.Status = domain.InquiryClosed
	inquiry.ClosedAt = &now
	if err := inquiries.Save(ctx, inquiry); err != nil {
		return fmt.Errorf("save inquiry: %w", err)
	}
	note := domain.Message{
		ID:             newID(),
		ConversationID: inquiry.ConversationID,
		InquiryID:      inquiry.ID,
		Direction:      domain.DirectionOutbound,
		Kind:           domain.MessageSystem,
		Body:           line,
		CreatedAt:      now,
	}
	if err := messages.Append(ctx, &note); err != nil {
		return fmt.Errorf("record close note: %w", err)
	}
	return nil
}

// ForgetCustomer erases a deleted account's words: every message body in
// their web conversation becomes the purge placeholder, the email goes, and
// an open consultation is closed. The rows stay, so inquiry history keeps
// its shape, as the retention purge already does.
func (w *WebChat) ForgetCustomer(ctx context.Context, customerID string) error {
	if strings.TrimSpace(customerID) == "" {
		return nil
	}
	conversation, err := w.conversations.FindByChatID(ctx, domain.WebChatID(customerID))
	if err != nil {
		return fmt.Errorf("find conversation: %w", err)
	}
	if conversation == nil {
		return nil
	}
	if open, err := w.inquiries.FindOpenByChatID(ctx, conversation.ChatID); err == nil && open != nil {
		now := w.now()
		open.Status = domain.InquiryClosed
		open.ClosedAt = &now
		if err := w.inquiries.Save(ctx, open); err != nil {
			return fmt.Errorf("close inquiry: %w", err)
		}
	}
	if _, err := w.messages.PurgeConversation(ctx, conversation.ID, domain.PurgedBody); err != nil {
		return err
	}
	return w.conversations.ForgetCustomer(ctx, conversation.ID)
}

// SendDueNotices emails shoppers about replies they have left unread for at
// least delay. One email covers every unread reply in a conversation until
// the shopper reads; each reply records whether it was sent, failed, or
// skipped (read in time, already covered, or no way to send). Returns how
// many emails went out.
func (w *WebChat) SendDueNotices(ctx context.Context, delay time.Duration) (int, error) {
	due, err := w.messages.DueNotices(ctx, w.now().Add(-delay), 200)
	if err != nil {
		return 0, err
	}

	// Group by conversation, keeping the newest reply of each: whether the
	// shopper has read past it decides for the whole group.
	type group struct {
		ids    []string
		latest domain.Message
	}
	groups := map[string]*group{}
	var order []string
	for _, message := range due {
		g, ok := groups[message.ConversationID]
		if !ok {
			g = &group{}
			groups[message.ConversationID] = g
			order = append(order, message.ConversationID)
		}
		g.ids = append(g.ids, message.ID)
		g.latest = message
	}

	sent := 0
	var errs []error
	for _, conversationID := range order {
		g := groups[conversationID]
		status, err := w.noticeFor(ctx, conversationID, g.latest)
		if err != nil {
			errs = append(errs, err)
		}
		if status == domain.NoticeSent {
			sent++
		}
		if err := w.messages.SetNoticeStatus(ctx, g.ids, status); err != nil {
			errs = append(errs, err)
		}
	}
	return sent, errors.Join(errs...)
}

func (w *WebChat) noticeFor(ctx context.Context, conversationID string, reply domain.Message) (string, error) {
	conversation, err := w.conversations.FindByID(ctx, conversationID)
	if err != nil {
		return domain.NoticeSkipped, fmt.Errorf("find conversation: %w", err)
	}
	if conversation == nil || !conversation.IsWeb() || !conversation.NeedsNotice(reply) {
		return domain.NoticeSkipped, nil
	}
	if w.notifier == nil || strings.TrimSpace(conversation.CustomerEmail) == "" || w.chatURL == "" {
		return domain.NoticeSkipped, nil
	}

	subject := w.subjectOf(ctx, conversation, reply.InquiryID)
	if err := w.notifier.NotifyReply(ctx, conversation.CustomerEmail, subject, w.chatURL); err != nil {
		// Logged without the address: an email is customer data.
		return domain.NoticeFailed, fmt.Errorf("reply notice for conversation %s: %w", conversation.ID, err)
	}
	now := w.now()
	conversation.CustomerNotifiedAt = &now
	if err := w.conversations.Save(ctx, conversation); err != nil {
		return domain.NoticeSent, fmt.Errorf("record notice: %w", err)
	}
	return domain.NoticeSent, nil
}

// subjectOf names what the consultation is about for the email: the product
// the shopper attached, or the order, or nothing for a general question.
func (w *WebChat) subjectOf(ctx context.Context, conversation *domain.Conversation, inquiryID string) string {
	inquiry, err := w.inquiries.FindByID(ctx, inquiryID)
	if err != nil || inquiry == nil {
		return ""
	}
	if inquiry.OrderID != "" {
		return "주문 " + inquiry.OrderID
	}
	if inquiry.SkuID == "" {
		return ""
	}
	transcript, err := w.messages.Transcript(ctx, conversation.ID)
	if err != nil {
		return ""
	}
	for _, message := range transcript {
		if message.Kind == domain.MessageProductRef && message.RefID == inquiry.SkuID {
			var ref ports.ProductRef
			if json.Unmarshal(message.RefSnapshot, &ref) == nil && ref.Name != "" {
				return ref.Name
			}
		}
	}
	return ""
}

func (w *WebChat) publish(ctx context.Context, event ports.LiveEvent) {
	if w.live != nil {
		w.live.PublishLive(ctx, event)
	}
}

// resolvedRefs are the references of one send, checked and snapshotted.
type resolvedRefs struct {
	product *ports.ProductRef
	order   *ports.OrderRef
}

func (r resolvedRefs) skuID() string {
	if r.product == nil {
		return ""
	}
	return r.product.SkuID
}

func (r resolvedRefs) productID() string {
	if r.product == nil {
		return ""
	}
	return r.product.ProductID
}

func (r resolvedRefs) orderID() string {
	if r.order == nil {
		return ""
	}
	return r.order.OrderID
}

// messages renders the references as transcript rows. Each carries a short
// readable body too, so a transcript read without the card (the Telegram-era
// console, an export) still says what was attached.
func (r resolvedRefs) messages(newID IDGenerator, conversationID, inquiryID, direction, author string, now time.Time) []domain.Message {
	var out []domain.Message
	if r.product != nil {
		snapshot, _ := json.Marshal(r.product)
		body := "상품: " + r.product.Name
		if r.product.Color != "" {
			body += " (" + r.product.Color + ")"
		}
		out = append(out, domain.Message{
			ID: newID(), ConversationID: conversationID, InquiryID: inquiryID,
			Direction: direction, Author: author, Kind: domain.MessageProductRef,
			Body: body, RefID: r.product.SkuID, RefSnapshot: snapshot, CreatedAt: now,
		})
	}
	if r.order != nil {
		snapshot, _ := json.Marshal(r.order)
		out = append(out, domain.Message{
			ID: newID(), ConversationID: conversationID, InquiryID: inquiryID,
			Direction: direction, Author: author, Kind: domain.MessageOrderRef,
			Body: "주문: " + r.order.OrderID, RefID: r.order.OrderID, RefSnapshot: snapshot, CreatedAt: now,
		})
	}
	return out
}

// resolveRefs checks a send's references against their owning services.
func resolveRefs(ctx context.Context, products ports.ProductReader, orders ports.OrderReader,
	bearer, productID, skuID, orderID string) (resolvedRefs, error) {
	var refs resolvedRefs
	productID, skuID, orderID = strings.TrimSpace(productID), strings.TrimSpace(skuID), strings.TrimSpace(orderID)

	if productID != "" && skuID == "" {
		// A product is referenced by its variant: that is the sellable item,
		// and product's variant lookup is the one that counts no PDP view.
		return refs, ErrInvalidReference
	}
	if skuID != "" {
		if products == nil {
			return refs, ErrReferencesUnavailable
		}
		ref, err := products.Variant(ctx, skuID)
		switch {
		case errors.Is(err, ports.ErrReferenceNotFound):
			return refs, ErrInvalidReference
		case err != nil:
			return refs, fmt.Errorf("%w: %v", ErrReferencesUnavailable, err)
		case productID != "" && ref.ProductID != productID:
			return refs, ErrInvalidReference
		}
		refs.product = ref
	}
	if orderID != "" {
		if orders == nil {
			return refs, ErrReferencesUnavailable
		}
		ref, err := orders.Order(ctx, bearer, orderID)
		switch {
		case errors.Is(err, ports.ErrReferenceNotFound):
			return refs, ErrInvalidReference
		case err != nil:
			return refs, fmt.Errorf("%w: %v", ErrReferencesUnavailable, err)
		}
		refs.order = ref
	}
	return refs, nil
}

// sendLimiter caps how fast one shopper can send: a burst per minute and a
// daily ceiling, the web counterpart of the Telegram bot's flood guard. It is
// per replica, which is fine for its job — stopping a stuck client or a
// script — not metering.
type sendLimiter struct {
	mu      sync.Mutex
	perMin  int
	perDay  int
	windows map[string]*sendWindow
}

type sendWindow struct {
	minuteStart time.Time
	minuteCount int
	dayStart    time.Time
	dayCount    int
}

func newSendLimiter() *sendLimiter {
	return &sendLimiter{perMin: 20, perDay: 500, windows: map[string]*sendWindow{}}
}

func (l *sendLimiter) allow(customerID string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Shed shoppers whose day window has lapsed, so the map holds today's
	// senders rather than everyone this replica has ever seen.
	if len(l.windows) > 10000 {
		for id, w := range l.windows {
			if now.Sub(w.dayStart) >= 24*time.Hour {
				delete(l.windows, id)
			}
		}
	}
	win, ok := l.windows[customerID]
	if !ok {
		win = &sendWindow{minuteStart: now, dayStart: now}
		l.windows[customerID] = win
	}
	if now.Sub(win.minuteStart) >= time.Minute {
		win.minuteStart, win.minuteCount = now, 0
	}
	if now.Sub(win.dayStart) >= 24*time.Hour {
		win.dayStart, win.dayCount = now, 0
	}
	if win.minuteCount >= l.perMin || win.dayCount >= l.perDay {
		return false
	}
	win.minuteCount++
	win.dayCount++
	return true
}
