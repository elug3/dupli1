package events_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/events"
)

func TestSubjectConstants(t *testing.T) {
	// Cross-service NATS wiring depends on these exact strings; a typo breaks
	// the publisher/subscriber pair silently (messages never delivered).
	cases := map[string]string{
		"OrderCreated":            events.OrderCreated,
		"OrderStatusUpdate":       events.OrderStatusUpdate,
		"OrderPaid":               events.OrderPaid,
		"PaymentSucceeded":        events.PaymentSucceeded,
		"PaymentCanceled":         events.PaymentCanceled,
		"PaymentCallbackRejected": events.PaymentCallbackRejected,
		"ProductCreated":          events.ProductCreated,
		"ProductUpdated":          events.ProductUpdated,
		"ProductDeleted":          events.ProductDeleted,
		"ProductImage":            events.ProductImage,
		"UserDeleted":             events.UserDeleted,
	}
	for name, subject := range cases {
		if subject == "" {
			t.Fatalf("%s subject is empty", name)
		}
		if strings.Contains(subject, " ") {
			t.Fatalf("%s subject %q contains whitespace", name, subject)
		}
	}
}

func TestOrderJSONRoundTrip(t *testing.T) {
	occurred := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	created := occurred.Add(-5 * time.Minute)
	orig := events.Order{
		EventType:   events.OrderCreated,
		OrderID:     "ord_01",
		CustomerID:  "cust_01",
		Status:      "pending",
		SubtotalWon: 120000,
		DiscountWon: 5000,
		TotalWon:    115000,
		Items: []events.OrderItem{
			{SkuID: "01HXYZ", SKU: "PRADA_GALLERIA_BLK_M", Quantity: 1, UnitPriceWon: 115000},
		},
		CreatedAt: created,
		Occurred:  occurred,
	}

	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("Unmarshal wire: %v", err)
	}
	for _, key := range []string{"order_id", "customer_id", "subtotal_won", "discount_won", "total_won", "created_at", "occurred_at"} {
		if _, ok := wire[key]; !ok {
			t.Fatalf("missing snake_case field %q in %s", key, string(raw))
		}
	}
	items, ok := wire["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %v, want one element", wire["items"])
	}
	item := items[0].(map[string]any)
	if item["sku_id"] != "01HXYZ" || item["unit_price_won"] != float64(115000) {
		t.Fatalf("item wire = %v", item)
	}

	var decoded events.Order
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal Order: %v", err)
	}
	if decoded.OrderID != orig.OrderID || decoded.TotalWon != orig.TotalWon {
		t.Fatalf("decoded = %+v, want %+v", decoded, orig)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].SkuID != "01HXYZ" {
		t.Fatalf("decoded items = %+v", decoded.Items)
	}
}

func TestPaymentSucceededEventJSONRoundTrip(t *testing.T) {
	orig := events.PaymentSucceededEvent{
		EventType: events.PaymentSucceeded,
		OrderID:   "ord_pay",
		PaymentID: "pay_nano_1",
		AmountWon: 99000,
	}

	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, key := range []string{"event_type", "order_id", "payment_id", "amount_won"} {
		if !strings.Contains(string(raw), `"`+key+`"`) {
			t.Fatalf("missing %q in %s", key, string(raw))
		}
	}

	var decoded events.PaymentSucceededEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.OrderID != orig.OrderID || decoded.PaymentID != orig.PaymentID || decoded.AmountWon != orig.AmountWon {
		t.Fatalf("decoded = %+v, want %+v", decoded, orig)
	}
}

func TestPaymentCanceledEventJSONRoundTrip(t *testing.T) {
	orig := events.PaymentCanceledEvent{
		EventType:    events.PaymentCanceled,
		OrderID:      "ord_pay",
		PaymentID:    "pay_nano_1",
		AmountWon:    70000,
		RemainingWon: 0,
		Reason:       "ops reject",
		CanceledBy:   "mgr_1",
		Occurred:     time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC),
	}

	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, key := range []string{"event_type", "order_id", "payment_id", "amount_won", "remaining_won"} {
		if !strings.Contains(string(raw), `"`+key+`"`) {
			t.Fatalf("missing %q in %s", key, string(raw))
		}
	}

	var decoded events.PaymentCanceledEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.OrderID != orig.OrderID || decoded.PaymentID != orig.PaymentID || decoded.AmountWon != orig.AmountWon {
		t.Fatalf("decoded = %+v, want %+v", decoded, orig)
	}
	if decoded.RemainingWon != 0 || !decoded.RemainingSpecified() {
		t.Fatalf("remaining = %d specified=%t, want 0 / true", decoded.RemainingWon, decoded.RemainingSpecified())
	}
}

