package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/events"
)

// sseOrderEventName is the SSE event name carrying an order snapshot.
const sseOrderEventName = "order"

// liveOrderEvent is the SSE data payload: the subject that fired plus the same
// order representation GET /api/v1/orders/{id} returns, so a live client needs
// no follow-up request to render the row.
type liveOrderEvent struct {
	Type  string        `json:"type"`
	Order *domain.Order `json:"order"`
}

// RegisterOrderEventRelay subscribes to the order.* subjects this service
// publishes and relays them to sink for live admin clients.
//
// Subscribing to its own published events (rather than fanning out inline at
// publish time) is deliberate: with more than one order task, only the task
// that handled the write would otherwise see the change, and clients connected
// to the other tasks would miss it.
func (s *Service) RegisterOrderEventRelay(ctx context.Context, subscriber ports.EventSubscriber, sink ports.OrderEventSink) error {
	if subscriber == nil || sink == nil {
		return nil
	}
	for _, subject := range []string{orderCreatedSubject, orderPaidSubject, orderUpdatedSubject} {
		handler := func(ctx context.Context, subject string, payload []byte) error {
			return s.relayOrderEvent(ctx, subject, payload, sink)
		}
		if err := subscriber.Subscribe(ctx, subject, handler); err != nil {
			return fmt.Errorf("subscribe %s for live stream: %w", subject, err)
		}
	}
	return nil
}

func (s *Service) relayOrderEvent(ctx context.Context, subject string, payload []byte, sink ports.OrderEventSink) error {
	var event events.Order
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode %s for live stream: %w", subject, err)
	}
	if event.OrderID == "" {
		return fmt.Errorf("%s missing order_id", subject)
	}

	eventID := orderEventStreamID(event)

	order, err := s.GetOrder(ctx, event.OrderID)
	if err != nil {
		// No authoritative snapshot to send. Tell live clients to reload rather
		// than leaving their table showing a row we know is stale.
		sink.PublishReset(eventID)
		return fmt.Errorf("load order %s for live stream: %w", event.OrderID, err)
	}

	body, err := json.Marshal(liveOrderEvent{Type: subject, Order: order})
	if err != nil {
		sink.PublishReset(eventID)
		return fmt.Errorf("marshal live %s for order %s: %w", subject, event.OrderID, err)
	}

	sink.PublishOrderEvent(eventID, sseOrderEventName, body)
	return nil
}

// orderEventStreamID derives the SSE id from the event payload so that every
// order task computes the same id for the same event — that is what lets a
// browser replay from its Last-Event-ID after reconnecting to another task.
//
// Ids are nanosecond timestamps from the publishing task's clock, so ordering
// across tasks is only as good as their clock sync. At admin order rates
// (events seconds apart at the closest) millisecond skew cannot reorder them.
// An unset timestamp yields "", which is delivered without moving the cursor.
func orderEventStreamID(event events.Order) string {
	at := event.Occurred
	if at.IsZero() {
		at = event.CreatedAt
	}
	if at.IsZero() {
		return ""
	}
	return strconv.FormatInt(at.UTC().UnixNano(), 10)
}
