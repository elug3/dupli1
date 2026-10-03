package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/infra/memory"
	"github.com/elug3/dupli1/support/pkg/ports"
	"github.com/elug3/dupli1/support/pkg/service"
)

type fakeProducts struct {
	variants map[string]ports.ProductRef
	err      error
}

func (p *fakeProducts) Variant(_ context.Context, skuID string) (*ports.ProductRef, error) {
	if p.err != nil {
		return nil, p.err
	}
	ref, ok := p.variants[skuID]
	if !ok {
		return nil, ports.ErrReferenceNotFound
	}
	return &ref, nil
}

// fakeOrders lets a bearer read only the orders listed against it, the way
// order's ABAC does for a customer.
type fakeOrders struct {
	byBearer map[string]map[string]ports.OrderRef
	bearers  []string
}

func (o *fakeOrders) Order(_ context.Context, bearer, orderID string) (*ports.OrderRef, error) {
	o.bearers = append(o.bearers, bearer)
	ref, ok := o.byBearer[bearer][orderID]
	if !ok {
		return nil, ports.ErrReferenceNotFound
	}
	return &ref, nil
}

type sentNotice struct{ to, subject, link string }

type fakeNotifier struct {
	sent []sentNotice
	err  error
}

func (n *fakeNotifier) NotifyReply(_ context.Context, to, subject, link string) error {
	if n.err != nil {
		return n.err
	}
	n.sent = append(n.sent, sentNotice{to, subject, link})
	return nil
}

type recordingLive struct {
	mu     sync.Mutex
	events []ports.LiveEvent
}

