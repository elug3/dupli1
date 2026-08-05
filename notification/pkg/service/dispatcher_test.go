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
