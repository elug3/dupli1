package ports

import "time"

// OutboxEvent is enqueued in the same transaction as a payment write.
type OutboxEvent struct {
	AggregateID string
	Subject     string
	Payload     []byte
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
