package livefeed

import (
	"context"
	"encoding/json"

	"github.com/elug3/dupli1/order/pkg/ports"
)

// Publisher feeds the hub from the outbox drainer. It is only for running
// without a broker (local dev, tests): with NATS configured every replica
// subscribes to order.* instead, which also carries changes committed by
// other replicas, and wrapping as well would send each change twice.
//
// Inner may be nil; the drainer then still marks rows published after the
// hub has seen them, as it does with no publisher at all.
type Publisher struct {
	Inner ports.EventPublisher
	Hub   *Hub
}

func (p Publisher) Publish(ctx context.Context, subject string, event any) error {
	if p.Inner != nil {
		if err := p.Inner.Publish(ctx, subject, event); err != nil {
			return err
		}
	}
	var payload []byte
	switch v := event.(type) {
	case json.RawMessage:
		payload = v
	case []byte:
		payload = v
	default:
		b, err := json.Marshal(event)
		if err != nil {
			return nil // the event itself went out; the stream just misses it
		}
		payload = b
	}
	p.Hub.Notify(ctx, subject, payload)
	return nil
}
