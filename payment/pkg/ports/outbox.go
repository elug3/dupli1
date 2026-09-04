package ports

import "github.com/elug3/dupli1/shared/pkg/outbox"

// OutboxEvent is enqueued in the same transaction as a payment write.
// Alias of the shared outbox type — see shared/pkg/outbox.
type OutboxEvent = outbox.Event

// OutboxMessage is a persisted outbox row awaiting (or completing) publish.
// Alias of the shared outbox type — see shared/pkg/outbox.
type OutboxMessage = outbox.Message
