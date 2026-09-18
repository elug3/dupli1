package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/notification/pkg/service"
)

type recordedNotifier struct {
	chatID   string
	message  string
	chatIDs  []string
	messages []string
	err      error
	failFor  string
}

func (r *recordedNotifier) Send(ctx context.Context, chatID string, message string) error {
	r.chatID = chatID
	r.message = message
	r.chatIDs = append(r.chatIDs, chatID)
	r.messages = append(r.messages, message)
	if r.err != nil && (r.failFor == "" || r.failFor == chatID) {
		return r.err
	}
	return nil
}

func TestDispatcherOrderCreated(t *testing.T) {
	notifier := &recordedNotifier{}
	createdAt := time.Date(2026, 8, 5, 10, 30, 0, 0, time.UTC)
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		OrderChatID:  "-100123",
		ManageWebURL: "https://manage.dupli1.com",
	})

	payload, err := json.Marshal(map[string]any{
		"event_type":   "order.created",
		"order_id":     "ORD-001",
		"customer_id":  "cust-1",
		"status":       "pending",
		"total_won":    25000,
		"subtotal_won": 25000,
		"discount_won": 0,
		"items": []map[string]any{
			{"sku": "BAG-001", "quantity": 1, "unit_price_won": 25000},
		},
		"created_at":  createdAt,
		"occurred_at": createdAt,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := dispatcher.HandleForTest(t.Context(), service.SubjectOrderCreated, payload); err != nil {
		t.Fatalf("handle order event: %v", err)
	}
	if notifier.chatID != "-100123" {
		t.Fatalf("chat id = %q, want -100123", notifier.chatID)
	}
	if !strings.Contains(notifier.message, "₩25,000") {
		t.Fatalf("expected KRW formatting in message, got %q", notifier.message)
	}
	if !strings.Contains(notifier.message, "주문 시각:") || !strings.Contains(notifier.message, "19:30 KST") {
		t.Fatalf("expected created_at in message, got %q", notifier.message)
	}
	if !strings.Contains(notifier.message, "신규 주문") {
		t.Fatalf("expected Korean new-order copy, got %q", notifier.message)
	}
	if !strings.Contains(notifier.message, "대기 중") {
		t.Fatalf("expected Korean pending status, got %q", notifier.message)
	}
	if !strings.Contains(notifier.message, `href="https://manage.dupli1.com/orders/ORD-001"`) {
		t.Fatalf("expected manage-web link in message, got %q", notifier.message)
	}
}

func TestDispatcherOrderPaid(t *testing.T) {
	notifier := &recordedNotifier{}
	createdAt := time.Date(2026, 8, 7, 1, 0, 0, 0, time.UTC)
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		OrderChatID:  "-100123",
		ManageWebURL: "https://manage.dupli1.com/",
	})

	payload, err := json.Marshal(map[string]any{
		"event_type":  "order.paid",
		"order_id":    "ORD-PAID",
		"customer_id": "cust-2",
		"status":      "paid",
		"total_won":   50000,
		"items": []map[string]any{
			{"sku": "BAG-002", "quantity": 2, "unit_price_won": 25000},
		},
		"created_at":  createdAt,
		"occurred_at": createdAt,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := dispatcher.HandleForTest(t.Context(), service.SubjectOrderPaid, payload); err != nil {
		t.Fatalf("handle order paid: %v", err)
	}
	if !strings.Contains(notifier.message, "주문 결제 완료") || !strings.Contains(notifier.message, "조치 필요") {
		t.Fatalf("expected paid alert copy, got %q", notifier.message)
	}
	if !strings.Contains(notifier.message, "결제 완료") {
		t.Fatalf("expected Korean paid status, got %q", notifier.message)
	}
	if !strings.Contains(notifier.message, "2× BAG-002") {
		t.Fatalf("expected item line, got %q", notifier.message)
	}
	if !strings.Contains(notifier.message, `href="https://manage.dupli1.com/orders/ORD-PAID"`) {
		t.Fatalf("expected trimmed manage-web link, got %q", notifier.message)
	}
}

func TestDispatcherUsesDynamicRouting(t *testing.T) {
	notifier := &recordedNotifier{}
	routing := &stubChatRouting{orderChats: []string{"-dynamic-order"}, productChats: []string{"-dynamic-product"}}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		Routing:       routing,
		OrderChatID:   "-static-order",
		ProductChatID: "-static-product",
	})

	orderPayload, _ := json.Marshal(map[string]any{
		"event_type":  "order.created",
		"order_id":    "ORD-DYN",
		"customer_id": "cust-1",
		"status":      "pending",
		"total_won":   1000,
		"occurred_at": time.Now().UTC(),
	})
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectOrderCreated, orderPayload); err != nil {
		t.Fatalf("handle order: %v", err)
	}
	if got := strings.Join(notifier.chatIDs, ","); got != "-dynamic-order,-static-order" {
		t.Fatalf("order chats = %q, want the routed chat and the static fallback", got)
	}

	productPayload, _ := json.Marshal(map[string]any{
		"event_type":  "product.created",
		"product_id":  "P-1",
		"name":        "Bag",
		"brand":       "Brand",
		"category":    "bags",
		"status":      "active",
		"price":       1000.0,
		"occurred_at": time.Now().UTC(),
	})
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectProductCreated, productPayload); err != nil {
		t.Fatalf("handle product: %v", err)
	}
	if got := strings.Join(notifier.chatIDs, ","); !strings.HasSuffix(got, "-dynamic-product,-static-product") {
		t.Fatalf("product chats = %q, want the routed chat and the static fallback", got)
	}
}

