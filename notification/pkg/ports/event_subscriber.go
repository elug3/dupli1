package ports

import "context"

// MessageHandler processes a NATS message payload.
type MessageHandler func(ctx context.Context, subject string, payload []byte) error

// EventSubscriber subscribes to NATS subjects and dispatches messages.
type EventSubscriber interface {
	Subscribe(ctx context.Context, subject string, handler MessageHandler) error
	Close()
}
