package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/order/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/events"
)

// relaySubscriber records a handler per subject, unlike recordingSubscriber
// which keeps only the last — the relay registers three.
type relaySubscriber struct {
	handlers map[string]ports.MessageHandler
}

func (r *relaySubscriber) Subscribe(_ context.Context, subject string, handler ports.MessageHandler) error {
	if r.handlers == nil {
		r.handlers = map[string]ports.MessageHandler{}
	}
	r.handlers[subject] = handler
	return nil
}

func (r *relaySubscriber) Close() {}

type sinkEvent struct {
	id        string
	eventType string
	payload   []byte
}

// recordingSink captures what the relay hands to the SSE hub.
type recordingSink struct {
	events []sinkEvent
	resets []string
}

func (s *recordingSink) PublishOrderEvent(id, eventType string, payload []byte) {
	s.events = append(s.events, sinkEvent{id: id, eventType: eventType, payload: payload})
}

func (s *recordingSink) PublishReset(id string) { s.resets = append(s.resets, id) }

func seedOrderForRelay(t *testing.T, repo *memory.Repository, id string) *domain.Order {
	t.Helper()
	now := time.Now().UTC()
	order, err := domain.NewOrder(id, "cust-7", "res-"+id, []domain.OrderItem{
		{SKU: "BAG-1", Quantity: 2, UnitPriceWon: 120000},
	}, "", 0, 30000, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := repo.Save(t.Context(), order); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return order
}

func relayFor(t *testing.T) (*service.Service, *memory.Repository, *relaySubscriber, *recordingSink) {
	t.Helper()
	repo := memory.NewRepository()
	svc := service.New(repo, &fakeStock{})
	sub := &relaySubscriber{}
	sink := &recordingSink{}
	if err := svc.RegisterOrderEventRelay(t.Context(), sub, sink); err != nil {
		t.Fatalf("RegisterOrderEventRelay: %v", err)
	}
	return svc, repo, sub, sink
}

func TestOrderEventRelay_SubscribesToEveryOrderSubject(t *testing.T) {
	_, _, sub, _ := relayFor(t)

	for _, subject := range []string{events.OrderCreated, events.OrderPaid, events.OrderStatusUpdate} {
		if sub.handlers[subject] == nil {
			t.Errorf("no live-stream handler registered for %s", subject)
		}
	}
}

// The stream carries the same order representation as GET /orders/{id} so the
// admin table can render a row without a follow-up request.
func TestOrderEventRelay_SendsFullOrderSnapshot(t *testing.T) {
	_, repo, sub, sink := relayFor(t)
	order := seedOrderForRelay(t, repo, "ord_relay_1")

	occurred := time.Date(2026, 9, 13, 4, 5, 6, 123456789, time.UTC)
	payload, err := json.Marshal(events.Order{
		EventType: events.OrderCreated,
		OrderID:   order.ID,
		Occurred:  occurred,
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	if err := sub.handlers[events.OrderCreated](t.Context(), events.OrderCreated, payload); err != nil {
		t.Fatalf("relay handler: %v", err)
	}

	if len(sink.events) != 1 {
		t.Fatalf("want 1 streamed event, got %d (resets: %v)", len(sink.events), sink.resets)
	}
	got := sink.events[0]
	if want := "1789272306123456789"; got.id != want {
		t.Errorf("id = %q, want %q (occurred_at in unix nanos)", got.id, want)
	}
	if got.eventType != "order" {
		t.Errorf("eventType = %q, want \"order\"", got.eventType)
	}

	var body struct {
		Type  string        `json:"type"`
		Order *domain.Order `json:"order"`
	}
	if err := json.Unmarshal(got.payload, &body); err != nil {
		t.Fatalf("decode streamed payload: %v", err)
	}
	if body.Type != events.OrderCreated {
		t.Errorf("type = %q, want %q", body.Type, events.OrderCreated)
	}
	if body.Order == nil {
		t.Fatal("streamed payload carries no order")
	}
	if body.Order.ID != order.ID || body.Order.CustomerID != "cust-7" {
		t.Errorf("order = %s/%s, want %s/cust-7", body.Order.ID, body.Order.CustomerID, order.ID)
	}
	if body.Order.TotalWon != order.TotalWon {
		t.Errorf("total_won = %d, want %d", body.Order.TotalWon, order.TotalWon)
	}
	// A single data line is what the SSE framing depends on.
	for _, b := range got.payload {
		if b == '\n' {
			t.Fatal("payload contains a literal newline; it would break SSE framing")
		}
	}
}

// Without a snapshot the row on screen is known-stale, so clients are told to
// reload rather than left with it.
func TestOrderEventRelay_UnknownOrderTriggersReset(t *testing.T) {
	_, _, sub, sink := relayFor(t)

	payload, err := json.Marshal(events.Order{
		EventType: events.OrderPaid,
		OrderID:   "ord_missing",
		Occurred:  time.Unix(0, 4242).UTC(),
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	if err := sub.handlers[events.OrderPaid](t.Context(), events.OrderPaid, payload); err == nil {
		t.Fatal("want an error for an order the service cannot load")
	}
	if len(sink.events) != 0 {
		t.Fatalf("want no streamed snapshot, got %d", len(sink.events))
	}
	if len(sink.resets) != 1 || sink.resets[0] != "4242" {
		t.Fatalf("want one reset at id 4242, got %v", sink.resets)
	}
}

func TestOrderEventRelay_RejectsEventWithoutOrderID(t *testing.T) {
	_, _, sub, sink := relayFor(t)

	payload := []byte(`{"event_type":"order.created"}`)
	if err := sub.handlers[events.OrderCreated](t.Context(), events.OrderCreated, payload); err == nil {
		t.Fatal("want an error for an event with no order_id")
	}
	if len(sink.events) != 0 || len(sink.resets) != 0 {
		t.Fatalf("nothing should reach clients: events=%d resets=%d", len(sink.events), len(sink.resets))
	}
}

// occurred_at is what positions an event for replay; older publishers that only
// set created_at must still produce a usable cursor.
func TestOrderEventRelay_FallsBackToCreatedAtForCursor(t *testing.T) {
	_, repo, sub, sink := relayFor(t)
	order := seedOrderForRelay(t, repo, "ord_relay_2")

	created := time.Date(2026, 9, 13, 0, 0, 0, 7, time.UTC)
	payload, err := json.Marshal(events.Order{
		EventType: events.OrderStatusUpdate,
		OrderID:   order.ID,
		CreatedAt: created,
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	if err := sub.handlers[events.OrderStatusUpdate](t.Context(), events.OrderStatusUpdate, payload); err != nil {
		t.Fatalf("relay handler: %v", err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("want 1 streamed event, got %d", len(sink.events))
	}
	if want := "1789257600000000007"; sink.events[0].id != want {
		t.Errorf("id = %q, want %q (created_at fallback)", sink.events[0].id, want)
	}
}

func TestOrderEventRelay_UntimedEventIsDeliveredWithoutCursor(t *testing.T) {
	_, repo, sub, sink := relayFor(t)
	order := seedOrderForRelay(t, repo, "ord_relay_3")

	payload, err := json.Marshal(events.Order{EventType: events.OrderCreated, OrderID: order.ID})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	if err := sub.handlers[events.OrderCreated](t.Context(), events.OrderCreated, payload); err != nil {
		t.Fatalf("relay handler: %v", err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("want 1 streamed event, got %d", len(sink.events))
	}
	if sink.events[0].id != "" {
		t.Errorf("id = %q, want empty so the client cursor does not move", sink.events[0].id)
	}
}

func TestOrderEventRelay_NilSinkIsANoop(t *testing.T) {
	repo := memory.NewRepository()
	svc := service.New(repo, &fakeStock{})
	sub := &relaySubscriber{}

	if err := svc.RegisterOrderEventRelay(t.Context(), sub, nil); err != nil {
		t.Fatalf("RegisterOrderEventRelay with no sink: %v", err)
	}
	if len(sub.handlers) != 0 {
		t.Fatalf("want no subscriptions without a sink, got %d", len(sub.handlers))
	}
}
