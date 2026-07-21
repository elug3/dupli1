package ports

import (
	"errors"
	"time"
)

var (
	// ErrIdempotencyConflict is returned when an Idempotency-Key is reused with a different request body.
	ErrIdempotencyConflict = errors.New("idempotency key reused with different request")
)

// OutboxEvent is enqueued in the same transaction as an order write.
type OutboxEvent struct {
	AggregateID string
	Subject     string
	Payload     []byte // JSON bytes matching the published event shape
}

// OutboxMessage is a persisted outbox row awaiting (or completing) publish.
type OutboxMessage struct {
	ID          int64
	AggregateID string
	Subject     string
	Payload     []byte
	CreatedAt   time.Time
	Attempts    int
	LastError   string
}

// IdempotencyRecord links a client Idempotency-Key to a created order.
type IdempotencyRecord struct {
	Key         string
	CustomerID  string
	OrderID     string
	RequestHash string
}
