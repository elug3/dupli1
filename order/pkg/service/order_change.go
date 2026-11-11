package service

import (
	"context"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/shared/pkg/events"
)

// orderChange is what a status event says happened, beside the order as it
// now stands: the status it left, the action, and whether a policy worker
// rather than a person did it. Ops alerts read it to show "확인됨 → 배송 중"
// instead of only where the order ended up.
type orderChange struct {
	from      domain.OrderStatus
	action    string
	automatic bool
}

type automaticKey struct{}

// withAutomatic marks ctx as a policy worker's (auto-confirm, auto-approve,
// auto-fulfill, unpaid expiry), so the changes it makes say so.
func withAutomatic(ctx context.Context) context.Context {
	return context.WithValue(ctx, automaticKey{}, true)
}

func isAutomatic(ctx context.Context) bool {
	v, _ := ctx.Value(automaticKey{}).(bool)
	return v
}

func (s *Service) change(ctx context.Context, from domain.OrderStatus, action string) *orderChange {
	return &orderChange{from: from, action: action, automatic: isAutomatic(ctx)}
}

// apply writes the change into the event. A nil change (order.created) adds
// nothing. The shopper's reason and the tracking are read from the order after
// the change, and only for the actions they belong to.
func (c *orderChange) apply(event *events.Order, order *domain.Order) {
	if c == nil {
		return
	}
	event.PreviousStatus = string(c.from)
	event.Change = c.action
	event.Automatic = c.automatic
	switch c.action {
	case events.OrderChangeCancelRequested:
		event.Reason = order.CancelRequestReason
	case events.OrderChangeDisputed:
		event.Reason = order.DisputeReason
	case events.OrderChangeShipped:
		event.Carrier = order.Carrier
		event.CarrierNote = order.CarrierNote
		event.TrackingNumber = order.TrackingNumber
	}
}

// orderChangePaid is for the one call site where a local named events
// shadows the package.
const orderChangePaid = events.OrderChangePaid