func TestPaymentCanceledEvent_OmittedRemainingWonIsNotSpecified(t *testing.T) {
	raw := []byte(`{"event_type":"payment.canceled","order_id":"ord_1","payment_id":"pay_1","amount_won":1000}`)
	var decoded events.PaymentCanceledEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.RemainingSpecified() {
		t.Fatal("omitted remaining_won must not count as specified")
	}
	if decoded.RemainingWon != 0 {
		t.Fatalf("omitted remaining unmarshals to %d, want 0", decoded.RemainingWon)
	}
}

func TestPaymentSucceededEvent_LegacyAmountCents(t *testing.T) {
	raw := []byte(`{"event_type":"payment.succeeded","order_id":"ord_1","payment_id":"pay_1","amount_cents":99000}`)
	var decoded events.PaymentSucceededEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.AmountWon != 99000 {
		t.Fatalf("legacy amount_cents = %d, want 99000", decoded.AmountWon)
	}
}

func TestPaymentSucceededEvent_AmountWonWinsOverCents(t *testing.T) {
	raw := []byte(`{"event_type":"payment.succeeded","order_id":"ord_1","payment_id":"pay_1","amount_won":1000,"amount_cents":99000}`)
	var decoded events.PaymentSucceededEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.AmountWon != 1000 {
		t.Fatalf("amount_won must win over amount_cents: got %d", decoded.AmountWon)
	}
}

func TestPaymentSucceededEvent_LegacyAmountKRW(t *testing.T) {
	raw := []byte(`{"event_type":"payment.succeeded","order_id":"ord_1","payment_id":"pay_1","amount_krw":99000}`)
	var decoded events.PaymentSucceededEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.AmountWon != 99000 {
		t.Fatalf("legacy amount_krw = %d, want 99000", decoded.AmountWon)
	}
}

func TestPaymentSucceededEvent_AmountWonWinsOverKRW(t *testing.T) {
	raw := []byte(`{"event_type":"payment.succeeded","order_id":"ord_1","payment_id":"pay_1","amount_won":1000,"amount_krw":99000}`)
	var decoded events.PaymentSucceededEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.AmountWon != 1000 {
		t.Fatalf("amount_won must win over amount_krw: got %d", decoded.AmountWon)
	}
}

func TestOrderEvent_LegacyKRWFields(t *testing.T) {
	raw := []byte(`{"event_type":"order.created","order_id":"ord_1","customer_id":"c1","status":"pending","subtotal_krw":120000,"discount_krw":0,"shipping_fee_krw":30000,"total_krw":150000,"items":[{"sku":"BAG-1","quantity":1,"unit_price_krw":120000}]}`)
	var decoded events.Order
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.SubtotalWon != 120000 || decoded.ShippingFeeWon != 30000 || decoded.TotalWon != 150000 {
		t.Fatalf("legacy *_krw order = %+v", decoded)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].UnitPriceWon != 120000 {
		t.Fatalf("legacy item unit_price_krw = %+v", decoded.Items)
	}
}

func TestPaymentCanceledEvent_ExplicitZeroRemainingIsSpecified(t *testing.T) {
	raw := []byte(`{"event_type":"payment.canceled","order_id":"ord_1","payment_id":"pay_1","amount_won":1000,"remaining_won":0}`)
	var decoded events.PaymentCanceledEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !decoded.RemainingSpecified() || decoded.RemainingWon != 0 {
		t.Fatalf("explicit 0 remaining: specified=%t remaining=%d", decoded.RemainingSpecified(), decoded.RemainingWon)
	}
}

func TestPaymentCanceledEvent_LegacyRemainingCentsIsSpecified(t *testing.T) {
	raw := []byte(`{"event_type":"payment.canceled","order_id":"ord_1","payment_id":"pay_1","amount_cents":1000,"remaining_cents":0}`)
	var decoded events.PaymentCanceledEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.AmountWon != 1000 {
		t.Fatalf("legacy amount_cents = %d, want 1000", decoded.AmountWon)
	}
	if !decoded.RemainingSpecified() || decoded.RemainingWon != 0 {
		t.Fatalf("legacy remaining_cents: specified=%t remaining=%d", decoded.RemainingSpecified(), decoded.RemainingWon)
	}
}

