package service_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/service"
)

func tapAgent(h *harness, t *testing.T) {
	t.Helper()
	err := h.router.Handle(t.Context(), service.Inbound{
		ChatID: "42", ChatType: "private", MessageID: 555,
		CallbackQueryID: "cbq-1", CallbackData: domain.CallbackData(domain.NodeAgent),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

func TestAskingForAHumanOpensAnInquiryAndAnnouncesIt(t *testing.T) {
	h := newHarnessAt(openHours)
	tapAgent(h, t)

	open, err := h.inquiries.FindOpenByChatID(t.Context(), "42")
	if err != nil || open == nil {
		t.Fatalf("open inquiry = (%v, %v), want one", open, err)
	}
	if open.Topic != domain.NodeAgent || open.Status != domain.InquiryOpen {
		t.Fatalf("inquiry = %+v", open)
	}

	if len(h.published.opened) != 1 {
		t.Fatalf("published %d events, want 1", len(h.published.opened))
	}
	event := h.published.opened[0]
	if event.InquiryID != open.ID || event.ChatID != "42" || event.Topic != domain.NodeAgent {
		t.Fatalf("event = %+v", event)
	}
	if event.AfterHours {
		t.Fatal("an inquiry opened at 11:00 on a Thursday is not after hours")
	}
}

func TestEscalationTellsTheShopperItIsQueued(t *testing.T) {
	h := newHarnessAt(openHours)
	tapAgent(h, t)

	text := h.bot.edits[0].text
	if !strings.Contains(text, "접수되었습니다") {
		t.Fatalf("shopper was not told the inquiry was received: %q", text)
	}
}

func TestAfterHoursEscalationStatesTheWindowAndNeverADay(t *testing.T) {
	// Saturday afternoon: closed.
	h := newHarnessAt(time.Date(2026, 9, 19, 14, 0, 0, 0, seoul()))
	tapAgent(h, t)

	text := h.bot.edits[0].text
	if !strings.Contains(text, "평일 10:00~22:00") {
		t.Fatalf("after-hours copy must state the window: %q", text)
	}
	if !strings.Contains(text, "공휴일") {
		t.Fatalf("after-hours copy must mention holidays are closed: %q", text)
	}
	for _, day := range []string{"내일", "월요일", "오늘"} {
		if strings.Contains(text, day) {
			t.Fatalf("after-hours copy names %q — without a holiday calendar that is a guess: %q", day, text)
		}
	}
	if !h.published.opened[0].AfterHours {
		t.Fatal("the event must mark the inquiry after-hours so the alert can arrive quietly")
	}
}

func TestNearClosingSoftensThePromiseButStillAlertsLoudly(t *testing.T) {
	h := newHarnessAt(time.Date(2026, 9, 17, 21, 55, 0, 0, seoul()))
	tapAgent(h, t)

	if !strings.Contains(h.bot.edits[0].text, "평일 10:00~22:00") {
		t.Fatalf("21:55 must not promise same-day handling: %q", h.bot.edits[0].text)
	}
	// Staff may well still be at their desks, so the alert is not silenced.
	if h.published.opened[0].AfterHours {
		t.Fatal("21:55 is inside the service window; the alert must stay loud")
	}
}

func TestSecondRequestDoesNotQueueTheShopperTwice(t *testing.T) {
	h := newHarnessAt(openHours)
	tapAgent(h, t)
	tapAgent(h, t)

	if len(h.published.opened) != 1 {
		t.Fatalf("published %d events, want 1 — staff would otherwise answer one shopper twice", len(h.published.opened))
	}
}

func TestTypedTextIsRecordedAndQuotedInTheAlert(t *testing.T) {
	h := newHarnessAt(openHours)
	ctx := t.Context()

	if err := h.router.Handle(ctx, service.Inbound{
		ChatID: "42", ChatType: "private", MessageID: 1,
		Text: "주문번호 01HXYZ 인데 배송이 안 와요",
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	tapAgent(h, t)

	if got := h.published.opened[0].Excerpt; got != "주문번호 01HXYZ 인데 배송이 안 와요" {
		t.Fatalf("excerpt = %q, want what the shopper typed", got)
	}
}

func TestAlertQuotesOnlyAnExcerpt(t *testing.T) {
	// Message bodies are customer data; an ops alert is a doorbell, not the
	// transcript. The rest waits behind the authenticated manager inbox.
	h := newHarnessAt(openHours)
	long := strings.Repeat("가", 500)

	if err := h.router.Handle(t.Context(), service.Inbound{
		ChatID: "42", ChatType: "private", Text: long,
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	tapAgent(h, t)

	excerpt := h.published.opened[0].Excerpt
	if len([]rune(excerpt)) > 200 {
		t.Fatalf("excerpt is %d runes — too much of the conversation", len([]rune(excerpt)))
	}
	if !strings.HasSuffix(excerpt, "…") {
		t.Fatalf("a truncated excerpt should say so: %q", excerpt)
	}
}

func TestMenuStaysQuietWhileStaffAreHandling(t *testing.T) {
	// Re-opening the menu under a shopper mid-sentence talks over them.
	h := newHarnessAt(openHours)
	ctx := t.Context()
	tapAgent(h, t)

	before := len(h.bot.menus)
	if err := h.router.Handle(ctx, service.Inbound{
		ChatID: "42", ChatType: "private", Text: "주문번호는 01HXYZ 입니다",
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(h.bot.menus) != before {
		t.Fatal("the menu must not reopen while an inquiry is open")
	}
}

func TestFailedAnnouncementSurfacesRatherThanBeingSwallowed(t *testing.T) {
	// The shopper has just been told someone will answer. If the alert never
	// reached staff, that has to reach a log, not vanish.
	h := newHarnessAt(openHours)
	h.published.err = errors.New("nats down")

	err := h.router.Handle(t.Context(), service.Inbound{
		ChatID: "42", ChatType: "private", MessageID: 555,
		CallbackQueryID: "cbq-1", CallbackData: domain.CallbackData(domain.NodeAgent),
	})
	if err == nil {
		t.Fatal("a failed announcement must surface")
	}
	// The inquiry is still recorded, so the manager inbox shows it even though
	// the ping was lost.
	if open, _ := h.inquiries.FindOpenByChatID(t.Context(), "42"); open == nil {
		t.Fatal("the inquiry must survive a failed announcement")
	}
}

func TestBrowsingTheMenuDoesNotEscalate(t *testing.T) {
	h := newHarnessAt(openHours)

	for _, node := range []string{domain.NodeOrder, domain.NodeOrderETA, domain.NodeReturn, domain.NodeRoot} {
		err := h.router.Handle(t.Context(), service.Inbound{
			ChatID: "42", ChatType: "private", MessageID: 555,
			CallbackQueryID: "cbq", CallbackData: domain.CallbackData(node),
		})
		if err != nil {
			t.Fatalf("Handle(%s): %v", node, err)
		}
	}
	if len(h.published.opened) != 0 {
		t.Fatalf("browsing queued %d inquiries, want none", len(h.published.opened))
	}
}
