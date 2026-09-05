package service_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elug3/dupli1/notification/pkg/service"
)

func dispatchPaymentCanceled(t *testing.T, payload map[string]any) *recordedNotifier {
	t.Helper()
	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		OrderChatID:  "-100123",
		ManageWebURL: "https://manage.dupli1.com",
	})
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectPaymentCanceled, raw); err != nil {
		t.Fatalf("HandleForTest: %v", err)
	}
	return notifier
}

// Refunds used to be silent: order.* covered creation and payment, so money
// going back to a customer produced no alert at all.
func TestDispatcher_FullRefundAlert(t *testing.T) {
	n := dispatchPaymentCanceled(t, map[string]any{
		"event_type": "payment.canceled", "order_id": "ORD-001", "payment_id": "pay_1",
		"amount_cents": 280000, "remaining_cents": 0,
		"reason": "ops reject", "canceled_by": "mgr-1",
	})

	if n.chatID != "-100123" {
		t.Fatalf("chat id = %q", n.chatID)
	}
	for _, want := range []string{"Refunded in full", "ORD-001", "₩280,000", "ops reject", "mgr-1", "canceled"} {
		if !strings.Contains(n.message, want) {
			t.Fatalf("message missing %q:\n%s", want, n.message)
		}
	}
}

// Ops must be able to tell the two apart: a partial refund leaves the order
// standing with money still owed, and it does not cancel itself.
func TestDispatcher_PartialRefundAlertDiffersFromFull(t *testing.T) {
	n := dispatchPaymentCanceled(t, map[string]any{
		"event_type": "payment.canceled", "order_id": "ORD-002", "payment_id": "pay_2",
		"amount_cents": 20000, "remaining_cents": 50000,
	})

	for _, want := range []string{"Partial refund", "₩20,000", "₩50,000", "unchanged"} {
		if !strings.Contains(n.message, want) {
			t.Fatalf("message missing %q:\n%s", want, n.message)
		}
	}
	if strings.Contains(n.message, "Refunded in full") {
		t.Fatalf("partial refund must not read as full:\n%s", n.message)
	}
}

// Optional fields absent should not leave dangling labels in the message.
func TestDispatcher_RefundAlertOmitsEmptyFields(t *testing.T) {
	n := dispatchPaymentCanceled(t, map[string]any{
		"event_type": "payment.canceled", "order_id": "ORD-003", "payment_id": "pay_3",
		"amount_cents": 1000, "remaining_cents": 0,
	})
	for _, unwanted := range []string{"Reason:", "By:"} {
		if strings.Contains(n.message, unwanted) {
			t.Fatalf("message should omit %q when unset:\n%s", unwanted, n.message)
		}
	}
}

// A chat that is not configured must be skipped, not treated as an error that
// wedges redelivery.
func TestDispatcher_RefundAlertSkippedWithoutChat(t *testing.T) {
	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{})
	raw, _ := json.Marshal(map[string]any{
		"event_type": "payment.canceled", "order_id": "ORD-004",
		"payment_id": "pay_4", "amount_cents": 1000,
	})
	if err := dispatcher.HandleForTest(t.Context(), service.SubjectPaymentCanceled, raw); err != nil {
		t.Fatalf("unconfigured chat should be skipped, got %v", err)
	}
	if notifier.message != "" {
		t.Fatalf("nothing should be sent, got %q", notifier.message)
	}
}