func TestDispatcherEscapesHTMLInOrderFields(t *testing.T) {
	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{OrderChatID: "-100123"})

	payload, err := json.Marshal(map[string]any{
		"event_type":  "order.created",
		"order_id":    "ORD-<script>",
		"customer_id": "cust&1",
		"status":      "pending",
		"total_won":   1000,
		"items": []map[string]any{
			{"sku": "SKU<1>", "quantity": 1, "unit_price_won": 1000},
		},
		"occurred_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := dispatcher.HandleForTest(t.Context(), service.SubjectOrderCreated, payload); err != nil {
		t.Fatalf("handle order: %v", err)
	}
	if strings.Contains(notifier.message, "<script>") {
		t.Fatalf("expected escaped order id, got %q", notifier.message)
	}
	if !strings.Contains(notifier.message, "&lt;script&gt;") {
		t.Fatalf("expected HTML escape in message, got %q", notifier.message)
	}
	if !strings.Contains(notifier.message, "SKU&lt;1&gt;") {
		t.Fatalf("expected escaped SKU, got %q", notifier.message)
	}
}

func TestDispatcherFallsBackToOccurredAt(t *testing.T) {
	notifier := &recordedNotifier{}
	occurredAt := time.Date(2026, 8, 7, 3, 15, 0, 0, time.UTC)
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{OrderChatID: "-100123"})

	payload, err := json.Marshal(map[string]any{
		"event_type":  "order.status_updated",
		"order_id":    "ORD-FALLBACK",
		"customer_id": "cust-1",
		"status":      "shipped",
		"total_won":   1000,
		"occurred_at": occurredAt,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := dispatcher.HandleForTest(t.Context(), service.SubjectOrderStatusUpdate, payload); err != nil {
		t.Fatalf("handle order update: %v", err)
	}
	if !strings.Contains(notifier.message, "주문 시각:") || !strings.Contains(notifier.message, "12:15 KST") {
		t.Fatalf("expected occurred_at fallback in message, got %q", notifier.message)
	}
	if !strings.Contains(notifier.message, "주문 변경") {
		t.Fatalf("expected Korean status-update copy, got %q", notifier.message)
	}
}

type stubChatRouting struct {
	orderChats   []string
	productChats []string
}

func (s *stubChatRouting) OrderChatIDs(_ context.Context) []string   { return s.orderChats }
func (s *stubChatRouting) ProductChatIDs(_ context.Context) []string { return s.productChats }

func TestDispatcherProductCreated(t *testing.T) {
	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		ProductChatID: "-100456",
	})

	payload, err := json.Marshal(map[string]any{
		"event_type":  "product.created",
		"product_id":  "BOT-003",
		"name":        "Tote",
		"brand":       "Bottega Veneta",
		"category":    "bags",
		"status":      "active",
		"price":       2890000.0,
		"occurred_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := dispatcher.HandleForTest(t.Context(), service.SubjectProductCreated, payload); err != nil {
		t.Fatalf("handle product event: %v", err)
	}
	if notifier.chatID != "-100456" {
		t.Fatalf("chat id = %q, want -100456", notifier.chatID)
	}
	if !strings.Contains(notifier.message, "₩2,890,000") {
		t.Fatalf("expected KRW product price, got %q", notifier.message)
	}
	if !strings.Contains(notifier.message, "상품 등록") {
		t.Fatalf("expected Korean product-created copy, got %q", notifier.message)
	}
	if !strings.Contains(notifier.message, "활성") {
		t.Fatalf("expected Korean active status, got %q", notifier.message)
	}
}