func TestPaymentCanceledEvent_RemainingWonWinsOverCents(t *testing.T) {
	raw := []byte(`{"event_type":"payment.canceled","order_id":"ord_1","payment_id":"pay_1","amount_won":1000,"remaining_won":500,"remaining_cents":0}`)
	var decoded events.PaymentCanceledEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !decoded.RemainingSpecified() || decoded.RemainingWon != 500 {
		t.Fatalf("remaining_won must win: specified=%t remaining=%d", decoded.RemainingSpecified(), decoded.RemainingWon)
	}
}

func TestProductJSONRoundTrip(t *testing.T) {
	occurred := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	orig := events.Product{
		EventType: events.ProductUpdated,
		ProductID: "prod_01",
		SKU:       "PRADA_GALLERIA_BLK_M",
		Name:      "Prada Galleria",
		Brand:     "Prada",
		Category:  "handbag",
		Status:    "active",
		Price:     1200000,
		ImageURL:  "https://images.dupli1.com/x.jpg",
		Occurred:  occurred,
	}

	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, key := range []string{"product_id", "image_url", "occurred_at"} {
		if !strings.Contains(string(raw), `"`+key+`"`) {
			t.Fatalf("missing %q in %s", key, string(raw))
		}
	}

	var decoded events.Product
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.ProductID != orig.ProductID || decoded.ImageURL != orig.ImageURL || decoded.Price != orig.Price {
		t.Fatalf("decoded = %+v, want %+v", decoded, orig)
	}
}

func TestUserDeletedEventJSONRoundTrip(t *testing.T) {
	occurred := time.Date(2026, 9, 4, 3, 50, 0, 0, time.UTC)
	orig := events.UserDeletedEvent{
		EventType: events.UserDeleted,
		UserID:    "user_01",
		Occurred:  occurred,
	}

	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, key := range []string{"event_type", "user_id", "occurred_at"} {
		if !strings.Contains(string(raw), `"`+key+`"`) {
			t.Fatalf("missing %q in %s", key, string(raw))
		}
	}

	var decoded events.UserDeletedEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.UserID != orig.UserID || decoded.EventType != orig.EventType {
		t.Fatalf("decoded = %+v, want %+v", decoded, orig)
	}
	if decoded.Occurred.IsZero() {
		t.Fatal("occurred_at must round-trip")
	}
}

func TestPaymentCallbackRejectedJSONRoundTrip(t *testing.T) {
	occurred := time.Date(2026, 9, 5, 11, 30, 0, 0, time.UTC)
	orig := events.PaymentCallbackRejectedEvent{
		EventType:      events.PaymentCallbackRejected,
		Provider:       "nano",
		Source:         "return",
		PaymentID:      "pay_000023",
		OrderID:        "ord_000023",
		Reason:         "verify_failed",
		ResultCode:     "0000",
		ExpectedWon:    31004,
		ReportedAmount: "31004",
		TranNo:         "260905001496",
		Detail:         "callback hash did not verify",
		Occurred:       occurred,
	}

	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("Unmarshal wire: %v", err)
	}
	// The subscriber reads these exact keys; renaming one silently empties the alert.
	for _, key := range []string{"event_type", "provider", "source", "payment_id", "order_id", "reason", "result_code", "expected_won", "reported_amount", "occurred_at"} {
		if _, ok := wire[key]; !ok {
			t.Errorf("wire payload missing %q: %s", key, raw)
		}
	}

	var back events.PaymentCallbackRejectedEvent
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if back != orig {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", back, orig)
	}
}

func TestPaymentCallbackRejectedOmitsUnknownPaymentFields(t *testing.T) {
	// A callback can be rejected because it identified no payment at all; the
	// alert must still be publishable without inventing ids or amounts.
	raw, err := json.Marshal(events.PaymentCallbackRejectedEvent{
		EventType: events.PaymentCallbackRejected,
		Provider:  "nano",
		Source:    "webhook",
		Reason:    "unknown_payment",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, key := range []string{"payment_id", "order_id", "expected_won", "reported_amount", "tran_no"} {
		if strings.Contains(string(raw), `"`+key+`"`) {
			t.Errorf("empty %s should be omitted: %s", key, raw)
		}
	}
}
