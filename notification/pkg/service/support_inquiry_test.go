package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/notification/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/events"
)

// silentNotifier records which deliveries pinged and which did not.
type silentNotifier struct {
	recordedNotifier
	silentChats   []string
	silentMessage string
}

func (s *silentNotifier) SendSilent(_ context.Context, chatID string, message string) error {
	s.silentChats = append(s.silentChats, chatID)
	s.silentMessage = message
	return nil
}

func inquiryPayload(t *testing.T, event events.SupportInquiry) []byte {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return payload
}

func TestSupportInquiryReachesOptedInChatsOnly(t *testing.T) {
	notifier := &recordedNotifier{}
	routing := &stubChatRouting{
		orderChats:   []string{"-order-only"},
		productChats: []string{"-product-only"},
		supportChats: []string{"-support"},
	}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		Routing:       routing,
		OrderChatID:   "-env-order",
		ProductChatID: "-env-product",
		ManageWebURL:  "https://manage.dupli1.com",
	})

	payload := inquiryPayload(t, events.SupportInquiry{
		InquiryID: "01HINQ", ChatID: "42", Topic: "ret",
		Username: "shopper", Excerpt: "반품하고 싶어요",
		ManageURL: "https://manage.dupli1.com/support/inquiries/01HINQ",
		OpenedAt:  time.Now().UTC(),
	})
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectSupportInquiryOpened, payload); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if len(notifier.chatIDs) != 1 || notifier.chatIDs[0] != "-support" {
		t.Fatalf("chats = %v, want only the chat that opted into support alerts", notifier.chatIDs)
	}
	for _, unwanted := range []string{"반품하고 싶어요"} {
		if !strings.Contains(notifier.message, unwanted) {
			t.Fatalf("alert should quote the excerpt: %q", notifier.message)
		}
	}
	if !strings.Contains(notifier.message, "01HINQ") || !strings.Contains(notifier.message, "교환·반품") {
		t.Fatalf("alert = %q, want the inquiry id and the topic", notifier.message)
	}
	if !strings.Contains(notifier.message, "/support/inquiries/01HINQ") {
		t.Fatalf("alert = %q, want a link into the manager inbox", notifier.message)
	}
}

func TestOrderChatsDoNotReceiveConsultations(t *testing.T) {
	// A chat configured for order alerts never asked to field consultations;
	// the env fallback that order alerts use must not apply here.
	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		Routing:     &stubChatRouting{orderChats: []string{"-order"}},
		OrderChatID: "-env-order",
	})

	payload := inquiryPayload(t, events.SupportInquiry{InquiryID: "01HINQ", ChatID: "42", Topic: "agt"})
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectSupportInquiryOpened, payload); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(notifier.chatIDs) != 0 {
		t.Fatalf("chats = %v, want none", notifier.chatIDs)
	}
}

func TestAfterHoursInquiryArrivesWithoutAPing(t *testing.T) {
	notifier := &silentNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		Routing: &stubChatRouting{supportChats: []string{"-support"}},
	})

	payload := inquiryPayload(t, events.SupportInquiry{
		InquiryID: "01HINQ", ChatID: "42", Topic: "agt", AfterHours: true,
	})
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectSupportInquiryOpened, payload); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if len(notifier.silentChats) != 1 || notifier.silentChats[0] != "-support" {
		t.Fatalf("silent chats = %v, want the after-hours alert delivered quietly", notifier.silentChats)
	}
	if len(notifier.chatIDs) != 0 {
		t.Fatalf("an after-hours alert must not ping: %v", notifier.chatIDs)
	}
	if !strings.Contains(notifier.silentMessage, "영업시간 외") {
		t.Fatalf("after-hours alert should say so: %q", notifier.silentMessage)
	}
}

func TestInHoursInquiryPings(t *testing.T) {
	notifier := &silentNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		Routing: &stubChatRouting{supportChats: []string{"-support"}},
	})

	payload := inquiryPayload(t, events.SupportInquiry{InquiryID: "01HINQ", ChatID: "42", Topic: "agt"})
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectSupportInquiryOpened, payload); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(notifier.chatIDs) != 1 {
		t.Fatalf("an inquiry inside the window should ping: %v", notifier.chatIDs)
	}
	if len(notifier.silentChats) != 0 {
		t.Fatalf("it must not be silenced: %v", notifier.silentChats)
	}
}

func TestNoSupportChatIsASkipNotAFailure(t *testing.T) {
	// Losing the alert is bad and is logged; failing the handler would have
	// NATS redeliver an event nobody can route either.
	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{Routing: &stubChatRouting{}})

	payload := inquiryPayload(t, events.SupportInquiry{InquiryID: "01HINQ", ChatID: "42", Topic: "agt"})
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectSupportInquiryOpened, payload); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(notifier.chatIDs) != 0 {
		t.Fatalf("chats = %v", notifier.chatIDs)
	}
}

func TestAlertEscapesShopperText(t *testing.T) {
	// A shopper's message is arbitrary text going into an HTML-parsed message;
	// an unescaped angle bracket would break the alert or inject markup.
	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		Routing: &stubChatRouting{supportChats: []string{"-support"}},
	})

	payload := inquiryPayload(t, events.SupportInquiry{
		InquiryID: "01HINQ", ChatID: "42", Topic: "agt",
		Excerpt: `<b>굵게</b> & "따옴표"`, Username: `<script>`,
	})
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectSupportInquiryOpened, payload); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if strings.Contains(notifier.message, "<b>굵게</b>") || strings.Contains(notifier.message, "<script>") {
		t.Fatalf("shopper text reached the alert unescaped: %q", notifier.message)
	}
	if !strings.Contains(notifier.message, "&lt;b&gt;") {
		t.Fatalf("expected escaped markup in %q", notifier.message)
	}
}

func TestWebInquiryAlertNamesTheChannel(t *testing.T) {
	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		Routing: &stubChatRouting{supportChats: []string{"-support"}},
	})

	payload := inquiryPayload(t, events.SupportInquiry{
		InquiryID: "01HWEB", ChatID: "web:user-1", Topic: "prd", Channel: "web",
	})
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectSupportInquiryOpened, payload); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !strings.Contains(notifier.message, "채널: 웹") {
		t.Fatalf("alert = %q, want the web channel named", notifier.message)
	}
	// The web chat id is the shopper's account id; it stays out of ops chats.
	if strings.Contains(notifier.message, "user-1") {
		t.Fatalf("alert leaked the account id: %q", notifier.message)
	}
}

func TestProductQuestionAlertSaysItIsAProductQuestion(t *testing.T) {
	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		Routing: &stubChatRouting{supportChats: []string{"-support"}},
	})
	payload := inquiryPayload(t, events.SupportInquiry{
		InquiryID: "01HQ", Topic: "prd", Channel: "product_question",
		EntryContext: "숏 패딩 재킷 Black / M", Excerpt: "평소 L인데 M?",
		ManageURL: "https://manage.dupli1.com/support?tab=questions&question=01HQ",
		OpenedAt:  time.Now().UTC(),
	})
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectSupportInquiryOpened, payload); err != nil {
		t.Fatalf("handle: %v", err)
	}
	for _, want := range []string{"<b>상품 문의</b>", "상품 페이지 문의", "평소 L인데 M?", ">문의 열기</a>", "tab=questions&amp;question=01HQ"} {
		if !strings.Contains(notifier.message, want) {
			t.Fatalf("alert = %q, want %q", notifier.message, want)
		}
	}
	if strings.Contains(notifier.message, "상담 요청") {
		t.Fatalf("a product question is not a consultation: %q", notifier.message)
	}
}
