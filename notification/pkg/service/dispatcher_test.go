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
	chatID  string
	message string
}

func (r *recordedNotifier) Send(ctx context.Context, chatID string, message string) error {
	r.chatID = chatID
	r.message = message
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
		"event_type":     "order.created",
		"order_id":       "ORD-001",
		"customer_id":    "cust-1",
		"status":         "pending",
		"total_cents":    25000,
		"subtotal_cents": 25000,
		"discount_cents": 0,
		"items": []map[string]any{
			{"sku": "BAG-001", "quantity": 1, "unit_price_cents": 25000},
		},
		"created_at":  createdAt,
		"occurred_at": createdAt,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := dispatcher.HandleForTest(context.Background(), service.SubjectOrderCreated, payload); err != nil {
		t.Fatalf("handle order event: %v", err)
	}
	if notifier.chatID != "-100123" {
		t.Fatalf("chat id = %q, want -100123", notifier.chatID)
	}
	if !strings.Contains(notifier.message, "₩25,000") {
		t.Fatalf("expected KRW formatting in message, got %q", notifier.message)
	}
	if !strings.Contains(notifier.message, "Created:") || !strings.Contains(notifier.message, "19:30 KST") {
		t.Fatalf("expected created_at in message, got %q", notifier.message)
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
		"total_cents": 50000,
		"items": []map[string]any{
			{"sku": "BAG-002", "quantity": 2, "unit_price_cents": 25000},
		},
		"created_at":  createdAt,
		"occurred_at": createdAt,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := dispatcher.HandleForTest(context.Background(), service.SubjectOrderPaid, payload); err != nil {
		t.Fatalf("handle order paid: %v", err)
	}
	if !strings.Contains(notifier.message, "Order paid") || !strings.Contains(notifier.message, "action required") {
		t.Fatalf("expected paid alert copy, got %q", notifier.message)
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
	routing := &stubChatRouting{orderChat: "-dynamic-order", productChat: "-dynamic-product"}
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
		"total_cents": 1000,
		"occurred_at": time.Now().UTC(),
	})
	if err := dispatcher.HandleForTest(context.Background(), service.SubjectOrderCreated, orderPayload); err != nil {
		t.Fatalf("handle order: %v", err)
	}
	if notifier.chatID != "-dynamic-order" {
		t.Fatalf("order chat = %q, want dynamic routing", notifier.chatID)
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
	if err := dispatcher.HandleForTest(context.Background(), service.SubjectProductCreated, productPayload); err != nil {
		t.Fatalf("handle product: %v", err)
	}
	if notifier.chatID != "-dynamic-product" {
		t.Fatalf("product chat = %q, want dynamic routing", notifier.chatID)
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
		"total_cents": 1000,
		"items": []map[string]any{
			{"sku": "SKU<1>", "quantity": 1, "unit_price_cents": 1000},
		},
		"occurred_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := dispatcher.HandleForTest(context.Background(), service.SubjectOrderCreated, payload); err != nil {
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
		"total_cents": 1000,
		"occurred_at": occurredAt,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := dispatcher.HandleForTest(context.Background(), service.SubjectOrderStatusUpdate, payload); err != nil {
		t.Fatalf("handle order update: %v", err)
	}
	if !strings.Contains(notifier.message, "Created:") || !strings.Contains(notifier.message, "12:15 KST") {
		t.Fatalf("expected occurred_at fallback in message, got %q", notifier.message)
	}
}

type stubChatRouting struct {
	orderChat   string
	productChat string
}

func (s *stubChatRouting) OrderChatID(_ context.Context) string   { return s.orderChat }
func (s *stubChatRouting) ProductChatID(_ context.Context) string { return s.productChat }

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

	if err := dispatcher.HandleForTest(context.Background(), service.SubjectProductCreated, payload); err != nil {
		t.Fatalf("handle product event: %v", err)
	}
	if notifier.chatID != "-100456" {
		t.Fatalf("chat id = %q, want -100456", notifier.chatID)
	}
	if !strings.Contains(notifier.message, "₩2,890,000") {
		t.Fatalf("expected KRW product price, got %q", notifier.message)
	}
}
