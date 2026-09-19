package service_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/ports"
	"github.com/elug3/dupli1/support/pkg/service"
)

func newInbox(h *harness) *service.Inbox {
	n := 0
	return service.NewInbox(h.conversations, h.inquiries, h.messages, h.bot, func() string {
		n++
		return "msg-" + string(rune('0'+n))
	}, func() time.Time { return openHours })
}

// escalated drives a shopper through a typed message and a request for a human.
func escalated(t *testing.T) (*harness, *service.Inbox, string) {
	t.Helper()
	h := newHarnessAt(openHours)
	ctx := t.Context()

	if err := h.router.Handle(ctx, service.Inbound{
		ChatID: "42", ChatType: "private", Text: "반품하고 싶어요",
	}); err != nil {
		t.Fatalf("message: %v", err)
	}
	err := h.router.Handle(ctx, service.Inbound{
		ChatID: "42", ChatType: "private", MessageID: 555,
		CallbackQueryID: "cbq-1", CallbackData: domain.CallbackData(domain.NodeAgent),
	})
	if err != nil {
		t.Fatalf("escalate: %v", err)
	}
	open, err := h.inquiries.FindOpenByChatID(ctx, "42")
	if err != nil || open == nil {
		t.Fatalf("open inquiry = (%v, %v)", open, err)
	}
	return h, newInbox(h), open.ID
}

