package ports

import (
	"context"

	"github.com/elug3/dupli1/shared/pkg/events"
)

// PaymentSucceededSubject is the NATS subject payment publishes on success.
// Alias of the shared event — see shared/pkg/events.
const PaymentSucceededSubject = events.PaymentSucceeded

// PaymentSucceededEvent is an alias of the shared event payload — see shared/pkg/events.
type PaymentSucceededEvent = events.PaymentSucceededEvent

type EventPublisher interface {
	Publish(ctx context.Context, subject string, event any) error
}
