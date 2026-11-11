package service_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elug3/dupli1/notification/pkg/service"
)

func dispatch(t *testing.T, subject string, payload map[string]any) string {
	t.Helper()
	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{OrderChatID: "-1", ProductChatID: "-2"})
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.HandleForTest(t.Context(), subject, raw); err != nil {
		t.Fatalf("handle %s: %v", subject, err)
	}
	return notifier.message
}

func wantLines(t *testing.T, message string, lines ...string) {
	t.Helper()
	for _, line := range lines {
		if !strings.Contains(message, line) {
			t.Fatalf("message lacks %q:\n%s", line, message)
		}
	}
}

func TestOrderStatusUpdateShowsPreviousStatusAndTracking(t *testing.T) {
	msg := dispatch(t, service.SubjectOrderStatusUpdate, map[string]any{
		"order_id": "ORD-1", "customer_id": "c-1", "status": "in_transit", "total_won": 1000,
		"previous_status": "confirmed", "change": "shipped",
		"carrier": "cj", "tracking_number": "123456789012",
	})
	wantLines(t, msg,
		"<b>주문 변경 · 출고</b> ORD-1",
		"상태: 확인됨 → <b>배송 중</b>",
		"송장: CJ대한통운 123456789012",
	)
}

// A cancel request leaves the status alone; the alert says what happened and
// why, instead of a bare "상태: 확인됨".
func TestOrderStatusUpdateCancelRequest(t *testing.T) {
	msg := dispatch(t, service.SubjectOrderStatusUpdate, map[string]any{
		"order_id": "ORD-1", "customer_id": "c-1", "status": "confirmed", "total_won": 1000,
		"previous_status": "confirmed", "change": "cancel_requested", "reason": "사이즈 <교환>",
	})
	wantLines(t, msg,
		"<b>주문 변경 · 취소 요청 접수</b>",
		"상태: <b>확인됨</b> (변경 없음)",
		"취소 사유: 사이즈 &lt;교환&gt;",
	)
}

func TestOrderStatusUpdateAutomatic(t *testing.T) {
	msg := dispatch(t, service.SubjectOrderStatusUpdate, map[string]any{
		"order_id": "ORD-1", "customer_id": "c-1", "status": "confirmed", "total_won": 1000,
		"previous_status": "paid", "change": "confirmed", "automatic": true,
	})
	wantLines(t, msg,
		"<b>주문 변경 · 주문 확정 (자동)</b>",
		"상태: 결제 완료 → <b>확인됨</b>",
		"처리: 2시간 안에 확정하지 않아 자동 확정",
	)
}

// An event from before the change fields still renders as it did.
func TestOrderStatusUpdateLegacyEvent(t *testing.T) {
	msg := dispatch(t, service.SubjectOrderStatusUpdate, map[string]any{
		"order_id": "ORD-1", "customer_id": "c-1", "status": "in_transit", "total_won": 1000,
	})
	wantLines(t, msg, "<b>주문 변경</b> ORD-1", "상태: <b>배송 중</b>")
	if strings.Contains(msg, "→") {
		t.Fatalf("legacy event shows a transition:\n%s", msg)
	}
}

func TestProductUpdatedShowsEachChange(t *testing.T) {
	msg := dispatch(t, service.SubjectProductUpdated, map[string]any{
		"product_id": "BOT-001", "name": "Cassette", "brand": "Bottega", "status": "draft", "price": 990000,
		"changes": []map[string]string{
			{"field": "price", "before": "1200000", "after": "990000"},
			{"field": "status", "before": "active", "after": "draft"},
			{"field": "attributes.care", "before": "", "after": "dry <clean>"},
		},
	})
	wantLines(t, msg,
		"<b>상품 수정</b>",
		"BOT-001 — Cassette (Bottega)",
		"판매가: ₩1,200,000 → <b>₩990,000</b>",
		"상태: 활성 → <b>초안</b>",
		"속성 care: (없음) → <b>dry &lt;clean&gt;</b>",
	)
}

func TestProductUpdatedAutoDraft(t *testing.T) {
	msg := dispatch(t, service.SubjectProductUpdated, map[string]any{
		"product_id": "BOT-001", "name": "Cassette", "status": "draft", "reason": "auto_draft",
		"changes": []map[string]string{{"field": "status", "before": "active", "after": "draft"}},
	})
	wantLines(t, msg, "<b>상품 자동 초안 전환</b>", "판매 중인 SKU가 없어", "상태: 활성 → <b>초안</b>")
}

func TestProductUpdatedWithoutChangesShowsCurrentValues(t *testing.T) {
	msg := dispatch(t, service.SubjectProductUpdated, map[string]any{
		"product_id": "BOT-001", "name": "Cassette", "status": "active", "price": 2500,
	})
	wantLines(t, msg, "<b>상품 수정</b>", "상태: 활성", "가격: ₩2,500")
}

func TestProductUpdatedCapsLongEdits(t *testing.T) {
	changes := make([]map[string]string, 0, 25)
	for i := 0; i < 25; i++ {
		changes = append(changes, map[string]string{"field": "attributes.k" + string(rune('a'+i)), "before": "", "after": strings.Repeat("x", 500)})
	}
	msg := dispatch(t, service.SubjectProductUpdated, map[string]any{
		"product_id": "BOT-001", "name": "Cassette", "changes": changes,
	})
	wantLines(t, msg, "외 5개 항목 변경")
	if len([]rune(msg)) > 4096 {
		t.Fatalf("message is %d runes, over Telegram's limit", len([]rune(msg)))
	}
}