func TestWaitingQueueHoldsUnclaimedInquiries(t *testing.T) {
	h, inbox, id := escalated(t)

	waiting, err := inbox.List(t.Context(), ports.InquiryFilter{Unassigned: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(waiting) != 1 || waiting[0].ID != id {
		t.Fatalf("waiting = %+v, want the new inquiry", waiting)
	}
	if waiting[0].LastMessage != "반품하고 싶어요" {
		t.Fatalf("list shows %q, want the shopper's last line", waiting[0].LastMessage)
	}

	if _, err := inbox.Claim(t.Context(), id, "manager-1"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	waiting, _ = inbox.List(t.Context(), ports.InquiryFilter{Unassigned: true})
	if len(waiting) != 0 {
		t.Fatalf("a claimed inquiry must leave 대기: %+v", waiting)
	}
	mine, _ := inbox.List(t.Context(), ports.InquiryFilter{AssignedTo: "manager-1"})
	if len(mine) != 1 {
		t.Fatalf("내 상담 = %+v", mine)
	}
	_ = h
}

func TestReplyReachesTheShopperAndIsRecordedAgainstItsAuthor(t *testing.T) {
	h, inbox, id := escalated(t)

	view, err := inbox.Reply(t.Context(), id, "manager-1", "반품 접수해 드렸습니다. 택배 기사님이 방문합니다.")
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}

	if len(h.bot.replies) != 1 || h.bot.replies[0].chatID != "42" {
		t.Fatalf("replies = %+v, want one delivered to the shopper", h.bot.replies)
	}
	last := view.Transcript[len(view.Transcript)-1]
	if last.Direction != domain.DirectionOutbound || last.Author != "manager-1" {
		t.Fatalf("transcript tail = %+v, want the manager recorded as author", last)
	}
	if last.Delivery != domain.DeliverySent {
		t.Fatalf("delivery = %q, want sent", last.Delivery)
	}
}

func TestReplyingClaimsTheInquiry(t *testing.T) {
	// A manager who opens an unclaimed inquiry and just answers should not have
	// to press claim first.
	_, inbox, id := escalated(t)

	view, err := inbox.Reply(t.Context(), id, "manager-2", "확인 중입니다")
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if view.AssignedTo != "manager-2" {
		t.Fatalf("assigned to %q, want the replying manager", view.AssignedTo)
	}
}

func TestAnotherManagerCanTakeOver(t *testing.T) {
	// A claim is visible, not exclusive: a hard lock strands inquiries when a
	// shift ends mid-conversation.
	_, inbox, id := escalated(t)

	if _, err := inbox.Claim(t.Context(), id, "manager-1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	view, err := inbox.Claim(t.Context(), id, "manager-2")
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if view.AssignedTo != "manager-2" {
		t.Fatalf("assigned to %q, want the takeover recorded", view.AssignedTo)
	}
}

func TestUndeliverableReplyIsStoredAndReported(t *testing.T) {
	// The shopper blocked the bot. The manager did their part, so the reply is
	// on the record — but the console must show 미전송 rather than success.
	h, inbox, id := escalated(t)
	h.bot.replyErr = errors.New("telegram api status 403: bot was blocked by the user")

	view, err := inbox.Reply(t.Context(), id, "manager-1", "안내드립니다")
	if !errors.Is(err, service.ErrUndeliverable) {
		t.Fatalf("err = %v, want ErrUndeliverable", err)
	}
	if view == nil {
		t.Fatal("the inquiry must still come back so the console can render it")
	}
	last := view.Transcript[len(view.Transcript)-1]
	if last.Delivery != domain.DeliveryFailed {
		t.Fatalf("delivery = %q, want failed", last.Delivery)
	}
	if !strings.Contains(last.DeliveryError, "403") {
		t.Fatalf("delivery error = %q, want the reason kept", last.DeliveryError)
	}
	if last.Body != "안내드립니다" {
		t.Fatalf("body = %q, want the manager's words kept", last.Body)
	}
}

func TestAnUndeliverableReplyDoesNotMarkTheInquiryAnswered(t *testing.T) {
	h, inbox, id := escalated(t)
	h.bot.replyErr = errors.New("telegram api status 403: bot was blocked by the user")

	view, _ := inbox.Reply(t.Context(), id, "manager-1", "안내드립니다")
	if view.Status == domain.InquiryAnswered {
		t.Fatal("an inquiry whose reply never arrived has not been answered")
	}
}

func TestEmptyReplyIsRefused(t *testing.T) {
	_, inbox, id := escalated(t)
	if _, err := inbox.Reply(t.Context(), id, "manager-1", "   "); err == nil {
		t.Fatal("expected an error for an empty reply")
	}
}

func TestClosingFreesTheChatForALaterConsultation(t *testing.T) {
	h, inbox, id := escalated(t)
	ctx := t.Context()

	if _, err := inbox.Close(ctx, id, "manager-1"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if open, _ := h.inquiries.FindOpenByChatID(ctx, "42"); open != nil {
		t.Fatalf("chat still has an open inquiry: %+v", open)
	}

	// The shopper can now start a fresh consultation.
	err := h.router.Handle(ctx, service.Inbound{
		ChatID: "42", ChatType: "private", MessageID: 556,
		CallbackQueryID: "cbq-2", CallbackData: domain.CallbackData(domain.NodeAgent),
	})
	if err != nil {
		t.Fatalf("second escalation: %v", err)
	}
	if len(h.published.opened) != 2 {
		t.Fatalf("published %d events, want a second consultation announced", len(h.published.opened))
	}
}

func TestUnknownInquiryIsNotFound(t *testing.T) {
	h := newHarnessAt(openHours)
	inbox := newInbox(h)

	if _, err := inbox.Get(t.Context(), "nope"); !errors.Is(err, service.ErrInquiryNotFound) {
		t.Fatalf("err = %v, want ErrInquiryNotFound", err)
	}
	if _, err := inbox.Reply(t.Context(), "nope", "manager-1", "hi"); !errors.Is(err, service.ErrInquiryNotFound) {
		t.Fatalf("err = %v, want ErrInquiryNotFound", err)
	}
}

func TestStaleInquiriesCloseThemselves(t *testing.T) {
	// The queue should show live work, not history.
	h, inbox, id := escalated(t)
	ctx := t.Context()

	closed, err := inbox.CloseStale(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("CloseStale: %v", err)
	}
	if closed != 0 {
		t.Fatal("an inquiry opened moments ago is not stale")
	}

	// A week later, with nothing said.
	later := service.NewInbox(h.conversations, h.inquiries, h.messages, h.bot,
		func() string { return "msg-x" },
		func() time.Time { return openHours.Add(8 * 24 * time.Hour) })
	closed, err = later.CloseStale(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("CloseStale: %v", err)
	}
	if closed != 1 {
		t.Fatalf("closed %d, want the abandoned inquiry swept", closed)
	}
	view, _ := inbox.Get(ctx, id)
	if view.Status != domain.InquiryClosed || view.ClosedAt == nil {
		t.Fatalf("inquiry = %+v, want closed with a timestamp", view.Inquiry)
	}
}

func TestRetentionPurgeDropsOnlyExpiredWords(t *testing.T) {
	h := newHarnessAt(openHours)
	ctx := t.Context()

	if err := h.router.Handle(ctx, service.Inbound{
		ChatID: "42", ChatType: "private", Text: "제 연락처는 010-1234-5678 입니다",
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// Same day: nothing is due.
	inbox := newInbox(h)
	if purged, err := inbox.PurgeExpiredBodies(ctx, 180*24*time.Hour); err != nil || purged != 0 {
		t.Fatalf("purged %d (%v), want none while inside the window", purged, err)
	}

	// Half a year on, the words go.
	later := service.NewInbox(h.conversations, h.inquiries, h.messages, h.bot,
		func() string { return "msg-x" },
		func() time.Time { return openHours.Add(200 * 24 * time.Hour) })
	purged, err := later.PurgeExpiredBodies(ctx, 180*24*time.Hour)
	if err != nil {
		t.Fatalf("PurgeExpiredBodies: %v", err)
	}
	if purged != 1 {
		t.Fatalf("purged %d, want the expired message", purged)
	}
	transcript, _ := h.messages.Transcript(ctx, "id-1")
	for _, message := range transcript {
		if strings.Contains(message.Body, "010-1234-5678") {
			t.Fatal("a phone number survived its retention window")
		}
	}
}

func TestRetentionCanBeDisabled(t *testing.T) {
	h := newHarnessAt(openHours)
	inbox := newInbox(h)
	if purged, err := inbox.PurgeExpiredBodies(t.Context(), 0); err != nil || purged != 0 {
		t.Fatalf("purged %d (%v), want the sweep skipped entirely", purged, err)
	}
}
