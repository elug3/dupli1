package service_test

import (
	"strings"
	"testing"

	"github.com/elug3/dupli1/notification/pkg/service"
)

// Every order alert about a bypass order opens with the marker, so it cannot
// be read as a genuine card sale even from the notification preview.
func TestBypassOrderAlertsAreMarked(t *testing.T) {
	for _, subject := range []string{service.SubjectOrderCreated, service.SubjectOrderPaid, service.SubjectOrderStatusUpdate} {
		msg := dispatch(t, subject, map[string]any{
			"order_id": "ORD-1", "customer_id": "c-1", "status": "paid", "total_won": 1000,
			"payment_method": "bypass",
		})
		if !strings.HasPrefix(msg, "🧪 <b>[바이패스 주문]</b>") {
			t.Fatalf("%s: bypass order not marked first:\n%s", subject, msg)
		}
	}
}

func TestCardAndLegacyOrderAlertsAreNotMarked(t *testing.T) {
	for _, method := range []string{"credit_card", ""} {
		msg := dispatch(t, service.SubjectOrderPaid, map[string]any{
			"order_id": "ORD-1", "customer_id": "c-1", "status": "paid", "total_won": 1000,
			"payment_method": method,
		})
		if strings.Contains(msg, "바이패스") {
			t.Fatalf("method %q marked as bypass:\n%s", method, msg)
		}
	}
}

func TestBypassRefundIsMarked(t *testing.T) {
	msg := dispatch(t, service.SubjectPaymentCanceled, map[string]any{
		"order_id": "ORD-1", "payment_id": "pay-1", "amount_won": 1000, "remaining_won": 0,
		"method": "bypass",
	})
	if !strings.HasPrefix(msg, "🧪 <b>[바이패스 결제]</b>") || !strings.Contains(msg, "전액 환불") {
		t.Fatalf("bypass refund not marked:\n%s", msg)
	}
	card := dispatch(t, service.SubjectPaymentCanceled, map[string]any{
		"order_id": "ORD-1", "payment_id": "pay-1", "amount_won": 1000, "remaining_won": 0,
		"method": "credit_card",
	})
	if strings.Contains(card, "바이패스") {
		t.Fatalf("card refund marked as bypass:\n%s", card)
	}
}