// Escaped values are interpolated into an attribute as well as into text:
// formatManageOrderLink puts the order ID inside href="…". An unescaped quote
// there would close the attribute and let the rest of the value add its own.
// Order IDs are server-generated ULIDs today, so this guards the format rather
// than a reachable input.
func TestDispatcherEscapesQuotesInTheManageLink(t *testing.T) {
	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		OrderChatID:  "-100123",
		ManageWebURL: "https://manage.dupli1.com",
	})

	payload, err := json.Marshal(map[string]any{
		"event_type":  "order.created",
		"order_id":    `ORD" onmouseover="alert(1)`,
		"customer_id": "cust-1",
		"status":      "pending",
		"total_won":   1000,
		"occurred_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectOrderCreated, payload); err != nil {
		t.Fatalf("handle order: %v", err)
	}

	if strings.Contains(notifier.message, `onmouseover="`) {
		t.Fatalf("quote escaped out of the href attribute: %q", notifier.message)
	}
	if !strings.Contains(notifier.message, "&quot;") {
		t.Fatalf("expected the quote to be escaped, got %q", notifier.message)
	}
}

func TestDispatcherOrderCreatedKoreanSnapshot(t *testing.T) {
	notifier := &recordedNotifier{}
	createdAt := time.Date(2026, 8, 5, 10, 30, 0, 0, time.UTC)
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		OrderChatID:  "-100123",
		ManageWebURL: "https://manage.dupli1.com",
	})

	payload, err := json.Marshal(map[string]any{
		"event_type":  "order.created",
		"order_id":    "ORD-001",
		"customer_id": "cust-1",
		"status":      "pending",
		"total_won":   25000,
		"items": []map[string]any{
			{"sku": "BAG-001", "quantity": 1, "unit_price_won": 25000},
		},
		"created_at":  createdAt,
		"occurred_at": createdAt,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectOrderCreated, payload); err != nil {
		t.Fatalf("handle order: %v", err)
	}

	want := "🛒 <b>신규 주문</b> ORD-001\n" +
		"주문 시각: <b>2026-08-05 19:30 KST</b>\n" +
		"<a href=\"https://manage.dupli1.com/orders/ORD-001\">관리자에서 주문 보기</a>\n" +
		"상태: <b>대기 중</b>\n" +
		"고객: cust-1\n" +
		"상품: 1× BAG-001\n" +
		"합계: <b>₩25,000</b>"
	if notifier.message != want {
		t.Fatalf("korean order.created snapshot mismatch:\n got: %q\nwant: %q", notifier.message, want)
	}
}

func TestDispatcherTranslatesOrderStatuses(t *testing.T) {
	cases := map[string]string{
		"pending":    "대기 중",
		"paid":       "결제 완료",
		"confirmed":  "확인됨",
		"in_transit": "배송 중",
		"delivered":  "배송 완료",
		"fulfilled":  "완료",
		"disputed":   "분쟁",
		"canceled":   "취소됨",
	}
	for status, label := range cases {
		t.Run(status, func(t *testing.T) {
			notifier := &recordedNotifier{}
			dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{OrderChatID: "-100123"})
			payload, err := json.Marshal(map[string]any{
				"event_type":  "order.status_updated",
				"order_id":    "ORD-ST",
				"customer_id": "cust-1",
				"status":      status,
				"total_won":   1000,
			})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if err := dispatcher.HandleForTest(t.Context(), service.SubjectOrderStatusUpdate, payload); err != nil {
				t.Fatalf("handle: %v", err)
			}
			if !strings.Contains(notifier.message, "상태: <b>"+label+"</b>") {
				t.Fatalf("status %q: expected %q in %q", status, label, notifier.message)
			}
		})
	}
}

func TestDispatcherEmptyItemsUsesKoreanPlaceholder(t *testing.T) {
	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{OrderChatID: "-100123"})
	payload, err := json.Marshal(map[string]any{
		"event_type":  "order.created",
		"order_id":    "ORD-EMPTY",
		"customer_id": "cust-1",
		"status":      "pending",
		"total_won":   0,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectOrderCreated, payload); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !strings.Contains(notifier.message, "상품: 상품 없음") {
		t.Fatalf("expected empty-items placeholder, got %q", notifier.message)
	}
}
