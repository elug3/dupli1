package ports

import (
	"errors"

	"github.com/elug3/dupli1/shared/pkg/outbox"
)

var (
	// ErrIdempotencyConflict is returned when an Idempotency-Key is reused with a different request body.
	ErrIdempotencyConflict = errors.New("idempotency key reused with different request")
)

// OutboxEvent is enqueued in the same transaction as an order write.
// Alias of the shared outbox type — see shared/pkg/outbox.
type OutboxEvent = outbox.Event

// OutboxMessage is a persisted outbox row awaiting (or completing) publish.
// Alias of the shared outbox type — see shared/pkg/outbox.
type OutboxMessage = outbox.Message

// IdempotencyRecord links a client Idempotency-Key to a created order.
type IdempotencyRecord struct {
	Key         string
	CustomerID  string
	OrderID     string
	RequestHash string
}