func (l *recordingLive) PublishLive(_ context.Context, event ports.LiveEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

type webHarness struct {
	chat          *service.WebChat
	inbox         *service.Inbox
	conversations *memory.ConversationRepository
	inquiries     *memory.InquiryRepository
	messages      *memory.MessageRepository
	publisher     *fakePublisher
	live          *recordingLive
	products      *fakeProducts
	orders        *fakeOrders
	notifier      *fakeNotifier
	clock         *time.Time
}

var shopper = service.Customer{ID: "user-1", Email: "shopper@example.com", Bearer: "token-1"}

func newWebHarness(at time.Time) *webHarness {
	h := &webHarness{
		conversations: memory.NewConversationRepository(),
		inquiries:     memory.NewInquiryRepository(),
		messages:      memory.NewMessageRepository(),
		publisher:     &fakePublisher{},
		live:          &recordingLive{},
		products: &fakeProducts{variants: map[string]ports.ProductRef{
			"SKU01": {ProductID: "P01", SkuID: "SKU01", SKU: "PRADA_GAL_BLK_M", Name: "Prada Galleria", Color: "Black", PriceWon: 3200000},
		}},
		orders: &fakeOrders{byBearer: map[string]map[string]ports.OrderRef{
			"token-1":       {"ORD1": {OrderID: "ORD1", Status: "paid", TotalWon: 3200000, ItemCount: 1}},
			"manager-token": {"ORD1": {OrderID: "ORD1", Status: "paid", TotalWon: 3200000, ItemCount: 1}, "ORD2": {OrderID: "ORD2", Status: "fulfilled"}},
		}},
		notifier: &fakeNotifier{},
		clock:    &at,
	}
	n := 0
	newID := func() string {
		n++
		return fmt.Sprintf("id-%03d", n)
	}
	now := func() time.Time { return *h.clock }
	h.chat = service.NewWebChat(service.WebChatDeps{
		Conversations: h.conversations,
		Inquiries:     h.inquiries,
		Messages:      h.messages,
		Publisher:     h.publisher,
		Live:          h.live,
		Products:      h.products,
		Orders:        h.orders,
		Notifier:      h.notifier,
		Hours:         domain.DefaultBusinessHours(),
		ChatURL:       "https://dupli1.com/profile/support",
		NewID:         newID,
		Now:           now,
	})
	h.inbox = service.NewInbox(h.conversations, h.inquiries, h.messages, &fakeBot{}, newID, now).
		WithWebChat(h.products, h.orders, h.live)
	return h
}

func (h *webHarness) advance(d time.Duration) { *h.clock = h.clock.Add(d) }

func TestWebConversationIsEmptyUntilTheShopperWrites(t *testing.T) {
	h := newWebHarness(openHours)
	view, err := h.chat.Conversation(t.Context(), shopper)
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if view.Inquiry != nil || len(view.Messages) != 0 || view.ConversationID != "" {
		t.Fatalf("view = %+v, want nothing created by a read", view)
	}
	if !view.ServiceOpen {
		t.Fatalf("11:00 on a Thursday must be inside service hours")
	}
}

func TestFirstWebMessageOpensAnInquiryAboutTheProduct(t *testing.T) {
	h := newWebHarness(openHours)
	view, err := h.chat.Send(t.Context(), shopper, service.SendInput{
		Body: "이 가방 재입고 되나요?", ProductID: "P01", SkuID: "SKU01",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if view.Inquiry == nil {
		t.Fatal("no inquiry opened")
	}
	inq := view.Inquiry
	if inq.Channel != domain.ChannelWeb || inq.CustomerID != "user-1" || inq.SkuID != "SKU01" || inq.ProductID != "P01" {
		t.Fatalf("inquiry = %+v", inq)
	}
	if inq.Topic != domain.NodeProduct {
		t.Fatalf("topic = %q, want product", inq.Topic)
	}
	if len(view.Messages) != 2 || view.Messages[0].Kind != domain.MessageProductRef || view.Messages[1].Body != "이 가방 재입고 되나요?" {
		t.Fatalf("messages = %+v, want the product card then the text", view.Messages)
	}
	if len(h.publisher.opened) != 1 {
		t.Fatalf("staff must be told once, got %d", len(h.publisher.opened))
	}
	opened := h.publisher.opened[0]
	if opened.Channel != domain.ChannelWeb {
		t.Fatalf("announcement = %+v", opened)
	}

	// A second message joins the same consultation and does not ring again.
	if _, err := h.chat.Send(t.Context(), shopper, service.SendInput{Body: "블랙으로요"}); err != nil {
		t.Fatalf("second Send: %v", err)
	}
	if len(h.publisher.opened) != 1 {
		t.Fatalf("a follow-up must not announce a new inquiry")
	}
	open, _ := h.inquiries.List(t.Context(), ports.InquiryFilter{Channel: domain.ChannelWeb})
	if len(open) != 1 {
		t.Fatalf("web inquiries = %d, want 1", len(open))
	}
}

func TestWebSendRefusesBadReferencesBeforeWritingAnything(t *testing.T) {
	cases := []struct {
		name string
		in   service.SendInput
		want error
	}{
		{"unknown sku", service.SendInput{Body: "hi", SkuID: "NOPE"}, service.ErrInvalidReference},
		{"product without sku", service.SendInput{Body: "hi", ProductID: "P01"}, service.ErrInvalidReference},
		{"sku of another product", service.SendInput{Body: "hi", ProductID: "P99", SkuID: "SKU01"}, service.ErrInvalidReference},
		// ORD2 exists but is not this shopper's: indistinguishable from missing.
		{"someone else's order", service.SendInput{Body: "hi", OrderID: "ORD2"}, service.ErrInvalidReference},
		{"empty", service.SendInput{Body: "   "}, service.ErrInvalidMessage},
		{"too long", service.SendInput{Body: strings.Repeat("가", domain.MaxWebMessageRunes+1)}, service.ErrInvalidMessage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newWebHarness(openHours)
			_, err := h.chat.Send(t.Context(), shopper, tc.in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if rows, _ := h.inquiries.List(t.Context(), ports.InquiryFilter{}); len(rows) != 0 {
				t.Fatalf("a refused send left an inquiry behind")
			}
		})
	}
}

func TestWebOrderReferenceIsReadWithTheShoppersOwnToken(t *testing.T) {
	h := newWebHarness(openHours)
	view, err := h.chat.Send(t.Context(), shopper, service.SendInput{OrderID: "ORD1"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if view.Inquiry.OrderID != "ORD1" || view.Inquiry.Topic != domain.NodeOrder {
		t.Fatalf("inquiry = %+v", view.Inquiry)
	}
	if len(h.orders.bearers) != 1 || h.orders.bearers[0] != "token-1" {
		t.Fatalf("order was read with %v, want the shopper's token", h.orders.bearers)
	}
}

func TestWebReferencesUnavailableIsNotANotFound(t *testing.T) {
	h := newWebHarness(openHours)
	h.products.err = errors.New("gateway down")
	_, err := h.chat.Send(t.Context(), shopper, service.SendInput{Body: "hi", SkuID: "SKU01"})
	if !errors.Is(err, service.ErrReferencesUnavailable) {
		t.Fatalf("err = %v, want ErrReferencesUnavailable", err)
	}
}

func TestWebSendAfterHoursLeavesANoteNamingTheWindow(t *testing.T) {
	h := newWebHarness(time.Date(2026, 9, 19, 11, 0, 0, 0, seoul())) // Saturday
	view, err := h.chat.Send(t.Context(), shopper, service.SendInput{Body: "주말에도 되나요?"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	var note *domain.Message
	for i := range view.Messages {
		if view.Messages[i].Kind == domain.MessageSystem {
			note = &view.Messages[i]
		}
	}
	if note == nil || !strings.Contains(note.Body, "10:00") {
		t.Fatalf("messages = %+v, want an after-hours note naming the window", view.Messages)
	}
	if view.Unread != 0 {
		t.Fatalf("a system note must not count as unread, got %d", view.Unread)
	}
	if !h.publisher.opened[0].AfterHours {
		t.Fatalf("announcement must say it arrived after hours")
	}
}

func TestWebSendIsRateLimited(t *testing.T) {
	h := newWebHarness(openHours)
	for i := 0; i < 20; i++ {
		if _, err := h.chat.Send(t.Context(), shopper, service.SendInput{Body: "hi"}); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	if _, err := h.chat.Send(t.Context(), shopper, service.SendInput{Body: "hi"}); !errors.Is(err, service.ErrRateLimited) {
		t.Fatalf("21st send err = %v, want ErrRateLimited", err)
	}
	h.advance(time.Minute)
	if _, err := h.chat.Send(t.Context(), shopper, service.SendInput{Body: "hi"}); err != nil {
		t.Fatalf("a minute later the shopper may send again: %v", err)
	}
}

func TestManagerWebReplyCountsAsUnreadUntilRead(t *testing.T) {
	h := newWebHarness(openHours)
	view, _ := h.chat.Send(t.Context(), shopper, service.SendInput{Body: "문의드려요"})
	inquiryID := view.Inquiry.ID

	h.advance(time.Minute)
	reply, err := h.inbox.ReplyWith(t.Context(), service.ReplyInput{
		InquiryID: inquiryID, ManagerID: "manager-1", Body: "네, 확인해 드릴게요.",
		Bearer: "manager-token", OrderID: "ORD2",
	})
	if err != nil {
		t.Fatalf("ReplyWith: %v", err)
	}
	if reply.Status != domain.InquiryAnswered || reply.AssignedTo != "manager-1" {
		t.Fatalf("inquiry = %+v, want claimed and answered", reply.Inquiry)
	}
	last := reply.Transcript[len(reply.Transcript)-1]
	if last.NoticeStatus != domain.NoticePending || last.Delivery != domain.DeliverySent {
		t.Fatalf("reply row = %+v, want sent with a pending notice", last)
	}

	view, _ = h.chat.Conversation(t.Context(), shopper)
	if view.Unread != 2 {
		t.Fatalf("unread = %d, want the order card and the text", view.Unread)
	}
	h.advance(time.Second)
	if err := h.chat.MarkRead(t.Context(), shopper); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	view, _ = h.chat.Conversation(t.Context(), shopper)
	if view.Unread != 0 {
		t.Fatalf("unread after reading = %d", view.Unread)
	}
	got, _ := h.inbox.Get(t.Context(), inquiryID)
	if got.CustomerLastReadAt == nil || got.CustomerEmail != "shopper@example.com" {
		t.Fatalf("console view = %+v, want the read mark and the email", got)
	}
}

func TestReferenceOnTelegramInquiryIsRefused(t *testing.T) {
	_, inbox, id := escalated(t)
	_, err := inbox.ReplyWith(t.Context(), service.ReplyInput{InquiryID: id, ManagerID: "m", Body: "x", SkuID: "SKU01"})
	if !errors.Is(err, service.ErrReferenceOnTelegram) {
		t.Fatalf("err = %v, want ErrReferenceOnTelegram", err)
	}
}

func TestReplyNoticeGoesOutOnceForAnUnreadBatch(t *testing.T) {
	h := newWebHarness(openHours)
	view, _ := h.chat.Send(t.Context(), shopper, service.SendInput{Body: "이 가방요", SkuID: "SKU01"})
	id := view.Inquiry.ID

	h.advance(time.Minute)
	for _, body := range []string{"재입고 예정입니다.", "다음 주 화요일이에요."} {
		if _, err := h.inbox.ReplyWith(t.Context(), service.ReplyInput{InquiryID: id, ManagerID: "m", Body: body}); err != nil {
			t.Fatalf("reply: %v", err)
		}
	}

	// Not yet five minutes: nothing goes.
	h.advance(4 * time.Minute)
	if sent, err := h.chat.SendDueNotices(t.Context(), 5*time.Minute); err != nil || sent != 0 {
		t.Fatalf("early sweep = (%d, %v)", sent, err)
	}

	h.advance(2 * time.Minute)
	sent, err := h.chat.SendDueNotices(t.Context(), 5*time.Minute)
	if err != nil || sent != 1 {
		t.Fatalf("sweep = (%d, %v), want one email for both replies", sent, err)
	}
	notice := h.notifier.sent[0]
	if notice.to != "shopper@example.com" || notice.link != "https://dupli1.com/profile/support" || notice.subject != "Prada Galleria" {
		t.Fatalf("notice = %+v", notice)
	}
	for _, body := range []string{"재입고", "화요일"} {
		if strings.Contains(notice.subject, body) {
			t.Fatalf("the email must never carry the reply text")
		}
	}

	// Another reply while still unread is covered by the email already sent.
	if _, err := h.inbox.ReplyWith(t.Context(), service.ReplyInput{InquiryID: id, ManagerID: "m", Body: "참고로요"}); err != nil {
		t.Fatalf("reply: %v", err)
	}
	h.advance(6 * time.Minute)
	if sent, _ := h.chat.SendDueNotices(t.Context(), 5*time.Minute); sent != 0 {
		t.Fatalf("a second email for the same unread batch was sent")
	}

	// Once read, the next unread reply earns a new notice.
	_ = h.chat.MarkRead(t.Context(), shopper)
	h.advance(time.Second)
	if _, err := h.inbox.ReplyWith(t.Context(), service.ReplyInput{InquiryID: id, ManagerID: "m", Body: "또 하나요"}); err != nil {
		t.Fatalf("reply: %v", err)
	}
	h.advance(6 * time.Minute)
	if sent, _ := h.chat.SendDueNotices(t.Context(), 5*time.Minute); sent != 1 {
		t.Fatalf("a reply after reading must be noticed again")
	}

	// Nothing is left pending: every reply ended sent or skipped.
	if due, _ := h.messages.DueNotices(t.Context(), h.clock.Add(time.Hour), 100); len(due) != 0 {
		t.Fatalf("still pending: %+v", due)
	}
}

func TestReplyReadInTimeSendsNoNotice(t *testing.T) {
	h := newWebHarness(openHours)
	view, _ := h.chat.Send(t.Context(), shopper, service.SendInput{Body: "hi"})
	h.advance(time.Minute)
	_, _ = h.inbox.ReplyWith(t.Context(), service.ReplyInput{InquiryID: view.Inquiry.ID, ManagerID: "m", Body: "hello"})
	h.advance(time.Minute)
	_ = h.chat.MarkRead(t.Context(), shopper)
	h.advance(10 * time.Minute)
	if sent, _ := h.chat.SendDueNotices(t.Context(), 5*time.Minute); sent != 0 || len(h.notifier.sent) != 0 {
		t.Fatalf("a reply read in time must not be emailed")
	}
}

func TestShopperCloseLeavesASystemLine(t *testing.T) {
	h := newWebHarness(openHours)
	_, _ = h.chat.Send(t.Context(), shopper, service.SendInput{Body: "hi"})
	view, err := h.chat.Close(t.Context(), shopper)
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if view.Inquiry != nil {
		t.Fatalf("closed consultation still open: %+v", view.Inquiry)
	}
	last := view.Messages[len(view.Messages)-1]
	if last.Kind != domain.MessageSystem {
		t.Fatalf("last line = %+v, want a system line", last)
	}
	// Writing again opens a fresh consultation in the same conversation.
	again, _ := h.chat.Send(t.Context(), shopper, service.SendInput{Body: "또 질문요"})
	if again.Inquiry == nil || again.ConversationID != view.ConversationID {
		t.Fatalf("view = %+v, want a new inquiry on the same conversation", again)
	}
}

func TestForgetCustomerErasesTheirWords(t *testing.T) {
	h := newWebHarness(openHours)
	view, _ := h.chat.Send(t.Context(), shopper, service.SendInput{Body: "제 주소는 서울시...", SkuID: "SKU01"})
	if err := h.chat.ForgetCustomer(t.Context(), "user-1"); err != nil {
		t.Fatalf("ForgetCustomer: %v", err)
	}
	inq, _ := h.inquiries.FindByID(t.Context(), view.Inquiry.ID)
	if inq.Status != domain.InquiryClosed {
		t.Fatalf("open consultation of a deleted account = %q", inq.Status)
	}
	transcript, _ := h.messages.Transcript(t.Context(), view.ConversationID)
	for _, m := range transcript {
		if m.Body != domain.PurgedBody || len(m.RefSnapshot) != 0 {
			t.Fatalf("message %s still reads %q", m.ID, m.Body)
		}
	}
	conv, _ := h.conversations.FindByID(t.Context(), view.ConversationID)
	if conv.CustomerEmail != "" {
		t.Fatalf("email kept after deletion")
	}
}

func TestWebEventsCarryTheCustomer(t *testing.T) {
	h := newWebHarness(openHours)
	view, _ := h.chat.Send(t.Context(), shopper, service.SendInput{Body: "hi"})
	_, _ = h.inbox.ReplyWith(t.Context(), service.ReplyInput{InquiryID: view.Inquiry.ID, ManagerID: "m", Body: "yo"})
	if len(h.live.events) == 0 {
		t.Fatal("no live events")
	}
	for _, e := range h.live.events {
		if e.CustomerID != "user-1" || e.Channel != domain.ChannelWeb {
			t.Fatalf("event %+v must carry the customer, or their stream never hears it", e)
		}
	}
}
